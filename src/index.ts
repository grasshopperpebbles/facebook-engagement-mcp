#!/usr/bin/env node
import { pathToFileURL } from "node:url"
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js"
import { createServer } from "./server.js"

export { createServer, type ServerOptions } from "./server.js"

/**
 * Credentials come from the environment, never from tool arguments — anything
 * in a tool argument was produced by a model (architecture §7).
 */
async function main(): Promise<void> {
  const accessToken = process.env.META_ACCESS_TOKEN
  if (!accessToken) {
    console.error("META_ACCESS_TOKEN is required. See the README for the scopes it needs.")
    process.exit(1)
  }

  const enableWrites = process.env["FACEBOOK_ENGAGEMENT_ENABLE_WRITES"] === "true"
  if (!enableWrites) {
    console.error(
      "Write tools are disabled. Set FACEBOOK_ENGAGEMENT_ENABLE_WRITES=true to enable replying and hiding.",
    )
  }

  await createServer({ accessToken, enableWrites }).connect(new StdioServerTransport())
}

const invokedDirectly =
  process.argv[1] !== undefined && import.meta.url === pathToFileURL(process.argv[1]).href

if (invokedDirectly) {
  main().catch((error: unknown) => {
    console.error(error instanceof Error ? error.message : String(error))
    process.exit(1)
  })
}
