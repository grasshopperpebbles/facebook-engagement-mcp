import { MetaApiError } from "../vendor/meta-client/index.js"

const PERMISSION_BY_OPERATION: Record<string, string> = {
  read: "pages_read_engagement and pages_read_user_content",
  "ad read": "ads_read",
  reply: "pages_manage_engagement",
  moderate: "pages_manage_engagement",
}

/**
 * Turn a Graph failure into something the caller can act on.
 *
 * A bare "(#200) Permissions error" is the most common failure this server's
 * audience hits and the most expensive to debug: it does not say which
 * permission, whether App Review is the blocker, or whether the Page role is
 * simply missing.
 */
export function explainGraphError(
  error: unknown,
  context: { operation: string; pageId?: string; tasks?: string[] },
): string {
  if (!(error instanceof MetaApiError)) {
    return error instanceof Error ? error.message : "Unexpected error talking to Meta."
  }

  const trace = error.fbtraceId ? ` (Meta trace ${error.fbtraceId})` : ""

  if (error.isAuthError) {
    // Deliberately concrete. "Re-authenticate" reads as an OAuth flow, and this
    // server has none — a person pastes a token into the client's config — so
    // that wording sends a caller looking for a reconnect button that does not
    // exist. Observed happening on 2026-09-08.
    return (
      "The Meta access token has expired or been revoked, so this call was refused before it " +
      "reached the Page. Mint a new user token and set it as META_ACCESS_TOKEN wherever this " +
      "server is configured — in Claude Desktop that is Settings, Extensions, this extension, " +
      "Configure. A token copied from the Graph API Explorer lasts about an hour; exchange it " +
      `for a long-lived one to get about 60 days.${trace}`
    )
  }

  if (error.isThrottled) {
    return `Meta rate limited this request. Back off before retrying.${trace}`
  }

  if (error.code === 200 || error.status === 403) {
    const needsModerate = context.operation !== "read"
    if (needsModerate && context.tasks !== undefined && !context.tasks.includes("MODERATE")) {
      return (
        `This token has no MODERATE task on Page ${context.pageId ?? "(unknown)"}, so it cannot ` +
        `reply to or hide comments. Grant that role in Page settings and re-issue the token.${trace}`
      )
    }
    const permission = PERMISSION_BY_OPERATION[context.operation] ?? "the relevant Page permission"
    // Meta's own message is kept, always. It is often misleading — a refused
    // reply comes back naming `publish_actions`, deprecated since 2018 and not
    // the permission involved — but it is the only thing that distinguishes one
    // refusal from another, and a caller who cannot see it debugs the wrong
    // thing. Observed happening 2026-09-08.
    return (
      `Meta refused this ${context.operation} for lack of permission. It requires ${permission}, ` +
      "which requires App Review for Pages outside your own development-mode app. " +
      `Meta's own words, which may name a different or deprecated permission: "${error.message}"${trace}`
    )
  }

  return `${error.message}${trace}`
}
