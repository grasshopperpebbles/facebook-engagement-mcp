import { existsSync, readdirSync, readFileSync } from "node:fs"
import { dirname, join, relative, resolve } from "node:path"
import { describe, expect, it } from "vitest"

/**
 * Every relative link in every Markdown file resolves to something that exists.
 *
 * **This exists because of T-43 and because the check next to it does not cover
 * it.** `mcpb.test.ts` resolves the links inside `mcpb/manifest.json`, which is
 * four URLs. It says nothing about the hundreds of relative links between the
 * README and `docs/`, and the folder-per-language move broke six of those in one
 * commit by relocating the README one directory deeper.
 *
 * The spec for that move first claimed the manifest test would catch the
 * breakage. It would not have: the manifest's links are `blob/main/docs/…`,
 * addressed from the repository root, and `docs/` never moved. **A check that
 * covers a different set of links than you think it covers is the same failure
 * as a check that cannot fail** — it reports green about a question nobody
 * asked.
 *
 * Offline and path-based on purpose. This project has shipped two links that
 * pointed at nothing, and once adopted an HTTP check that could not fail because
 * the host answered 200 for every path including deliberate nonsense. A file
 * either exists in the working tree or it does not, and that is a check whose
 * failure mode needs no network to observe.
 *
 * **Repository-wide, though it lives in the Node package**, because `node/` is
 * the only thing here with a test runner today. It moves to a repo-level runner
 * when `conformance/` brings one.
 */

const repoRoot = join(import.meta.dirname, "..", "..")
const SKIP = new Set(["node_modules", "dist", "build", ".git", "coverage"])

function markdownFiles(dir: string): string[] {
  const found: string[] = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (SKIP.has(entry.name)) continue
    const full = join(dir, entry.name)
    if (entry.isDirectory()) found.push(...markdownFiles(full))
    else if (entry.name.endsWith(".md")) found.push(full)
  }
  return found
}

/**
 * Link targets that point at a path in this repository.
 *
 * Absolute URLs, mailto and pure `#anchor` links are somebody else's problem —
 * the first two cannot be checked offline and the third is within one document.
 * A trailing `#anchor` is stripped: the file has to exist, and whether the
 * heading does is not something a path check can answer honestly.
 */
function localTargets(markdown: string): string[] {
  return [...markdown.matchAll(/\]\(([^)\s]+)\)/g)]
    .map((match) => match[1] ?? "")
    .filter((target) => !/^(https?:|mailto:|#)/.test(target))
    .map((target) => target.split("#")[0] ?? "")
    .filter((target) => target.length > 0)
}

const files = markdownFiles(repoRoot)

describe("relative links in Markdown", () => {
  it("finds Markdown to check, so an empty sweep cannot pass silently", () => {
    // The obvious way for this whole file to stop meaning anything is for the
    // walk to return nothing and every assertion below to vacuously hold.
    expect(files.length).toBeGreaterThan(5)
    expect(localTargets(readFileSync(join(repoRoot, "README.md"), "utf8")).length).toBeGreaterThan(
      5,
    )
  })

  it.each(files.map((file) => [relative(repoRoot, file), file]))(
    "%s links only to files that exist",
    (_label, file) => {
      const targets = localTargets(readFileSync(file, "utf8"))
      const broken = targets.filter((target) => !existsSync(resolve(dirname(file), target)))

      expect(broken, `broken links in ${relative(repoRoot, file)}`).toEqual([])
    },
  )
})
