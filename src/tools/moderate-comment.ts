import { z } from "zod"
import type { TokenProvider } from "../auth/token-provider.js"
import { explainGraphError } from "../outcomes/errors.js"
import type { PagesClient } from "../vendor/meta-client/index.js"

export const TOOL_NAME = "moderate_comment"
export const ACTIONS = ["hide", "unhide"] as const
export type ModerationAction = (typeof ACTIONS)[number]

export const inputSchema = {
  commentId: z.string().describe("ID of the comment to hide or unhide."),
  action: z
    .enum(ACTIONS)
    .describe("hide removes the comment from public view; unhide restores it."),
  pageId: z
    .string()
    .describe(
      "Page that owns the comment. Required: hiding acts as the Page, which needs a Page " +
        "token, and this is what the server exchanges to get one.",
    ),
  dryRun: z.boolean().default(false).describe("Report the intended change without making it."),
}

export const description =
  "Hide or unhide a Facebook comment. Hiding removes it from public view without " +
  "deleting it, and is reversible in one call with the opposite action. This server " +
  "cannot delete comments."

export interface ModerateOptions {
  commentId: string
  action: ModerationAction
  pageId?: string | undefined
  dryRun?: boolean | undefined
}

export interface ModerateDeps {
  client: PagesClient
  tokens: TokenProvider
}

export async function runModerateComment(
  deps: ModerateDeps,
  options: ModerateOptions,
): Promise<
  | { dryRun: true; intended: { commentId: string; action: ModerationAction } }
  | { ok: true; commentId: string; action: ModerationAction; hidden: boolean }
  | { error: string }
> {
  // Same reason as respond_to_comment: without a pageId this falls through to the
  // user-token client, and Meta refuses a user-token moderation write by naming
  // `publish_actions`, dead since 2018. Checked before the dry run so a dry run
  // does not report that a call would work when it could not.
  if (options.pageId === undefined || options.pageId === "") {
    return {
      error:
        "Hiding a comment acts as the Page, so it needs a Page token. Supply `pageId` for the " +
        "Page that owns this comment. Without it the call would go out as the user, which Meta " +
        "refuses with a message about `publish_actions` — a permission removed in 2018.",
    }
  }

  if (options.dryRun === true) {
    return { dryRun: true, intended: { commentId: options.commentId, action: options.action } }
  }

  const hidden = options.action === "hide"
  let tasks: string[] | undefined
  let client = deps.client
  if (options.pageId !== undefined) {
    try {
      tasks = await deps.tokens.tasksFor(options.pageId)
      client = deps.client.withToken(await deps.tokens.forPage(options.pageId))
    } catch (cause) {
      return { error: cause instanceof Error ? cause.message : "Could not resolve a Page token." }
    }
  }

  try {
    const result = await client.comments.setHidden(options.commentId, hidden)
    if (result.success !== true) {
      // Graph answered 200 with success:false. Reporting that as ok would tell
      // the caller a moderation action happened when it did not.
      return {
        error: `Meta did not confirm the ${options.action} of comment ${options.commentId}.`,
      }
    }
    return { ok: true, commentId: options.commentId, action: options.action, hidden }
  } catch (cause) {
    return {
      error: explainGraphError(cause, {
        operation: "moderate",
        ...(options.pageId !== undefined && { pageId: options.pageId }),
        ...(tasks !== undefined && { tasks }),
      }),
    }
  }
}
