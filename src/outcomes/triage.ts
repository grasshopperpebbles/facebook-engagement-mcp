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
    // answer once T-38 is taken seriously. Graph withholds `from` for a comment
    // written by a PERSON and returns it for one written by a PAGE (confirmed
    // live 2026-09-15 across three people and three Pages, including a person
    // with no role on the Page and no connection to the app). The Page's own
    // comments are therefore the only reliable source of an author id in a
    // batch — so a batch with NO authored comment is a batch in which the Page
    // has not replied to anything.
    //
    // Which makes `needs_reply` here the accurate answer rather than a cautious
    // one: person asks, another person answers, and the Page has still never
    // spoken. The old reading reported that as handled, and it did so exactly
    // when the Page was behind on everything — the situation this tool exists
    // for.
    //
    // The residual assumption is named on purpose: this leans on the Page
    // always carrying `from`. That is 3-for-3 observed and structurally likely,
    // being the token's own identity, but it is still a claim about someone
    // else's system. If it ever fails, this mislabels an answered thread as
    // needing a reply — the harmless direction, and the one activity.ts's
    // invariant asks for.
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
