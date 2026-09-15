import { describe, expect, it } from "vitest"
import { assembleThreads } from "../src/outcomes/threads.js"
import type { Comment, PagePost } from "../src/vendor/meta-client/index.js"

const post: PagePost = { id: "p1", isPublished: false, message: "Ad copy" }
const comment = (id: string, extra: Partial<Comment> = {}): Comment => ({ id, ...extra })

describe("assembleThreads", () => {
  it("pairs each comment with its replies", () => {
    const threads = assembleThreads({
      post,
      comments: [comment("c1"), comment("c2")],
      repliesByCommentId: new Map([["c1", [comment("c1_r1")]]]),
    })

    expect(threads.map((t) => [t.comment.id, t.replies.map((r) => r.id)])).toEqual([
      ["c1", ["c1_r1"]],
      ["c2", []],
    ])
  })

  it("carries the post's published state onto every thread", () => {
    // This is what tells an agency that a comment came from an ad rather than
    // an organic post. Losing it loses the server's reason to exist.
    const threads = assembleThreads({
      post,
      comments: [comment("c1")],
      repliesByCommentId: new Map(),
    })

    expect(threads[0]!.post).toEqual({ id: "p1", isPublished: false, message: "Ad copy" })
  })

  it("returns no threads for a post with no comments", () => {
    expect(assembleThreads({ post, comments: [], repliesByCommentId: new Map() })).toEqual([])
  })

  it("ignores replies whose parent comment is absent", () => {
    const threads = assembleThreads({
      post,
      comments: [comment("c1")],
      repliesByCommentId: new Map([["c9", [comment("c9_r1")]]]),
    })

    expect(threads).toHaveLength(1)
    expect(threads[0]!.replies).toEqual([])
  })
})
