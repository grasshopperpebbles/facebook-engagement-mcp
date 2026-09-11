import type { Thread } from "./threads.js"

export type ThreadStatus = "needs_reply" | "answered" | "hidden"

/**
 * Which question the status actually answers.
 *
 * `author_identity` — no reply was authored by the Page. The real question.
 * `reply_count` — nobody replied at all. A weaker, different question, used
 * when Graph did not return `from`. A degraded answer that looks identical to a
 * confident one is the defect this label exists to prevent.
 */
export type StatusBasis = "author_identity" | "reply_count"

export const FILTERS = ["needs_reply", "unanswered", "hidden", "all"] as const
export type Filter = (typeof FILTERS)[number]

export interface TriagedThread extends Thread {
  status: ThreadStatus
}

export function statusBasisFor(threads: Thread[]): StatusBasis {
  const hasAuthor = threads.some(
    (t) => t.comment.author?.id !== undefined || t.replies.some((r) => r.author?.id !== undefined),
  )
  return hasAuthor ? "author_identity" : "reply_count"
}

export function triageThreads(
  threads: Thread[],
  pageId: string | undefined,
): { threads: TriagedThread[]; basis: StatusBasis } {
  // Identity is only usable when we know both the authors and which id is the
  // Page. Missing either means the weaker question, said out loud.
  const basis: StatusBasis = pageId === undefined ? "reply_count" : statusBasisFor(threads)

  const triaged = threads.map((thread): TriagedThread => {
    if (thread.comment.hidden === true) return { ...thread, status: "hidden" }

    if (basis === "author_identity" && pageId !== undefined) {
      // The Page must have the LAST word, not merely a word.
      //
      // This used to be `.some(reply => author === pageId)`, which reads
      // visitor -> Page -> visitor as answered and hides a customer who is
      // waiting. That shape was a hypothetical until 2026-09-11 (T-11), when
      // threads were confirmed against live Graph to go at least three levels
      // deep with correct `parent` pointers. activity.ts states the invariant
      // this triage is built on — over-surface rather than hide a waiting
      // customer — and `.some()` broke it in the one direction that matters.
      //
      // Replies arrive chronologically (`order: "chronological"` in the
      // client), so the last element is the most recent thing said.
      const lastWord = thread.replies.at(-1)
      const byPage =
        lastWord === undefined
          ? thread.comment.author?.id === pageId
          : lastWord.author?.id === pageId
      return { ...thread, status: byPage ? "answered" : "needs_reply" }
    }

    return { ...thread, status: thread.replies.length > 0 ? "answered" : "needs_reply" }
  })

  return { threads: triaged, basis }
}

export function applyFilter(threads: TriagedThread[], filter: Filter): TriagedThread[] {
  switch (filter) {
    case "needs_reply":
    case "unanswered":
      return threads.filter((t) => t.status === "needs_reply")
    case "hidden":
      return threads.filter((t) => t.status === "hidden")
    case "all":
      return threads
  }
}
