import type { Comment } from "../vendor/meta-client/index.js"

/** Declared truncation limit for free text (architecture §7). */
export const MAX_COMMENT_CHARS = 1000

/**
 * Declared truncation limit for a display name.
 *
 * A Facebook display name is free text chosen by whoever wrote the comment, so
 * it is exactly as untrusted as the comment body and gets a hard ceiling of its
 * own. Eighty characters is past any real name and far short of a paragraph of
 * smuggled instructions.
 */
export const MAX_AUTHOR_NAME_CHARS = 80

/**
 * Total free text one response may carry, across every post body, comment and
 * reply in it.
 *
 * The differentiation argument for this server is context economy: triaging
 * 400 comments server-side is supposed to cost the groups, not the comments.
 * Without a ceiling a default sweep of a busy Page returns a quarter of a
 * megabyte, which costs the caller both. Threads past the budget are returned
 * with their structure and without their text, and the answer says how many.
 */
export const MAX_RESPONSE_TEXT_CHARS = 40_000

export interface UntrustedText {
  /** Always true. A structural marker that this value came from a stranger. */
  untrusted: true
  value: string
  truncated: boolean
}

/**
 * A comment's author.
 *
 * `id` is a Graph identifier — not free text, and load-bearing for triage, so
 * it is returned as-is. `name` is attacker-chosen and gets the same delimited,
 * truncated treatment as the comment body.
 */
export interface RenderedAuthor {
  id?: string
  name?: UntrustedText
}

export type RenderedComment = Omit<Comment, "message" | "author"> & {
  text?: UntrustedText
  author?: RenderedAuthor
}

/** Wrap free text in its marker, truncating at a declared limit. */
export function untrustedText(value: string, limit = MAX_COMMENT_CHARS): UntrustedText {
  return { untrusted: true, value: value.slice(0, limit), truncated: value.length > limit }
}

/**
 * A drawdown allowance shared across everything rendered into one response.
 *
 * `take` is all-or-nothing on purpose: half a comment body is not a useful
 * thing to return, and a partially-spent thread would have to be described as
 * both complete and abbreviated.
 */
export interface TextBudget {
  /** Characters still unspent. */
  readonly remaining: number
  /** Spend `cost` if the whole amount is available; false when it is not. */
  take(cost: number): boolean
}

export function createTextBudget(total: number = MAX_RESPONSE_TEXT_CHARS): TextBudget {
  let remaining = Math.max(0, total)
  return {
    get remaining() {
      return remaining
    },
    take(cost: number): boolean {
      if (cost > remaining) return false
      remaining -= cost
      return true
    },
  }
}

/** Characters `renderComment` would spend on this comment. */
export function commentTextCost(comment: Comment): number {
  const body =
    comment.message === undefined ? 0 : Math.min(comment.message.length, MAX_COMMENT_CHARS)
  const name =
    comment.author?.name === undefined
      ? 0
      : Math.min(comment.author.name.length, MAX_AUTHOR_NAME_CHARS)
  return body + name
}

/**
 * Move every piece of comment free text into labelled, delimited fields.
 *
 * Comment text and author names are written by the public and this server also
 * holds tools that publish and hide. Structural separation does not stop a
 * model from being persuaded by content it legitimately reads — that
 * non-defence is documented in the README — but it stops the text being
 * mistaken for structure.
 */
export function renderComment(comment: Comment): RenderedComment {
  const { message, author, ...rest } = comment

  return {
    ...rest,
    ...(message !== undefined && { text: untrustedText(message) }),
    ...(author !== undefined && {
      author: {
        ...(author.id !== undefined && { id: author.id }),
        ...(author.name !== undefined && {
          name: untrustedText(author.name, MAX_AUTHOR_NAME_CHARS),
        }),
      },
    }),
  }
}

/**
 * The same comment with all of its free text withheld.
 *
 * Used once a response has spent its text budget: the caller still gets the
 * id, status, counts, timestamps and author id — enough to count, group and
 * act on the thread, or to fetch it specifically — without the body.
 */
export function abbreviateComment(comment: Comment): RenderedComment {
  const { message, author, ...rest } = comment
  void message

  return {
    ...rest,
    ...(author?.id !== undefined && { author: { id: author.id } }),
  }
}
