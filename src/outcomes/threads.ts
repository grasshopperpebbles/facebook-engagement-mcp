import type { Comment, PagePost } from "../vendor/meta-client/index.js"

export interface ThreadPost {
  id: string
  /**
   * False for unpublished, ad-backed posts; true for published ones.
   *
   * **Absent when it is not known.** Only a `/feed` sweep reads the post node,
   * so a post reached by id — a `post`, `ad` or `campaign` target — has no
   * published state to report, and an asserted default would mislabel a
   * boosted published post as ad-backed and an ad post fetched by id as
   * organic. Omission says "not checked"; it never means "published".
   */
  isPublished?: boolean
  message?: string
  permalink?: string
}

export interface Thread {
  comment: Comment
  replies: Comment[]
  post: ThreadPost
}

export function threadPost(post: PagePost): ThreadPost {
  return {
    id: post.id,
    isPublished: post.isPublished,
    ...(post.message !== undefined && { message: post.message }),
    ...(post.permalink !== undefined && { permalink: post.permalink }),
  }
}

/**
 * Milliseconds for a Graph timestamp such as `2026-08-30T10:00:00+0000`, or
 * `undefined` when it is missing or unparseable.
 *
 * Never throws. A malformed timestamp costs one comment its place in the
 * ordering, never the whole answer.
 */
export function commentTime(comment: Comment): number | undefined {
  if (comment.createdTime === undefined) return undefined
  const parsed = Date.parse(comment.createdTime)
  return Number.isNaN(parsed) ? undefined : parsed
}

/**
 * Comments in the order they were written.
 *
 * **Stable, and deliberately so.** A comment Graph timed incompletely keeps its
 * incoming position rather than sorting to one end: degrading to the order the
 * caller supplied is defensible, inventing an order is not.
 *
 * This exists because a thread is assembled from more than one Graph call and
 * the concatenation is not chronological — see `lastWord` and T-36.
 */
export function inTimeOrder(comments: Comment[]): Comment[] {
  return comments
    .map((comment, index) => ({ comment, index }))
    .sort((a, b) => {
      const left = commentTime(a.comment)
      const right = commentTime(b.comment)
      if (left === undefined || right === undefined) return a.index - b.index
      return left === right ? a.index - b.index : left - right
    })
    .map((entry) => entry.comment)
}

/**
 * The most recent thing said in this thread — the comment itself when nobody
 * has replied.
 *
 * **By the clock, not by array position, and that distinction is the whole of
 * T-36.** Triage asks who spoke last, and until 2026-09-14 it answered with
 * `replies.at(-1)`. That is the same question only while the array is in time
 * order, and `activity.ts` builds it by concatenating every second-level reply
 * with every third-level one: give a thread two branches and the Page's older
 * answer on the first lands last, behind a visitor's newer comment on the
 * second. The thread then reports `answered` with a customer waiting.
 *
 * That is exactly the inversion T-11 fixed on 2026-09-11, in the same
 * direction, inside the code T-11's fix was written into — because a fix
 * inherits the invariants of the code it lands in, and this one was never
 * written down. So the ordering is no longer assumed anywhere: `activity.ts`
 * sorts what it assembles, and this answers from the timestamps regardless,
 * because a caller that builds a `Thread` by hand must not be able to invert
 * the product's only answer by listing replies in the wrong order.
 */
export function lastWord(thread: Thread): Comment {
  return inTimeOrder(thread.replies).at(-1) ?? thread.comment
}

export function assembleThreads(input: {
  post: ThreadPost
  comments: Comment[]
  repliesByCommentId: Map<string, Comment[]>
}): Thread[] {
  const { post } = input
  return input.comments.map((comment) => ({
    comment,
    replies: input.repliesByCommentId.get(comment.id) ?? [],
    post,
  }))
}
