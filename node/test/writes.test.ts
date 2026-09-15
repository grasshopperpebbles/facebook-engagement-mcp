import { Client } from "@modelcontextprotocol/sdk/client/index.js"
import { InMemoryTransport } from "@modelcontextprotocol/sdk/inMemory.js"
import { describe, expect, it, vi } from "vitest"
import { createEnvTokenProvider } from "../src/auth/token-provider.js"
import { createServer } from "../src/server.js"
import { runModerateComment } from "../src/tools/moderate-comment.js"
import { runRespondToComment } from "../src/tools/respond-to-comment.js"
import { createPagesClient } from "../src/vendor/meta-client/index.js"

const ok = (body: unknown) => new Response(JSON.stringify(body), { status: 200 })

/**
 * A fetch mock whose `/me/accounts` lookup succeeds, so the test exercises the
 * write itself. Both write tools resolve a Page token before writing (T-26) —
 * a bare `mockResolvedValue` would hand the accounts lookup the write's reply.
 */
function withPage(writeResponse: () => Response) {
  return vi.fn(async (url: string) => {
    if (new URL(url).pathname.endsWith("/me/accounts")) {
      return ok({ data: [{ id: "pg1", access_token: "pt", tasks: ["MODERATE"] }] })
    }
    return writeResponse()
  })
}

/** Calls that are not the Page-token lookup, for counting actual writes. */
const writeCalls = (f: { mock: { calls: unknown[][] } }) =>
  f.mock.calls.filter((c) => !new URL(c[0] as string).pathname.endsWith("/me/accounts"))

function deps(fetchImpl: ReturnType<typeof vi.fn>) {
  return {
    client: createPagesClient({ accessToken: "fixture", fetchImpl: fetchImpl as never }),
    tokens: createEnvTokenProvider({ userToken: "fixture", fetchImpl: fetchImpl as never }),
  }
}

describe("registration gating", () => {
  async function toolNames(enableWrites: boolean) {
    const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair()
    const server = createServer({ accessToken: "fixture", enableWrites })
    const client = new Client({ name: "test", version: "1.0.0" })
    await Promise.all([server.connect(serverTransport), client.connect(clientTransport)])
    const names = (await client.listTools()).tools.map((t) => t.name).sort()
    await client.close()
    return names
  }

  it("registers no write tools by default", async () => {
    expect(await toolNames(false)).toEqual(["comment_activity"])
  })

  it("registers write tools when explicitly enabled", async () => {
    expect(await toolNames(true)).toEqual([
      "comment_activity",
      "moderate_comment",
      "respond_to_comment",
    ])
  })

  it("marks write tools as not read-only", async () => {
    const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair()
    const server = createServer({ accessToken: "fixture", enableWrites: true })
    const client = new Client({ name: "test", version: "1.0.0" })
    await Promise.all([server.connect(serverTransport), client.connect(clientTransport)])

    const { tools } = await client.listTools()
    const reply = tools.find((t) => t.name === "respond_to_comment")
    expect(reply?.annotations?.readOnlyHint).toBe(false)
    await client.close()
  })
})

describe("runRespondToComment", () => {
  it("does not call Graph in dry-run mode", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(ok({ id: "c1_r1" }))
    const result = await runRespondToComment(deps(fetchImpl), {
      commentId: "c1",
      message: "Thanks!",
      pageId: "pg1",
      dryRun: true,
    })

    expect(fetchImpl).not.toHaveBeenCalled()
    expect(result).toMatchObject({ dryRun: true })
  })

  it("reports what a dry run would publish and as whom", async () => {
    const fetchImpl = vi.fn()
    const result = await runRespondToComment(deps(fetchImpl), {
      commentId: "c1",
      message: "Thanks!",
      pageId: "pg1",
      dryRun: true,
    })

    expect(result).toMatchObject({
      dryRun: true,
      intended: { commentId: "c1", messageLength: 7, visibility: "public" },
    })
  })

  it("posts the reply and returns its id", async () => {
    const fetchImpl = withPage(() => ok({ id: "c1_r1" }))
    const result = await runRespondToComment(deps(fetchImpl), {
      commentId: "c1",
      message: "Thanks!",
      pageId: "pg1",
    })

    expect(result).toMatchObject({ ok: true, replyId: "c1_r1" })
  })

  it("rejects an empty message rather than posting it", async () => {
    const fetchImpl = vi.fn()
    const result = await runRespondToComment(deps(fetchImpl), {
      commentId: "c1",
      message: "   ",
      pageId: "pg1",
    })

    expect(result).toHaveProperty("error")
    expect(fetchImpl).not.toHaveBeenCalled()
  })

  it("explains a missing MODERATE task instead of passing Graph's error through", async () => {
    const fetchImpl = vi.fn(async (url: string) => {
      if (new URL(url).pathname.endsWith("/me/accounts")) {
        return ok({ data: [{ id: "pg1", access_token: "pt", tasks: ["ANALYZE"] }] })
      }
      return new Response(JSON.stringify({ error: { message: "(#200) Permissions", code: 200 } }), {
        status: 403,
      })
    })
    const result = await runRespondToComment(deps(fetchImpl), {
      commentId: "c1",
      message: "Hi",
      pageId: "pg1",
    })

    expect((result as { error: string }).error).toContain("MODERATE")
  })

  it("never retries a failed write", async () => {
    // A retried reply is a double post.
    const fetchImpl = withPage(() => new Response("{}", { status: 500 }))
    await runRespondToComment(deps(fetchImpl), { commentId: "c1", message: "Hi", pageId: "pg1" })

    expect(writeCalls(fetchImpl)).toHaveLength(1)
  })
})

