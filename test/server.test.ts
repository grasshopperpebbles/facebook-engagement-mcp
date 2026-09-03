import { Client } from "@modelcontextprotocol/sdk/client/index.js"
import { InMemoryTransport } from "@modelcontextprotocol/sdk/inMemory.js"
import { describe, expect, it } from "vitest"
import { createServer, SERVER_NAME, SERVER_VERSION } from "../src/server.js"

describe("server", () => {
  it("constructs and connects over a transport", async () => {
    const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair()
    const server = createServer({ accessToken: "fixture" })
    const client = new Client({ name: "test", version: "1.0.0" })

    await Promise.all([server.connect(serverTransport), client.connect(clientTransport)])

    expect(client.getServerVersion()).toMatchObject({
      name: SERVER_NAME,
      version: SERVER_VERSION,
    })
    expect((await client.listTools()).tools.map((t) => t.name)).toEqual(["comment_activity"])
    await client.close()
  })

  it("names itself for this server, not the marketing one", () => {
    expect(SERVER_NAME).toBe("facebook-engagement-mcp")
  })
})

describe("tool surface", () => {
  it("stays small enough not to crowd out the conversation", async () => {
    // Tool definitions are spent before the model reads the user's question.
    // The ceiling is the product, not a limitation of it.
    const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair()
    const server = createServer({ accessToken: "fixture", enableWrites: true })
    const client = new Client({ name: "test", version: "1.0.0" })
    await Promise.all([server.connect(serverTransport), client.connect(clientTransport)])

    const { tools } = await client.listTools()

    expect(tools.length).toBeLessThanOrEqual(12)
    // ~4 chars per token is the working estimate.
    expect(Math.ceil(JSON.stringify(tools).length / 4)).toBeLessThan(4000)
    await client.close()
  })

  it("registers exactly the three documented tools when writes are enabled", async () => {
    const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair()
    const server = createServer({ accessToken: "fixture", enableWrites: true })
    const client = new Client({ name: "test", version: "1.0.0" })
    await Promise.all([server.connect(serverTransport), client.connect(clientTransport)])

    expect((await client.listTools()).tools.map((t) => t.name).sort()).toEqual([
      "comment_activity",
      "moderate_comment",
      "respond_to_comment",
    ])
    await client.close()
  })
})
