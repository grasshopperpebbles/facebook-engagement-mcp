#!/usr/bin/env node
/**
 * Prove the packed bundle actually launches, by launching it.
 *
 * This exists because of a defect this project already shipped. The `bin`
 * entrypoint guard compared `import.meta.url` against an unresolved
 * `process.argv[1]`, so under npm's bin symlink the server exited 0 having
 * started nothing — silently, leaving the client reporting a closed
 * connection. Twelve reviews read that line. What found it was installing the
 * tarball and launching it the way a user would.
 *
 * The same guard runs under `node <bundle>/server/index.js`, which is exactly
 * how Claude Desktop starts an extension. So: unpack the real .mcpb, launch the
 * real entry point by the path `mcp_config` names, and speak real MCP to it.
 *
 * No credential is needed. The server only requires META_ACCESS_TOKEN to be
 * non-empty at startup; nothing here calls Graph.
 */
import { execFileSync } from "node:child_process"
import { mkdtempSync, readdirSync, readFileSync, rmSync } from "node:fs"
import { tmpdir } from "node:os"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

import { Client } from "@modelcontextprotocol/sdk/client/index.js"
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js"

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const pkg = JSON.parse(readFileSync(join(root, "package.json"), "utf8"))
const bundle = join(root, "build", `${pkg.name}-${pkg.version}.mcpb`)

const fail = (message) => {
  console.error(`FAIL: ${message}`)
  process.exit(1)
}

if (!readdirSync(join(root, "build")).some((name) => name.endsWith(".mcpb"))) {
  fail("no bundle found — run `npm run mcpb:pack` first")
}

const unpacked = mkdtempSync(join(tmpdir(), "mcpb-verify-"))
console.log(`• unpacking ${bundle}`)
// A .mcpb is a zip. Unzipping it is the closest thing to what the client does
// on install, and it proves the archive contains what the manifest promises.
execFileSync("unzip", ["-q", bundle, "-d", unpacked], { stdio: "inherit" })

const manifest = JSON.parse(readFileSync(join(unpacked, "manifest.json"), "utf8"))
const entry = join(unpacked, manifest.server.entry_point)
console.log(`• launching ${manifest.server.mcp_config.command} ${entry}`)

/** Start the packed server exactly as `mcp_config` says to, and ask it things. */
async function inspect(enableWrites) {
  const transport = new StdioClientTransport({
    command: manifest.server.mcp_config.command,
    args: [entry],
    env: {
      ...process.env,
      META_ACCESS_TOKEN: "verify-only-not-a-real-token",
      FACEBOOK_ENGAGEMENT_ENABLE_WRITES: enableWrites,
    },
  })
  const client = new Client({ name: "mcpb-verify", version: "1.0.0" })
  await client.connect(transport)
  const { tools } = await client.listTools()
  const server = client.getServerVersion()
  await client.close()
  const activity = tools.find((tool) => tool.name === "comment_activity")
  return {
    tools: tools.map((tool) => tool.name).sort(),
    activityInputs: Object.keys(activity?.inputSchema?.properties ?? {}),
    server,
  }
}

const readOnly = await inspect("false")
console.log(`• server: ${readOnly.server?.name} ${readOnly.server?.version}`)
console.log(`• tools (writes off): ${readOnly.tools.join(", ")}`)

if (!readOnly.tools.includes("comment_activity")) {
  fail("the packed server registered no comment_activity tool")
}
if (readOnly.tools.length !== 1) {
  fail(`writes are off, so only comment_activity should register; got ${readOnly.tools.join(", ")}`)
}

const withWrites = await inspect("true")
console.log(`• tools (writes on):  ${withWrites.tools.join(", ")}`)
if (withWrites.tools.length !== 3) {
  fail(
    "FACEBOOK_ENGAGEMENT_ENABLE_WRITES=true did not register the write tools — the manifest's " +
      "env name and the server's do not agree",
  )
}

// The ad rungs are why a non-technical caller can use this at all: without
// them an ad id has to come out of Ads Manager by hand. Assert the packed
// schema still offers them.
for (const target of ["adAccount", "ad", "campaign"]) {
  if (!readOnly.activityInputs.includes(target)) {
    fail(`the packed comment_activity schema has no \`${target}\` parameter`)
  }
}

rmSync(unpacked, { recursive: true, force: true })
console.log("\nOK — the packed bundle launches and speaks MCP.")
