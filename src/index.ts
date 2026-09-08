#!/usr/bin/env node
import { realpathSync } from "node:fs"
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

  // Deliberately environment-only: a model can set a tool argument, and did.
  // See ServerOptions.allowUnconfirmedWrites.
  const allowUnconfirmedWrites =
    process.env["FACEBOOK_ENGAGEMENT_ALLOW_UNCONFIRMED_WRITES"] === "true"
  if (enableWrites && allowUnconfirmedWrites) {
    console.error(
      "FACEBOOK_ENGAGEMENT_ALLOW_UNCONFIRMED_WRITES=true: replies will publish without asking, " +
        "on clients that cannot show a confirmation prompt.",
    )
  }

  await createServer({ accessToken, enableWrites, allowUnconfirmedWrites }).connect(
    new StdioServerTransport(),
  )
}

/**
 * Only start a stdio server when this module is the process entry point, so
 * importing `createServer` from it does not seize stdin and stdout.
 *
 * Two details make the comparison exact rather than approximate. Comparing
 * basenames instead would be wrong: any `index.js` anywhere on disk would
 * match, so an unrelated entry point that happened to share the name would
 * start a server nobody asked for. And `process.argv[1]` has to be resolved
 * through `realpathSync` first, because npm installs a `bin` as a symlink
 * (`node_modules/.bin/facebook-engagement-mcp` -> the real `dist/index.js`).
 * Node reports the symlink path in `argv[1]` and the resolved realpath in
 * `import.meta.url`, so an unresolved comparison is false for every launch a
 * client actually makes, and the process exits 0 having served nothing.
 */
const invokedDirectly =
  process.argv[1] !== undefined &&
  import.meta.url === pathToFileURL(realpathSync(process.argv[1])).href

if (invokedDirectly) {
  main().catch((error: unknown) => {
    console.error(error instanceof Error ? error.message : String(error))
    process.exit(1)
  })
}
