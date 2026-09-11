import {
  createPagesClient,
  type FetchImpl,
  type PageCredentials,
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
  /**
   * Page roles the token carries, e.g. MODERATE.
   *
   * **Undefined when unknown, `[]` when known to be none.** Graph omits the
   * field rather than returning it empty when it will not say, and a caller
   * that refuses a write on a missing role must act on `[]` and never on
   * undefined — refusing on silence produces a message that names a role the
   * identity may well hold.
   */
  tasksFor(pageId: string): Promise<string[] | undefined>
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
  let pending: Promise<PageCredentials> | undefined

  async function credentials(): Promise<PageCredentials> {
    pending ??= client.pages.credentials().catch((cause: unknown) => {
      pending = undefined // a failed exchange must not poison the cache
      throw cause
    })
    return pending
  }

  /**
   * Two failures, told apart.
   *
   * Graph can list a Page and withhold its `access_token`, and that is not the
   * same fault as Graph never listing the Page. This used to report the second
   * for both, sending a reader to check administration and `pages_show_list`
   * for a case that may be neither — which is how a Business Manager System
   * User is expected to arrive here if the asset assignment is wrong (T-20).
   */
  async function credentialFor(pageId: string) {
    const { usable, tokenless } = await credentials()
    const found = usable.find((c) => c.pageId === pageId)
    if (found) return found
    if (tokenless.includes(pageId)) {
      throw new PageAccessError(
        `Graph listed Page ${pageId} but returned no access token for it. The identity reaches ` +
          "the Page and cannot act as it: check that this Page is assigned to the identity with " +
          "a Page role, not merely visible to it.",
      )
    }
    throw new PageAccessError(
      `No Page access token for ${pageId}. This identity may not administer that Page, ` +
        "or the token may lack pages_show_list.",
    )
  }

  return {
    forUser: async () => options.userToken,
    forPage: async (pageId) => (await credentialFor(pageId)).accessToken,
    // Spread a copy, because the cached credential is shared and a caller that
    // received the live array could push a role into it. `undefined` passes
    // through as itself — it is an answer, not a missing one.
    tasksFor: async (pageId) => {
      const { tasks } = await credentialFor(pageId)
      return tasks === undefined ? undefined : [...tasks]
    },
  }
}
