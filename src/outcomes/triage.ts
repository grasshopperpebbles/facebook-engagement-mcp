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
      const byPage =
        thread.comment.author?.id === pageId ||
        thread.replies.some((reply) => reply.author?.id === pageId)
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
