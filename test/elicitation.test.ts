import { Client } from "@modelcontextprotocol/sdk/client/index.js"
import { InMemoryTransport } from "@modelcontextprotocol/sdk/inMemory.js"
import { describe, expect, it, vi } from "vitest"
import { createServer } from "../src/server.js"

const ok = (body: unknown) => new Response(JSON.stringify(body), { status: 200 })

/**
 * Connects a client that either supports elicitation and answers as told, or
 * does not support it at all.
 */
async function connect(options: {
  elicitation?: "accept" | "decline"
  /**
   * A raw elicitation reply, for the shapes `accept`/`decline` cannot express:
   * a cancelled prompt, or an accept whose `confirm` is missing, false, or not
   * a boolean at all. Every one of those is a refusal.
   */
  elicitReply?: { action: string; content?: Record<string, unknown> }
  /** Mirrors the operator-set environment override. */
  allowUnconfirmedWrites?: boolean
  fetchImpl: ReturnType<typeof vi.fn>
}) {
  const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair()
  const server = createServer({
    accessToken: "fixture",
    enableWrites: true,
    ...(options.allowUnconfirmedWrites === true && { allowUnconfirmedWrites: true }),
    fetchImpl: options.fetchImpl as never,
  })
  const elicits = options.elicitation !== undefined || options.elicitReply !== undefined
  const client = new Client(
    { name: "test", version: "1.0.0" },
    { capabilities: elicits ? { elicitation: {} } : {} },
  )

  if (elicits) {
    client.setRequestHandler(
      // The elicitation request schema is exported by the SDK; import it in the
      // implementation and reuse it here.
      (await import("@modelcontextprotocol/sdk/types.js")).ElicitRequestSchema,
      async () =>
        options.elicitReply ??
        (options.elicitation === "accept"
          ? { action: "accept", content: { confirm: true } }
          : { action: "decline" }),
    )
  }

  await Promise.all([server.connect(serverTransport), client.connect(clientTransport)])
  return client
}

