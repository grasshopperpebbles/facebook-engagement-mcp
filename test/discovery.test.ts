import { readFileSync } from "node:fs"
import { join } from "node:path"
import { Client } from "@modelcontextprotocol/sdk/client/index.js"
import { InMemoryTransport } from "@modelcontextprotocol/sdk/inMemory.js"
import { describe, expect, it, vi } from "vitest"
import { createEnvTokenProvider } from "../src/auth/token-provider.js"
import { MAX_CAMPAIGNS, runCommentActivity } from "../src/outcomes/activity.js"
import { createServer } from "../src/server.js"
import { createMetaClient, createPagesClient } from "../src/vendor/meta-client/index.js"

const root = join(import.meta.dirname, "..")
const fixture = (name: string) => readFileSync(join(root, "fixtures", `${name}.json`), "utf8")

/**
 * Serves the orientation fixtures. `overrides` replaces the response for a
 * path suffix, which is how the permission-failure cases are set up without a
 * second fetch stub.
 */
const discoveryFetch = (overrides: Record<string, () => Response> = {}) =>
  vi.fn(async (url: string) => {
    const { pathname } = new URL(url)
    for (const [suffix, respond] of Object.entries(overrides)) {
      if (pathname.endsWith(suffix)) return respond()
    }
    if (pathname.endsWith("/me/accounts")) return new Response(fixture("pages"), { status: 200 })
    if (pathname.endsWith("/me/adaccounts")) {
      return new Response(fixture("ad-accounts"), { status: 200 })
    }
    if (pathname.endsWith("/act_1/campaigns")) {
      return new Response(fixture("campaigns"), { status: 200 })
    }
    return new Response(JSON.stringify({ data: [] }), { status: 200 })
  })

function deps(fetchImpl = discoveryFetch()) {
  return {
    client: createPagesClient({ accessToken: "fixture", fetchImpl: fetchImpl as never }),
    tokens: createEnvTokenProvider({ userToken: "fixture", fetchImpl: fetchImpl as never }),
    meta: createMetaClient({ accessToken: "fixture", fetchImpl: fetchImpl as never }),
  }
}

/** A Graph permission refusal, the shape Meta returns for a missing scope. */
const refusal = (message: string) => () =>
  new Response(
    JSON.stringify({
      error: { message, type: "OAuthException", code: 200, fbtrace_id: "Atrace1" },
    }),
    { status: 403 },
  )

describe("orientation with no target", () => {
  it("lists the ad accounts this identity can reach alongside its Pages", async () => {
    const result = await runCommentActivity(deps(), {})

    expect(result).toMatchObject({
      pages: [{ id: "pg1", name: "Test Shop" }],
      adAccounts: [{ id: "act_1", name: "Harbour Grill Ads" }],
    })
  })

  it("still returns Pages when the token lacks ads_read, naming the scope in a note", async () => {
    // Most tokens will not carry ads_read. Orientation is the call a model
    // makes to find its feet, so a missing ad scope must degrade to "here are
    // your Pages, ads need this permission" rather than failing outright.
    const fetchImpl = discoveryFetch({
      "/me/adaccounts": refusal("(#200) Requires ads_read permission"),
    })

    const result = await runCommentActivity(deps(fetchImpl), {})

    expect(result).toMatchObject({ pages: [{ id: "pg1" }] })
    expect((result as { adAccounts?: unknown[] }).adAccounts).toBeUndefined()
    expect((result as { notes?: string[] }).notes?.join(" ")).toContain("ads_read")
  })
})

describe("adAccount target", () => {
  it("lists the account's campaigns by name and status", async () => {
    const result = await runCommentActivity(deps(), { adAccount: "act_1" })

    expect(result).toMatchObject({
      campaigns: [
        { id: "cmp1", name: "Spring Menu", status: "ACTIVE" },
        { id: "cmp2", name: "Weekend Brunch", status: "PAUSED" },
      ],
    })
  })

  it("reads no comments, so a campaign that ended is still listed", async () => {
    // Discovery is the cheap rung: it must not sweep posts or comments, or
    // orientation costs as much as the answer it is meant to precede.
    const fetchImpl = discoveryFetch()
    await runCommentActivity(deps(fetchImpl), { adAccount: "act_1" })

    const paths = fetchImpl.mock.calls.map(([url]) => new URL(url as string).pathname)
    expect(paths.some((path) => path.endsWith("/act_1/campaigns"))).toBe(true)
    expect(paths.some((path) => path.endsWith("/comments"))).toBe(false)
    expect(paths.some((path) => path.endsWith("/feed"))).toBe(false)
  })

  it("reports truncation rather than implying the list is complete", async () => {
    // An account with more campaigns than the cap must say so. A silently
    // short list is the exact defect this client was built to avoid.
    const overflowing = Array.from({ length: MAX_CAMPAIGNS + 1 }, (_, i) => ({
      id: `cmp${i}`,
      name: `Campaign ${i}`,
      effective_status: "ACTIVE",
    }))
    const fetchImpl = discoveryFetch({
      "/act_1/campaigns": () =>
        new Response(JSON.stringify({ data: overflowing }), { status: 200 }),
    })

    const result = await runCommentActivity(deps(fetchImpl), { adAccount: "act_1" })

    expect((result as { campaigns: unknown[] }).campaigns).toHaveLength(MAX_CAMPAIGNS)
    expect((result as { truncated?: boolean }).truncated).toBe(true)
  })

  it("explains a missing ads_read rather than returning an empty campaign list", async () => {
    const fetchImpl = discoveryFetch({
      "/act_1/campaigns": refusal("(#200) Requires ads_read permission"),
    })

    const result = await runCommentActivity(deps(fetchImpl), { adAccount: "act_1" })

    expect((result as { error: string }).error).toContain("ads_read")
  })

  it("refuses an adAccount given alongside another target", async () => {
    const result = await runCommentActivity(deps(), { adAccount: "act_1", page: "pg1" })

    expect((result as { error: string }).error).toMatch(/one target only/i)
  })
})

describe("the tool surface", () => {
  it("accepts an adAccount target, so a model can walk from account to campaigns", async () => {
    const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair()
    const server = createServer({ accessToken: "fixture", fetchImpl: discoveryFetch() as never })
    const client = new Client({ name: "test", version: "1.0.0" })
    await Promise.all([server.connect(serverTransport), client.connect(clientTransport)])

    const result = await client.callTool({
      name: "comment_activity",
      arguments: { adAccount: "act_1" },
    })
    await client.close()

    const [content] = result.content as { text: string }[]
    expect(JSON.parse(content?.text ?? "{}")).toMatchObject({
      campaigns: [{ name: "Spring Menu" }, { name: "Weekend Brunch" }],
    })
  })
})
