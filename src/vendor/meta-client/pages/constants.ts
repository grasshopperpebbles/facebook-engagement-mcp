/**
 * Field selections for the Pages surface.
 *
 * No API version string appears here — the pin lives in `../constants.ts` and
 * nowhere else.
 *
 * Unverified against a live Page as of 2026-09-02: `from` may not be returned
 * for comments authored by someone other than the Page, and `can_hide` is not
 * listed in the v26.0 Comment node reference. Both are optional in the
 * normalized shape, so absence degrades rather than breaks.
 */

export const PAGE_FIELDS = ["id", "name", "category", "tasks"] as const

/** `access_token` is requested only by `pages.credentials()`, never by `pages.list()`. */
export const PAGE_CREDENTIAL_FIELDS = ["id", "access_token", "tasks"] as const

export const POST_FIELDS = [
  "id",
  "message",
  "created_time",
  "permalink_url",
  "is_published",
] as const

export const COMMENT_FIELDS = [
  "id",
  "message",
  "created_time",
  "from",
  "like_count",
  "comment_count",
  "is_hidden",
  "can_comment",
  "can_hide",
  "permalink_url",
  "parent",
] as const
