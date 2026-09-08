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
    return `The access token is invalid or expired; re-authenticate and retry.${trace}`
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
    return (
      `Meta refused this ${context.operation} for lack of permission. It requires ${permission}, ` +
      `which requires App Review for Pages outside your own development-mode app.${trace}`
    )
  }

  return `${error.message}${trace}`
}
