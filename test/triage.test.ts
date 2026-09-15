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

  it("marks a thread needing a reply when a visitor spoke after the Page", () => {
    // Verified against live Graph 2026-09-11 (T-11): threads go at least three
    // levels deep, so visitor -> Page -> visitor is a real shape and not a
    // hypothetical. "The Page appears somewhere in this thread" reads that as
    // answered and hides a customer who is waiting — the exact outcome
    // activity.ts says this triage must never produce. The Page has to have
    // the LAST word, not any word.
    const { threads } = triageThreads(
      [
        thread({ id: "c1", author: { id: "u1" } }, [
          { id: "r1", author: { id: pageId } },
          { id: "r2", author: { id: "u1" } },
        ]),
      ],
      pageId,
    )

    expect(threads[0]!.status).toBe("needs_reply")
  })

  it("keeps a thread answered when the Page spoke last", () => {
    const { threads } = triageThreads(
      [
        thread({ id: "c1", author: { id: "u1" } }, [
          { id: "r1", author: { id: "u1" } },
          { id: "r2", author: { id: pageId } },
        ]),
      ],
      pageId,
    )

    expect(threads[0]!.status).toBe("answered")
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

  it("reads the last word off the clock, not off array position (T-36)", () => {
    // The replies are in the order `activity.ts` used to assemble them:
    // every second-level reply, then every third-level one. `rA1` is the Page
    // answering branch A at 12:00 and sits LAST in the array; `rB` is a visitor
    // commenting on branch B at 13:00 and sits in the middle. Whoever spoke
    // last is the visitor, and this used to answer "the Page" — reporting a
    // waiting customer as handled, which is the one direction triage must never
    // fail in.
    const { threads } = triageThreads(
      [
        thread({ id: "c1", author: { id: "u1" }, createdTime: "2026-09-10T10:00:00+0000" }, [
          { id: "rA", author: { id: pageId }, createdTime: "2026-09-10T11:00:00+0000" },
          { id: "rB", author: { id: "u1" }, createdTime: "2026-09-10T13:00:00+0000" },
          { id: "rA1", author: { id: pageId }, createdTime: "2026-09-10T12:00:00+0000" },
        ]),
      ],
      pageId,
    )

    expect(threads[0]!.status).toBe("needs_reply")
  })

  it("keeps the caller's order when Graph timed the replies incompletely", () => {
    // An untimed reply must not sort to one end and change the answer. With no
    // clock to read, the array order is the best available evidence and the
    // last element stands — the pre-T-36 behaviour, kept exactly where it is
    // still the only defensible one.
    const { threads } = triageThreads(
      [
        thread({ id: "c1", author: { id: "u1" } }, [
          { id: "r1", author: { id: "u2" } },
          { id: "r2", author: { id: pageId } },
        ]),
      ],
      pageId,
    )

    expect(threads[0]!.status).toBe("answered")
  })
})

describe("triageThreads without author identity", () => {
  it("refuses to call anything answered when the Page id is known (T-39)", () => {
    // This asserted `answered` until 2026-09-15, and that was the defect.
    // Graph returns `from` for a comment written by a Page and withholds it for
    // one written by a person (T-38), so the Page's own comments are the only
    // dependable author ids in a batch — and a batch with none is a batch where
    // the Page has replied to nothing. Person asks, person answers, Page has
    // still never spoken. `needs_reply` is the accurate answer here, not the
    // cautious one.
    const { threads, basis } = triageThreads([thread({ id: "c1" }, [{ id: "r1" }])], "pg1")

    expect(basis).toBe("reply_count")
    expect(threads[0]!.status).toBe("needs_reply")
  })

  it("does not report a waiting customer as handled when only people spoke (T-39)", () => {
    // The shape this fix exists for, written the way it actually arrives: two
    // people talking to each other under a Page post, the Page absent. Neither
    // comment carries an author, because neither was written by a Page. Before
    // T-39 this came back `answered` — T-11's inversion by a third route, and
    // it arrived exactly when the Page was behind on everything, which is the
    // case the tool exists for.
    const { threads } = triageThreads(
      [
        thread({ id: "c1", createdTime: "2026-09-15T00:00:00+0000" }, [
          { id: "r1", createdTime: "2026-09-15T01:00:00+0000" },
          { id: "r2", createdTime: "2026-09-15T02:00:00+0000" },
        ]),
      ],
      "pg1",
    )

    expect(threads[0]!.status).toBe("needs_reply")
  })

  it("keeps identity triage as soon as ONE Page comment is present", () => {
    // The boundary. A single Page-authored comment anywhere in the batch is
    // enough to restore `author_identity`, and then the ordinary rule applies
    // per thread — including calling a thread answered when the Page really did
    // have the last word. T-39 must not have made `answered` unreachable.
    const { basis, threads } = triageThreads(
      [
        thread({ id: "c1", createdTime: "2026-09-15T00:00:00+0000" }, [
          { id: "r1", author: { id: "pg1" }, createdTime: "2026-09-15T01:00:00+0000" },
        ]),
        thread({ id: "c2", createdTime: "2026-09-15T00:00:00+0000" }, [
          { id: "r2", createdTime: "2026-09-15T01:00:00+0000" },
        ]),
      ],
      "pg1",
    )

    expect(basis).toBe("author_identity")
    expect(threads.map((t) => t.status)).toEqual(["answered", "needs_reply"])
  })

  it("still answers 'somebody replied' when there is no Page id at all", () => {
    // Nothing can identify anyone here, so "did anyone reply" is the only
    // question the data supports. Unchanged by T-39.
    const { threads, basis } = triageThreads([thread({ id: "c1" }, [{ id: "r1" }])], undefined)

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
