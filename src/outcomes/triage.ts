import { lastWord, type Thread } from "./threads.js"

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
      // "Last" is by the clock. It used to be `thread.replies.at(-1)`, resting
      // on the replies arriving chronologically — true of any ONE Graph call
      // (`order: "chronological"` in the client) and untrue of the thread
      // activity.ts assembles from several. See `lastWord` and T-36.
      return {
        ...thread,
        status: lastWord(thread).author?.id === pageId ? "answered" : "needs_reply",
      }
    }

    // `reply_count` WITH a known Page: nobody is answered (T-39).
    //
    // This branch used to read `replies.length > 0 ? "answered" : "needs_reply"`
    // — "somebody replied, so it is handled" — and that inverts the product's
    // answer: nothing here can tell whether the somebody was the Page.
    //
    // **The reason recorded here was wrong for a day, and the wrong version is
    // kept because the shape of the error is the useful part.** It read: Graph
    // withholds `from` for a comment written by a PERSON and returns it for one
    // written by a PAGE, confirmed live across three people and three Pages —
    // therefore a batch with no authored comment is one in which the Page has
    // replied to nothing, and `needs_reply` is the ACCURATE answer rather than
    // a cautious one. Every observation in that sample shared one uncontrolled
    // variable: the token. Hours later a personally-granted token for the same
    // app returned `from` for the same person's same comments (T-42). Author
    // identity is not what decides it, and the cause is not established.
    //
    // What the branch rests on now does not need the cause. With no author
    // anywhere in the batch, no thread can be shown to have ended with the
    // Page, so none can be called answered. `needs_reply` for all of them is
    // the over-surfacing direction activity.ts's invariant demands.
    //
    // Say plainly what that costs, since the old comment claimed the opposite:
    // this is a CAUTIOUS answer, not an accurate one. Some of these threads may
    // be handled and are being reported as waiting. That is the harmless
    // direction — the reverse is the defect T-11, T-36 and T-39 each fixed, and
    // it is the one this tool exists to prevent.
    //
    // Option (b) from T-39 stays rejected on the same ground as before: marking
    // these `unknown` reads as the careful choice, and `applyFilter` matches
    // exact status while `needs_reply` is the default filter, so every waiting
    // customer would drop silently out of the default view.
    if (pageId !== undefined) return { ...thread, status: "needs_reply" }

    // No Page id at all: nothing here can identify anyone, and "somebody
    // replied" is the only question the data supports. Unchanged.
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