describe("runModerateComment", () => {
  it("hides through one call with is_hidden true", async () => {
    const fetchImpl = withPage(() => ok({ success: true }))
    const result = await runModerateComment(deps(fetchImpl), {
      commentId: "c1",
      action: "hide",
      pageId: "pg1",
    })

    expect(String((writeCalls(fetchImpl)[0]![1] as RequestInit).body)).toContain("is_hidden=true")
    expect(result).toMatchObject({ ok: true, action: "hide" })
  })

  it("unhides through the same call with is_hidden false", async () => {
    const fetchImpl = withPage(() => ok({ success: true }))
    await runModerateComment(deps(fetchImpl), {
      commentId: "c1",
      action: "unhide",
      pageId: "pg1",
    })

    expect(String((writeCalls(fetchImpl)[0]![1] as RequestInit).body)).toContain("is_hidden=false")
  })

  it("does not call Graph in dry-run mode", async () => {
    const fetchImpl = vi.fn()
    const result = await runModerateComment(deps(fetchImpl), {
      commentId: "c1",
      action: "hide",
      pageId: "pg1",
      dryRun: true,
    })

    expect(fetchImpl).not.toHaveBeenCalled()
    expect(result).toMatchObject({ dryRun: true, intended: { commentId: "c1", action: "hide" } })
  })

  it("reports an unsuccessful Graph response as an error, not a success", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(ok({ success: false }))
    const result = await runModerateComment(deps(fetchImpl), { commentId: "c1", action: "hide" })

    expect(result).toHaveProperty("error")
  })
})

describe("a write must carry a Page token (T-26)", () => {
  // Established live on 2026-09-08. `publish_actions` was the permission for
  // publishing AS A USER; it was removed in 2018. So a comment write carrying a
  // user token reaches a code path whose permission no longer exists, and Graph
  // reports that permission by name:
  //
  //   (#200) The permission(s) publish_actions are not available. It has been deprecated.
  //
  // The message is literally true and entirely misleading — the problem is the
  // identity, not the permission — and it cost a day. Both write tools took
  // `pageId` as OPTIONAL and fell back to the startup user-token client when it
  // was absent, so a caller supplying only a comment id got exactly that.
  const userTokenRefusal = () =>
    new Response(
      JSON.stringify({
        error: {
          message:
            "(#200) The permission(s) publish_actions are not available. It has been deprecated.",
          type: "OAuthException",
          code: 200,
        },
      }),
      { status: 403 },
    )

  it("respond_to_comment refuses without a pageId rather than publishing as the user", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(userTokenRefusal())
    const result = await runRespondToComment(deps(fetchImpl), {
      commentId: "c1",
      message: "Thanks!",
    })
    expect(result).toHaveProperty("error")
    // Refused locally: the call must not have been attempted at all.
    expect(fetchImpl).not.toHaveBeenCalled()
    expect((result as { error: string }).error).toMatch(/pageId/)
  })

  it("moderate_comment refuses without a pageId rather than acting as the user", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(userTokenRefusal())
    const result = await runModerateComment(deps(fetchImpl), { commentId: "c1", action: "hide" })
    expect(result).toHaveProperty("error")
    expect(fetchImpl).not.toHaveBeenCalled()
    expect((result as { error: string }).error).toMatch(/pageId/)
  })

  it("explains the publish_actions refusal as an identity problem, not a permission one", async () => {
    // The Page lookup must succeed so the refusal under test is the REPLY, not
    // the token exchange in front of it. Routed by URL rather than queued:
    // the credential lookup is cached, so a queued sequence puts the accounts
    // response in front of the write.
    const fetchImpl = withPage(() => userTokenRefusal())
    const result = await runRespondToComment(deps(fetchImpl), {
      commentId: "c1",
      message: "Thanks!",
      pageId: "pg1",
    })
    const error = (result as { error: string }).error
    // Whatever else it says, it must not send the reader to App Review for a
    // permission that has not existed since 2018.
    expect(error).toMatch(/publish_actions/)
    expect(error).toMatch(/Page token|identity|user token/i)
  })
})
