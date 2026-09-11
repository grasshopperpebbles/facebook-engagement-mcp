/**
 * Structural types for the Page resources this client reads.
 *
 * Not runtime validated, matching the Marketing surface: Meta adds fields
 * freely, and validating responses would break on harmless additions. Every
 * field but `id` is optional because Graph omits rather than nulls anything the
 * token lacks permission to see.
 */

export interface RawPage {
  id: string
  name?: string
  category?: string
  tasks?: string[]
  /** Credential material. Never leaves the client except via `pages.credentials()`. */
  access_token?: string
}

export interface RawPost {
  id: string
  message?: string
  created_time?: string
  permalink_url?: string
  is_published?: boolean
}

export interface RawComment {
  id: string
  message?: string
  created_time?: string
  from?: { id?: string; name?: string }
  like_count?: number
  comment_count?: number
  is_hidden?: boolean
  can_comment?: boolean
  can_hide?: boolean
  permalink_url?: string
  parent?: { id?: string }
}

export interface Page {
  id: string
  name?: string
  category?: string
  /** Page roles this token holds, e.g. MODERATE, ANALYZE, CREATE_CONTENT. */
  tasks?: string[]
}

/** A Page access token and the tasks it carries. Treat as a secret. */
export interface PageCredential {
  pageId: string
  accessToken: string
  /**
   * Page roles this token holds, e.g. MODERATE.
   *
   * **Absent and empty are different answers.** Undefined means Graph did not
   * return the field, so what this identity may do is unknown; `[]` means Graph
   * returned it empty, so the identity holds no role. Callers that refuse a
   * write on a missing role must act on the second and not the first — see the
   * MODERATE guard in `page-engagement`'s `outcomes/errors.ts`.
   */
  tasks?: string[]
}

/**
 * What `/me/accounts` yielded, with the two outcomes kept apart.
 *
 * A Page can appear in that listing and still carry no `access_token`. Folding
 * those rows away made them indistinguishable from Pages the listing never
 * mentioned, and the caller then reported the wrong cause for both.
 */
export interface PageCredentials {
  /** Pages that yielded a token. Credential material. */
  usable: PageCredential[]
  /** Page ids Graph listed and withheld a token for. Carries no secret. */
  tokenless: string[]
}

export interface PagePost {
  id: string
  message?: string
  createdTime?: string
  permalink?: string
  /**
   * False for unpublished posts, which are the object type behind ads. Defaults
   * to true when Graph omits the field — an absent value must not be read as
   * "this is an ad post".
   */
  isPublished: boolean
}

/**
 * A comment, normalized.
 *
 * `author` is absent, not empty, when Graph does not return `from`. Whether
 * `from` is returned for non-Page authors is unverified — see the design spec
 * §4. Consumers must handle its absence rather than assume it.
 */
export interface Comment {
  id: string
  message?: string
  createdTime?: string
  author?: { id?: string; name?: string }
  likeCount?: number
  replyCount?: number
  hidden?: boolean
  /** Whether a reply can be posted to this comment. Graph's `can_comment`. */
  canComment?: boolean
  canHide?: boolean
  permalink?: string
  parentId?: string
}
