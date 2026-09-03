import { describe, expect, it } from "vitest"
import type { Thread } from "../src/outcomes/threads.js"
import { applyFilter, statusBasisFor, triageThreads } from "../src/outcomes/triage.js"
import type { Comment } from "../src/vendor/meta-client/index.js"

const post = { id: "p1", isPublished: true }
const thread = (comment: Comment, replies: Comment[] = []): Thread => ({ comment, replies, post })

describe("statusBasisFor", () => {
  it("uses author identity when any comment carries an author", () => {
    expect(statusBasisFor([thread({ id: "c1", author: { id: "u1" } })])).toBe("author_identity")
  })

  it("falls back to reply count when no comment carries an author", () => {
    expect(statusBasisFor([thread({ id: "c1" })])).toBe("reply_count")
  })

  it("falls back on an empty set rather than claiming identity", () => {
    expect(statusBasisFor([])).toBe("reply_count")
  })
})

describe("triageThreads with author identity", () => {
  const pageId = "pg1"

  it("marks a thread answered when the Page replied", () => {
    const { threads, basis } = triageThreads(
      [thread({ id: "c1", author: { id: "u1" } }, [{ id: "r1", author: { id: pageId } }])],
      pageId,
    )

    expect(basis).toBe("author_identity")
    expect(threads[0]!.status).toBe("answered")
  })

  it("marks a thread needing a reply when only other users replied", () => {
    // The distinction the degraded path cannot make: replies exist, but none
    // are the Page's, so the customer is still waiting.
    const { threads } = triageThreads(
      [thread({ id: "c1", author: { id: "u1" } }, [{ id: "r1", author: { id: "u2" } }])],
      pageId,
    )

    expect(threads[0]!.status).toBe("needs_reply")
  })

  it("marks a hidden comment hidden regardless of replies", () => {
    const { threads } = triageThreads(
      [
        thread({ id: "c1", author: { id: "u1" }, hidden: true }, [
          { id: "r1", author: { id: pageId } },
        ]),
      ],
      pageId,
    )

    expect(threads[0]!.status).toBe("hidden")
  })

  it("never marks the Page's own comment as needing a reply", () => {
    const { threads } = triageThreads([thread({ id: "c1", author: { id: pageId } })], pageId)

    expect(threads[0]!.status).toBe("answered")
  })
})

describe("triageThreads without author identity", () => {
  it("treats any reply as answered and says the basis was reply count", () => {
    const { threads, basis } = triageThreads([thread({ id: "c1" }, [{ id: "r1" }])], "pg1")

    expect(basis).toBe("reply_count")
    expect(threads[0]!.status).toBe("answered")
  })

  it("treats a reply-less comment as needing a reply", () => {
    const { threads } = triageThreads([thread({ id: "c1" })], "pg1")

    expect(threads[0]!.status).toBe("needs_reply")
  })

  it("degrades when the Page id is unknown, even if authors exist", () => {
    const { basis, threads } = triageThreads(
      [thread({ id: "c1", author: { id: "u1" } }, [{ id: "r1", author: { id: "u2" } }])],
      undefined,
    )

    expect(basis).toBe("reply_count")
    expect(threads[0]!.status).toBe("answered")
  })
})

describe("applyFilter", () => {
  const triaged = [
    { ...thread({ id: "c1" }), status: "needs_reply" as const },
    { ...thread({ id: "c2" }), status: "answered" as const },
    { ...thread({ id: "c3" }), status: "hidden" as const },
  ]

  it("selects only threads needing a reply", () => {
    expect(applyFilter(triaged, "needs_reply").map((t) => t.comment.id)).toEqual(["c1"])
  })

  it("treats `unanswered` as a synonym of needs_reply", () => {
    expect(applyFilter(triaged, "unanswered").map((t) => t.comment.id)).toEqual(["c1"])
  })

  it("selects only hidden threads", () => {
    expect(applyFilter(triaged, "hidden").map((t) => t.comment.id)).toEqual(["c3"])
  })

  it("passes everything through on `all`", () => {
    expect(applyFilter(triaged, "all")).toHaveLength(3)
  })
})
