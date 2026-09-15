import type { TriagedThread } from "./triage.js"

export const GROUP_BYS = ["post", "author", "status", "day", "none"] as const
export type GroupBy = (typeof GROUP_BYS)[number]

export interface Counts {
  threads: number
  /** Top-level comments plus their replies. */
  comments: number
  needsReply: number
  hidden: number
}

export interface Group {
  key: string
  label: string
  counts: Counts
  threads: TriagedThread[]
}

export function countThreads(threads: TriagedThread[]): Counts {
  return {
    threads: threads.length,
    comments: threads.reduce((sum, t) => sum + 1 + t.replies.length, 0),
    needsReply: threads.filter((t) => t.status === "needs_reply").length,
    hidden: threads.filter((t) => t.status === "hidden").length,
  }
}

/**
 * Group key and human label.
 *
 * Labels never carry untrusted text — not comment bodies and not author
 * display names, which are free text chosen by whoever wrote the comment and
 * would otherwise put attacker-authored strings into a structural field. Only
 * Graph identifiers and this server's own words appear here; the name itself
 * is returned per comment, delimited, by `render/truncate.ts`.
 */
function keyFor(thread: TriagedThread, groupBy: GroupBy): { key: string; label: string } {
  switch (groupBy) {
    case "post": {
      // `isPublished` is absent when the post node was never read; saying
      // nothing beats asserting either state. See ThreadPost.
      const suffix = thread.post.isPublished === false ? " (unpublished — ad-backed)" : ""
      return { key: thread.post.id, label: `Post ${thread.post.id}${suffix}` }
    }
    case "author": {
      const id = thread.comment.author?.id
      if (id === undefined) return { key: "unknown", label: "Author not returned by Graph" }
      return { key: id, label: `Author ${id}` }
    }
    case "status":
      return { key: thread.status, label: thread.status.replace("_", " ") }
    case "day": {
      const day = thread.comment.createdTime?.slice(0, 10)
      return day === undefined
        ? { key: "unknown", label: "Date not returned by Graph" }
        : { key: day, label: day }
    }
    case "none":
      return { key: "all", label: "All threads" }
  }
}

export function groupThreads(threads: TriagedThread[], groupBy: GroupBy): Group[] {
  const buckets = new Map<string, { label: string; threads: TriagedThread[] }>()

  for (const thread of threads) {
    const { key, label } = keyFor(thread, groupBy)
    const bucket = buckets.get(key)
    if (bucket) bucket.threads.push(thread)
    else buckets.set(key, { label, threads: [thread] })
  }

  return (
    [...buckets.entries()]
      .map(([key, { label, threads: grouped }]) => ({
        key,
        label,
        counts: countThreads(grouped),
        threads: grouped,
      }))
      // Most waiting customers first — this is opened to find work.
      .sort(
        (a, b) => b.counts.needsReply - a.counts.needsReply || b.counts.threads - a.counts.threads,
      )
  )
}