describe("respond_to_comment confirmation", () => {
  it("publishes after the user accepts the elicitation", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(ok({ id: "c1_r1" }))
    const client = await connect({ elicitation: "accept", fetchImpl })

    const result = await client.callTool({
      name: "respond_to_comment",
      arguments: { commentId: "c1", message: "Thanks!" },
    })

    expect(result.isError).toBeFalsy()
    expect(fetchImpl).toHaveBeenCalled()
    await client.close()
  })

  it("publishes nothing when the user declines", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(ok({ id: "c1_r1" }))
    const client = await connect({ elicitation: "decline", fetchImpl })

    const result = await client.callTool({
      name: "respond_to_comment",
      arguments: { commentId: "c1", message: "Thanks!" },
    })

    expect(result.isError).toBe(true)
    expect(fetchImpl).not.toHaveBeenCalled()
    await client.close()
  })

  it("does not ask for confirmation on a dry run", async () => {
    const fetchImpl = vi.fn()
    const client = await connect({ fetchImpl })

    const result = await client.callTool({
      name: "respond_to_comment",
      arguments: { commentId: "c1", message: "Thanks!", dryRun: true },
    })

    expect(result.isError).toBeFalsy()
    await client.close()
  })

  it("refuses rather than publishing unconfirmed when the client cannot elicit", async () => {
    // Falling back to publishing would turn an unsupported capability into a
    // silent removal of the gate.
    const fetchImpl = vi.fn()
    const client = await connect({ fetchImpl })

    const result = await client.callTool({
      name: "respond_to_comment",
      arguments: { commentId: "c1", message: "Thanks!" },
    })

    expect(result.isError).toBe(true)
    const content = result.content as { text: string }[]
    expect(content[0]!.text).toMatch(/confirmed/i)
    expect(fetchImpl).not.toHaveBeenCalled()
    await client.close()
  })

  // Reversal of an earlier decision, 2026-09-08. The tool used to accept a
  // `confirmed` boolean as a stand-in for a human when the client could not
  // prompt. That flag is a tool argument, so the model supplied it — observed
  // in Claude Desktop, which does not elicit: asked to publish, the model
  // announced it would "fire it with dryRun: false and confirmed: true". A
  // confirmation the model can grant itself is not a confirmation.
  it("refuses to publish when the client cannot elicit, even if asked to confirm", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(ok({ id: "c1_r1" }))
    const client = await connect({ fetchImpl })

    const result = await client.callTool({
      name: "respond_to_comment",
      arguments: { commentId: "c1", message: "Thanks!", confirmed: true },
    })

    expect(result.isError).toBe(true)
    expect(fetchImpl).not.toHaveBeenCalled()
    await client.close()
  })

  // The escape hatch for automation lives in the environment, which a person
  // sets and a model cannot reach — the whole point of moving it there.
  it("publishes without a prompt only when the operator set the environment override", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(ok({ id: "c1_r1" }))
    const client = await connect({ fetchImpl, allowUnconfirmedWrites: true })

    const result = await client.callTool({
      name: "respond_to_comment",
      arguments: { commentId: "c1", message: "Thanks!" },
    })

    expect(result.isError).toBeFalsy()
    expect(fetchImpl).toHaveBeenCalled()
    await client.close()
  })

  it("publishes nothing when the user cancels the prompt", async () => {
    // "cancel" is not "decline" on the wire — a client sends it when the user
    // dismisses the dialog rather than answering it. Anything but an explicit
    // yes is a refusal.
    const fetchImpl = vi.fn().mockResolvedValue(ok({ id: "c1_r1" }))
    const client = await connect({ elicitReply: { action: "cancel" }, fetchImpl })

    const result = await client.callTool({
      name: "respond_to_comment",
      arguments: { commentId: "c1", message: "Thanks!" },
    })

    expect(result.isError).toBe(true)
    expect(fetchImpl).not.toHaveBeenCalled()
    await client.close()
  })

  it("publishes nothing when an accept carries no confirm at all", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(ok({ id: "c1_r1" }))
    const client = await connect({ elicitReply: { action: "accept", content: {} }, fetchImpl })

    const result = await client.callTool({
      name: "respond_to_comment",
      arguments: { commentId: "c1", message: "Thanks!" },
    })

    expect(result.isError).toBe(true)
    expect(fetchImpl).not.toHaveBeenCalled()
    await client.close()
  })

  it("publishes nothing when an accept carries confirm: false", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(ok({ id: "c1_r1" }))
    const client = await connect({
      elicitReply: { action: "accept", content: { confirm: false } },
      fetchImpl,
    })

    const result = await client.callTool({
      name: "respond_to_comment",
      arguments: { commentId: "c1", message: "Thanks!" },
    })

    expect(result.isError).toBe(true)
    expect(fetchImpl).not.toHaveBeenCalled()
    await client.close()
  })

  it("publishes nothing when confirm is a truthy non-boolean", async () => {
    // A string "yes" or the number 1 is not consent this server can read.
    // Anything less than `=== true` must not open the gate.
    const fetchImpl = vi.fn().mockResolvedValue(ok({ id: "c1_r1" }))
    const client = await connect({
      elicitReply: { action: "accept", content: { confirm: "yes" } },
      fetchImpl,
    })

    const result = await client.callTool({
      name: "respond_to_comment",
      arguments: { commentId: "c1", message: "Thanks!" },
    })

    expect(result.isError).toBe(true)
    expect(fetchImpl).not.toHaveBeenCalled()
    await client.close()
  })

  it("never asks for confirmation before hiding, which is reversible", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(ok({ success: true }))
    const client = await connect({ fetchImpl })

    const result = await client.callTool({
      name: "moderate_comment",
      arguments: { commentId: "c1", action: "hide" },
    })

    expect(result.isError).toBeFalsy()
    await client.close()
  })
})
