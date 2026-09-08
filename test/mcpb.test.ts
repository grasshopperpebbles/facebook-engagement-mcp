import { readFileSync } from "node:fs"
import { join } from "node:path"
import { describe, expect, it } from "vitest"

/**
 * Consistency checks on the MCPB manifest.
 *
 * These are cheap and run in `npm run check`. They do NOT prove the bundle
 * works — only `npm run mcpb:verify`, which packs it and launches the packed
 * entry point, does that. What they catch is drift: a manifest that names an
 * environment variable the server does not read, or a `${user_config.x}` with
 * no field behind it, fails silently and in the safest-looking way. A manifest
 * naming `PAGE_ENGAGEMENT_ENABLE_WRITES` (the monorepo's spelling) would leave
 * writes disabled forever, and nothing at runtime would say why.
 */

const root = join(import.meta.dirname, "..")

interface Manifest {
  manifest_version: string
  name: string
  version: string
  server: {
    type: string
    entry_point: string
    mcp_config: { command: string; args: string[]; env: Record<string, string> }
  }
  user_config: Record<string, { type: string; required?: boolean; sensitive?: boolean }>
}

const manifest = JSON.parse(readFileSync(join(root, "mcpb", "manifest.json"), "utf8")) as Manifest
const packageJson = JSON.parse(readFileSync(join(root, "package.json"), "utf8")) as {
  version: string
}
const entrySource = readFileSync(join(root, "src", "index.ts"), "utf8")

/** Every `process.env.X` and `process.env["X"]` the server's entry point reads. */
function environmentRead(source: string): Set<string> {
  const names = new Set<string>()
  for (const match of source.matchAll(/process\.env(?:\.(\w+)|\["([^"]+)"\])/g)) {
    names.add(match[1] ?? match[2] ?? "")
  }
  return names
}

/**
 * Environment variables the server reads that the manifest deliberately does
 * NOT pass, with the reason. A bundle is installed by the person least equipped
 * to judge the consequence, so anything dangerous stays out of the install form.
 */
const DELIBERATELY_NOT_IN_MANIFEST = new Map([
  [
    "FACEBOOK_ENGAGEMENT_ALLOW_UNCONFIRMED_WRITES",
    "Publishing without a confirmation prompt. Claude Desktop cannot elicit, so a checkbox " +
      "here would make every reply publish silently — handing the bypass to exactly the " +
      "audience this bundle exists for. Set it in a shell if you are automating deliberately.",
  ],
])

describe("mcpb manifest", () => {
  it("passes every environment variable the server reads, minus the deliberate omissions", () => {
    const passed = new Set(Object.keys(manifest.server.mcp_config.env))
    const read = environmentRead(entrySource)
    for (const name of DELIBERATELY_NOT_IN_MANIFEST.keys()) read.delete(name)

    expect(passed).toEqual(read)
  })

  it("does not offer the unconfirmed-write override in the install form", () => {
    for (const name of DELIBERATELY_NOT_IN_MANIFEST.keys()) {
      expect(JSON.stringify(manifest)).not.toContain(name)
    }
  })

  it("resolves every ${user_config.x} to a declared field", () => {
    const referenced = [
      ...JSON.stringify(manifest.server.mcp_config).matchAll(/\$\{user_config\.([^}]+)\}/g),
    ].map((match) => match[1])

    expect(referenced.length).toBeGreaterThan(0)
    for (const key of referenced) {
      expect(Object.keys(manifest.user_config)).toContain(key)
    }
  })

  it("marks the access token sensitive and required", () => {
    const tokenKey = Object.entries(manifest.server.mcp_config.env).find(
      ([name]) => name === "META_ACCESS_TOKEN",
    )?.[1]
    const key = tokenKey?.match(/\$\{user_config\.([^}]+)\}/)?.[1] ?? ""

    expect(manifest.user_config[key]).toMatchObject({ sensitive: true, required: true })
  })

  it("defaults writes to off, so a bundle cannot post as the Page until asked", () => {
    const writesKey =
      manifest.server.mcp_config.env["FACEBOOK_ENGAGEMENT_ENABLE_WRITES"]?.match(
        /\$\{user_config\.([^}]+)\}/,
      )?.[1] ?? ""

    expect(manifest.user_config[writesKey]).toMatchObject({ type: "boolean", default: false })
  })

  it("launches the entry point the packer stages, by the path mcp_config names", () => {
    expect(manifest.server.entry_point).toBe("server/index.js")
    expect(manifest.server.mcp_config.args).toEqual(["${__dirname}/server/index.js"])
    expect(manifest.server.mcp_config.command).toBe("node")
  })

  it("carries the package's own version, so a bundle is traceable to a build", () => {
    expect(manifest.version).toBe(packageJson.version)
  })
})
