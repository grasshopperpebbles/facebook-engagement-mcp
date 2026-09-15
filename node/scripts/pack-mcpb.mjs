#!/usr/bin/env node
/**
 * Build the .mcpb bundle Claude Desktop installs.
 *
 * A bundle is a zip of a staging directory: the manifest at its root, the
 * compiled server under `server/`, and the production `node_modules` beside
 * them. Claude Desktop supplies its own Node, so nothing here is a runtime —
 * only this package's two dependencies.
 *
 * The staging directory is generated and never committed. `npm run mcpb:verify`
 * is what proves the result works; this script only builds it.
 */
import { execFileSync } from "node:child_process"
import { cpSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const staging = join(root, "build", "mcpb")
const manifestPath = join(root, "mcpb", "manifest.json")

const run = (command, args, cwd) =>
  execFileSync(command, args, { cwd, stdio: "inherit", env: process.env })

const pkg = JSON.parse(readFileSync(join(root, "package.json"), "utf8"))
const manifest = JSON.parse(readFileSync(manifestPath, "utf8"))

// The version is asserted equal in test/mcpb.test.ts; re-checking here means a
// packed bundle can never carry a version the tests have not seen.
if (manifest.version !== pkg.version) {
  throw new Error(
    `manifest.json is ${manifest.version} but package.json is ${pkg.version}. ` +
      "A bundle must be traceable to a build.",
  )
}

console.log("• building")
run("npm", ["run", "build"], root)

console.log("• staging")
rmSync(staging, { recursive: true, force: true })
mkdirSync(staging, { recursive: true })
cpSync(join(root, "dist"), join(staging, "server"), { recursive: true })
cpSync(manifestPath, join(staging, "manifest.json"))
cpSync(join(root, "README.md"), join(staging, "README.md"))
cpSync(join(root, "LICENSE"), join(staging, "LICENSE"))

// `type: module` is not optional. The compiled server is ESM, and without this
// Node reads a bare .js under the bundle as CommonJS and the extension dies at
// launch with a syntax error the user cannot interpret.
writeFileSync(
  join(staging, "package.json"),
  `${JSON.stringify(
    {
      name: pkg.name,
      version: pkg.version,
      type: "module",
      private: true,
      dependencies: pkg.dependencies,
    },
    null,
    2,
  )}\n`,
)

console.log("• installing production dependencies")
run("npm", ["install", "--omit=dev", "--no-audit", "--no-fund", "--silent"], staging)

console.log("• packing")
const output = join(root, "build", `${pkg.name}-${pkg.version}.mcpb`)
run("npx", ["--yes", "@anthropic-ai/mcpb@2", "pack", staging, output], root)

console.log(`\nbundle: ${output}`)
console.log("Now run `npm run mcpb:verify` — packing is not evidence that it launches.")
