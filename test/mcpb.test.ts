import { existsSync, readFileSync } from "node:fs"
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
  documentation?: string
  user_config: Record<
    string,
    { type: string; required?: boolean; sensitive?: boolean; description?: string }
  >
}

/** The blob URL prefix under which every in-repo link in the manifest lives. */
const BLOB_PREFIX = "https://github.com/grasshopperpebbles/facebook-engagement-mcp/blob/main/"

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
const DELIBERATELY_NOT_IN_MANIFEST = new Map<string, string>()

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

  // Reversal, 2026-09-08. This override was deliberately kept out of the
  // manifest earlier the same day, on the reasoning that a checkbox would let a
  // non-technical operator turn off confirmation. That reasoning was wrong about
  // the client it ships to: Claude Desktop cannot show the server's prompt at
  // all, so withholding the field did not add a safeguard — it made publishing
  // impossible. The property that mattered was never "hard to switch on", it was
  // "a model cannot switch it on", and a config field a person ticks keeps that.
  it("offers the confirmation override as its own field, off by default", () => {
    const key = manifest.server.mcp_config.env[
      "FACEBOOK_ENGAGEMENT_ALLOW_UNCONFIRMED_WRITES"
    ]?.match(/\$\{user_config\.([^}]+)\}/)?.[1]

    expect(key).toBeDefined()
    expect(manifest.user_config[key ?? ""]).toMatchObject({ type: "boolean", default: false })
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

  // The install form asks a non-technical person for a Meta access token, which
  // is the one thing in this bundle they cannot produce by reading the form. The
  // field used to describe a token without saying where to get one, so the
  // answer was "ask whoever sent you the file".
  it("sends someone with no token to the guide that makes one", () => {
    const description = manifest.user_config.meta_access_token?.description ?? ""

    expect(manifest.documentation).toContain(BLOB_PREFIX)
    expect(description).toContain(manifest.documentation)
  })

  // A link that 404s is this project's own recurring defect — the monorepo
  // README carried one, and the setup article's slug is not the one its title
  // suggests. Every in-repo link the manifest names is resolved against the
  // working tree here, offline, so a renamed doc breaks the build rather than
  // the install screen.
  it("names only in-repo documents that exist", () => {
    const paths = [...JSON.stringify(manifest).matchAll(/blob\/main\/([\w./-]+\.md)/g)].map(
      (match) => match[1],
    )

    expect(paths.length).toBeGreaterThan(0)
    for (const path of paths) {
      expect(existsSync(join(root, path)), `${path} is linked from the manifest`).toBe(true)
    }
  })
})
