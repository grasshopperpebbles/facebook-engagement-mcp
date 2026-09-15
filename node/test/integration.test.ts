import { describe, expect, it } from "vitest"
import { createEnvTokenProvider } from "../src/auth/token-provider.js"
import { runCommentActivity } from "../src/outcomes/activity.js"
import { runModerateComment } from "../src/tools/moderate-comment.js"
import { createPagesClient } from "../src/vendor/meta-client/index.js"

/**
 * Live tests against a real Facebook Page.
 *
 * Two separate opt-ins, deliberately. META_INTEGRATION_TESTS enables reads.
 * Writes need META_INTEGRATION_WRITE_TESTS as well, because a write test
 * against a real Page is visible to that Page's real audience.
 */
const readsEnabled = process.env.META_INTEGRATION_TESTS === "true"
const writesEnabled = readsEnabled && process.env.META_INTEGRATION_WRITE_TESTS === "true"

const token = process.env.META_ACCESS_TOKEN ?? ""
const pageId = process.env.META_INTEGRATION_PAGE_ID ?? ""
const commentId = process.env.META_INTEGRATION_COMMENT_ID ?? ""

const deps = () => ({
  client: createPagesClient({ accessToken: token }),
  tokens: createEnvTokenProvider({ userToken: token }),
})

describe.skipIf(!readsEnabled)("live reads", () => {
  it("lists Pages this identity administers", async () => {
    const result = await runCommentActivity(deps(), {})

    expect(result).toHaveProperty("pages")
    expect((result as { pages: unknown[] }).pages.length).toBeGreaterThan(0)
  })

  it("reads comment activity for the configured Page", async () => {
    expect(pageId, "set META_INTEGRATION_PAGE_ID").not.toBe("")
    const result = await runCommentActivity(deps(), { page: pageId, filter: "all" })

    expect(result).not.toHaveProperty("error")
  })

  it("records whether Graph returns comment authors", async () => {
    // The unverified question from spec §4. This test does not assert an
    // outcome — it reports one, so the answer stops being a guess.
    expect(pageId, "set META_INTEGRATION_PAGE_ID").not.toBe("")
    const result = await runCommentActivity(deps(), { page: pageId, filter: "all" })
    const basis = (result as { statusBasis?: string }).statusBasis

    console.error(`[integration] statusBasis for ${pageId}: ${String(basis)}`)
    expect(["author_identity", "reply_count"]).toContain(basis)
  })

  it("returns no access token in any live response", async () => {
    const result = await runCommentActivity(deps(), {})

    expect(JSON.stringify(result)).not.toContain(token)
  })
})

describe.skipIf(!writesEnabled)("live writes", () => {
  it("hides and then unhides a comment, leaving it as it was found", async () => {
    expect(commentId, "set META_INTEGRATION_COMMENT_ID").not.toBe("")

    let hidden: unknown
    let restored: unknown

    try {
      hidden = await runModerateComment(deps(), { commentId, action: "hide", pageId })
      expect(hidden).toMatchObject({ ok: true })
    } finally {
      // Restore even if the assertion above failed. A comment left hidden by a
      // failed test is a visible change to someone else's Page.
      restored = await runModerateComment(deps(), { commentId, action: "unhide", pageId })
    }

    expect(restored).toMatchObject({ ok: true })
  })
})

describe("integration test gating", () => {
  it("cannot enable live writes without also enabling live reads", () => {
    // Writes are gated behind reads deliberately: a write test against a real
    // Page is visible to that Page's real audience. This holds in every
    // environment, including CI where neither variable is set.
    expect(writesEnabled && !readsEnabled).toBe(false)
  })
})
