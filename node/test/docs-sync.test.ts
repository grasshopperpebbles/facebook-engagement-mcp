import { spawnSync } from "node:child_process"
import { existsSync } from "node:fs"
import { join, resolve } from "node:path"
import { describe, expect, it } from "vitest"

/**
 * `docs/` is generated from the broadkast articles. This fails when it drifts.
 *
 * **Why it is not simply a diff.** Three documents live twice — once in the
 * content repo that publishes to the website, once in `docs/` so the README can
 * link to something that renders on GitHub without depending on a site being
 * live. Nothing held them in step, and on 2026-09-15 they had drifted in BOTH
 * directions at once: the source article was missing a finding its own
 * derivative carried, and the derivative was missing a summary bullet the source
 * had. A manual diff found it, and only because somebody happened to ask.
 *
 * **It skips when the content repo is absent, and says so.** That is the
 * ordinary state of a clone that has only this repository, and of CI, which
 * cannot check out a private sibling. It is the same choice the conformance
 * runner makes for `gpp-mcp-page-engagement`: skipping quietly would let a run
 * test less than it appears to, so the skip is announced.
 *
 * Skipping is not a loophole. This check exists for the machine where both
 * repositories are checked out, which is the only machine where the drift can
 * be introduced in the first place.
 *
 * It drives the script rather than reimplementing it, so what is asserted is
 * what a person runs.
 */

const repoRoot = join(import.meta.dirname, "..", "..")
const script = join(repoRoot, "scripts", "sync-docs.mjs")

/** Mirrors DEFAULT_SOURCE in the script. Kept here only to decide skip vs run. */
const contentRepo = resolve(
  repoRoot,
  "..",
  "broadkast",
  "docs",
  "my social media posts",
  "Founder_Ecosystem_Command_Center Rewrite",
  "GrasshopperPebbles",
  "grasshopperpebbles articles",
  "facebook-engagement-mcp-setup",
)

const available = existsSync(join(contentRepo, "article.md"))

describe("docs/ is generated, not edited", () => {
  it("has the generator it claims to have", () => {
    // The guard on the guard: if the script is renamed or removed, the check
    // below would skip forever and report nothing, which is the failure mode
    // this repository has recorded twice.
    expect(existsSync(script), `${script} is missing`).toBe(true)
  })

  it.skipIf(!available)("matches the articles it is generated from", () => {
    const result = spawnSync(process.execPath, [script, "--check"], {
      cwd: repoRoot,
      encoding: "utf8",
    })

    // A non-zero exit carries the list of drifted documents and what to do
    // about it, so the script's own message is the failure message.
    expect(result.stderr + result.stdout).toBeTruthy()
    expect(result.status, `\n${result.stderr}${result.stdout}`).toBe(0)
  })

  it.runIf(!available)("says out loud that it could not check", () => {
    // Reached only when the content repo is absent. It asserts nothing about
    // the docs — it exists so the run shows a line about this rather than
    // silently testing one thing less than it looks like it did.
    console.warn(
      `docs/ sync NOT CHECKED: content repo not found at ${contentRepo}. ` +
        "This is expected in CI and in a clone of this repository alone.",
    )
    expect(available).toBe(false)
  })
})
