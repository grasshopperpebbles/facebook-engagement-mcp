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
 * Absolute URLs and mailto are somebody else's problem — they cannot be checked
 * offline.
 *
 * **The anchor half used to be excluded too, and the reason given was wrong.**
 * This comment said whether the heading exists "is not something a path check
 * can answer honestly". It is: GitHub derives a heading's anchor by a published,
 * deterministic rule, and computing it needs no network and no rendering. The
 * exclusion cost four live broken anchors, found 2026-09-15 — three of them
 * same-page `#` links this function did not return at all, so they were never
 * even candidates. Two pointed at headings that had been reworded, one at a
 * heading that had gained a suffix, and one at a section that lives in a
 * different file entirely.
 *
 * That is the same failure the doc comment above describes, one level down: a
 * check that declines to cover something, for a reason nobody re-examined.
 */
function localTargets(markdown: string): string[] {
  return [...markdown.matchAll(/\]\(([^)\s]+)\)/g)]
    .map((match) => match[1] ?? "")
    .filter((target) => !/^(https?:|mailto:|#)/.test(target))
    .map((target) => target.split("#")[0] ?? "")
    .filter((target) => target.length > 0)
}

/** Every link target carrying a `#fragment`, including same-page `#` links. */
function anchorTargets(markdown: string): string[] {
  return [...markdown.matchAll(/\]\(([^)\s]+)\)/g)]
    .map((match) => match[1] ?? "")
    .filter((target) => !/^(https?:|mailto:)/.test(target))
    .filter((target) => target.includes("#"))
}

/**
 * The anchors GitHub generates for a Markdown file's headings.
 *
 * The rule: lowercase, drop everything that is not a word character, whitespace
 * or hyphen, then replace each space with one hyphen. **Each space, not each
 * run** — an em dash sits between two spaces, is dropped as punctuation, and
 * leaves the double hyphen that `from-source--developers-only` carries. Getting
 * that wrong reports a correct link as broken, which is how this check was
 * first written and immediately disbelieved.
 *
 * A repeated heading gets `-1`, `-2` … appended, same as GitHub.
 */
function headingAnchors(markdown: string): Set<string> {
  const seen = new Map<string, number>()
  const anchors = new Set<string>()

  for (const match of markdown.matchAll(/^#{1,6}\s+(.*?)\s*$/gm)) {
    const base = (match[1] ?? "")
      .trim()
      .toLowerCase()
      .replace(/[^\w\s-]/g, "")
      .replace(/ /g, "-")
      .replace(/^-+|-+$/g, "")
    const count = seen.get(base) ?? 0
    seen.set(base, count + 1)
    anchors.add(count === 0 ? base : `${base}-${count}`)
  }
  return anchors
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

  it.each(files.map((file) => [relative(repoRoot, file), file]))(
    "%s links only to headings that exist",
    (_label, file) => {
      const markdown = readFileSync(file, "utf8")
      const broken: string[] = []

      for (const target of anchorTargets(markdown)) {
        const [path = "", fragment = ""] = target.split("#")
        if (fragment.length === 0) continue

        const resolved = path.length === 0 ? file : resolve(dirname(file), path)
        // A missing FILE is the other test's finding, not this one's. Reporting
        // it twice would make one fix look like two.
        if (!existsSync(resolved) || !resolved.endsWith(".md")) continue

        if (!headingAnchors(readFileSync(resolved, "utf8")).has(fragment)) broken.push(target)
      }

      expect(broken, `links to missing headings in ${relative(repoRoot, file)}`).toEqual([])
    },
  )

  it("can tell a real anchor from an invented one", () => {
    // The guard on the guard. An anchor check that accepted anything would pass
    // every assertion above and mean nothing — which is this repository's
    // recorded failure mode for link checks specifically: it once adopted an
    // HTTP check that could not fail because the host answered 200 for every
    // path, including deliberate nonsense.
    const sample =
      "# From source — developers only\n\n## Reader downloads (optional — developers)\n"
    const anchors = headingAnchors(sample)

    expect(anchors.has("from-source--developers-only")).toBe(true)
    expect(anchors.has("reader-downloads-optional--developers")).toBe(true)
    // The shape that was actually broken: a heading that has been reworded.
    expect(anchors.has("reader-downloads")).toBe(false)
    expect(anchors.has("from-source-developers-only")).toBe(false)
  })
})
