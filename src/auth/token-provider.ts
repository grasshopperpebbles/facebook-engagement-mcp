import {
  createPagesClient,
  type FetchImpl,
  type PageCredential,
} from "../vendor/meta-client/index.js"

/**
 * Resolves the access tokens this server needs.
 *
 * Two scopes exist: the user or system token, and a per-Page token. Comment
 * reads return empty data for a user token, so the Page exchange is required
 * rather than an optimization.
 *
 * This interface is the seam a multi-client deployment plugs into. A
 * `DatabaseTokenProvider` or per-client provider replaces the implementation
 * without touching any service — none of that is built here.
 */
export interface TokenProvider {
  forUser(): Promise<string>
  forPage(pageId: string): Promise<string>
  /** Page roles the token carries, e.g. MODERATE. Empty when unknown. */
  tasksFor(pageId: string): Promise<string[]>
}

/** A Page the current credentials cannot reach. Carries no credential material. */
export class PageAccessError extends Error {
  override readonly name = "PageAccessError"
}

/**
 * Reads the user token from configuration and exchanges it via `/me/accounts`.
 *
 * The credential map is held in a closure and never exposed: the returned
 * object has exactly three methods, so no caller can enumerate tokens.
 */
export function createEnvTokenProvider(options: {
  userToken: string
  fetchImpl?: FetchImpl
}): TokenProvider {
  const client = createPagesClient({
    accessToken: options.userToken,
    ...(options.fetchImpl && { fetchImpl: options.fetchImpl }),
  })

  // A single in-flight promise rather than a resolved map, so concurrent
  // callers share one exchange instead of racing several.
  let pending: Promise<Map<string, PageCredential>> | undefined

  async function credentials(): Promise<Map<string, PageCredential>> {
    pending ??= client.pages
      .credentials()
      .then((list) => new Map(list.map((c) => [c.pageId, c])))
      .catch((cause: unknown) => {
        pending = undefined // a failed exchange must not poison the cache
        throw cause
      })
    return pending
  }

  async function credentialFor(pageId: string): Promise<PageCredential> {
    const found = (await credentials()).get(pageId)
    if (!found) {
      throw new PageAccessError(
        `No Page access token for ${pageId}. This identity may not administer that Page, ` +
          "or the token may lack pages_show_list.",
      )
    }
    return found
  }

  return {
    forUser: async () => options.userToken,
    forPage: async (pageId) => (await credentialFor(pageId)).accessToken,
    tasksFor: async (pageId) => [...(await credentialFor(pageId)).tasks],
  }
}
