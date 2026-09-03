import { createTransport, type FetchImpl, type PagedResult, type TokenResolver } from "../http.js"
import { COMMENT_FIELDS, PAGE_CREDENTIAL_FIELDS, PAGE_FIELDS, POST_FIELDS } from "./constants.js"
import { normalizeComment, normalizePage, normalizePost } from "./normalize.js"
import type {
  Comment,
  Page,
  PageCredential,
  PagePost,
  RawComment,
  RawPage,
  RawPost,
} from "./types.js"

export * from "./constants.js"
export * from "./types.js"

export interface ListOptions {
  maxItems?: number
}

export interface PostListOptions extends ListOptions {
  /** ISO date or Unix timestamp. Graph filters the feed server-side. */
  since?: string
}

export interface PagesClient {
  pages: {
    list(options?: ListOptions): Promise<PagedResult<Page>>
    /** Credential material — never log, return through a tool, or cache to disk. */
    credentials(): Promise<PageCredential[]>
  }
  posts: {
    list(pageId: string, options?: PostListOptions): Promise<PagedResult<PagePost>>
  }
  comments: {
    forPost(postId: string, options?: ListOptions): Promise<PagedResult<Comment>>
    get(commentId: string): Promise<Comment>
    replies(commentId: string, options?: ListOptions): Promise<PagedResult<Comment>>
    reply(commentId: string, message: string): Promise<{ id: string }>
    setHidden(commentId: string, hidden: boolean): Promise<{ success: boolean }>
  }
  /** A client bound to a different token — used for per-Page reads. */
  withToken(accessToken: string): PagesClient
}

/**
 * Client for Facebook Page engagement: Pages, their posts, and comments.
 *
 * Reads `/{page-id}/feed` rather than `/posts` or `/published_posts` because
 * only `/feed` returns unpublished posts, which are the object type behind ads.
 * Comments on advertising are unreachable through the published edges.
 */
export function createPagesClient(options: {
  accessToken: string | TokenResolver
  fetchImpl?: FetchImpl
}): PagesClient {
  const transport = createTransport(options)

  const page = <Raw, Out>(
    path: string,
    fields: readonly string[],
    normalize: (raw: Raw) => Out,
    opts: ListOptions = {},
    params: Record<string, string | number | undefined> = {},
  ) =>
    transport
      .getPage<Raw>(path, {
        ...(opts.maxItems !== undefined && { maxItems: opts.maxItems }),
        params: { fields: fields.join(","), ...params },
      })
      .then(({ items, truncated }) => ({ items: items.map(normalize), truncated }))

  return {
    pages: {
      list: (opts) => page<RawPage, Page>("/me/accounts", PAGE_FIELDS, normalizePage, opts),

      credentials: async () => {
        const { items } = await transport.getPage<RawPage>("/me/accounts", {
          params: { fields: PAGE_CREDENTIAL_FIELDS.join(",") },
        })
        return items
          .filter((raw): raw is RawPage & { access_token: string } => Boolean(raw.access_token))
          .map((raw) => ({ pageId: raw.id, accessToken: raw.access_token, tasks: raw.tasks ?? [] }))
      },
    },

    posts: {
      list: (pageId, opts = {}) =>
        page<RawPost, PagePost>(`/${pageId}/feed`, POST_FIELDS, normalizePost, opts, {
          ...(opts.since !== undefined && { since: opts.since }),
        }),
    },

    comments: {
      forPost: (postId, opts) =>
        page<RawComment, Comment>(`/${postId}/comments`, COMMENT_FIELDS, normalizeComment, opts, {
          filter: "toplevel",
          order: "chronological",
        }),

      replies: (commentId, opts) =>
        page<RawComment, Comment>(
          `/${commentId}/comments`,
          COMMENT_FIELDS,
          normalizeComment,
          opts,
          { order: "chronological" },
        ),

      get: async (commentId) =>
        normalizeComment(
          await transport.get<RawComment>(`/${commentId}`, { fields: COMMENT_FIELDS.join(",") }),
        ),

      reply: (commentId, message) =>
        transport.post<{ id: string }>(`/${commentId}/comments`, { message }),

      // Hide and unhide are one endpoint and one boolean, which is why this is
      // one method rather than two.
      setHidden: (commentId, hidden) =>
        transport.post<{ success: boolean }>(`/${commentId}`, { is_hidden: String(hidden) }),
    },

    withToken: (token) =>
      createPagesClient({
        accessToken: token,
        ...(options.fetchImpl && { fetchImpl: options.fetchImpl }),
      }),
  }
}
