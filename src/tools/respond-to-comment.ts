import { z } from "zod"
import type { TokenProvider } from "../auth/token-provider.js"
import { explainGraphError } from "../outcomes/errors.js"
import type { PagesClient } from "../vendor/meta-client/index.js"

export const TOOL_NAME = "respond_to_comment"

export const inputSchema = {
  commentId: z.string().describe("ID of the comment to reply to."),
  message: z.string().min(1).describe("Reply text. Published publicly as the Page."),
  pageId: z
    .string()
    .optional()
    .describe("Page that owns the comment. Supplying it gives clearer permission errors."),
  dryRun: z
    .boolean()
    .default(false)
    .describe("Report what would be published without publishing it."),
  confirmed: z
    .boolean()
    .default(false)
    .describe(
      "Set true to confirm publishing when the client cannot show a confirmation " +
        "prompt. Review a dryRun first.",
    ),
}

export const description =
  "Publish a reply to a Facebook comment as the Page. This is a write: the reply is " +
  "public immediately and deleting it later does not unpublish what people saw. Use " +
  "dryRun first to confirm the target and the text."

export interface RespondOptions {
  commentId: string
  message: string
  pageId?: string | undefined
  dryRun?: boolean | undefined
  /** Ignored by the runner. Confirmation is enforced at registration, where the server object is in scope. */
  confirmed?: boolean | undefined
}

export interface RespondDeps {
  client: PagesClient
  tokens: TokenProvider
}

export async function runRespondToComment(
  deps: RespondDeps,
  options: RespondOptions,
): Promise<
  | { dryRun: true; intended: { commentId: string; messageLength: number; visibility: string } }
  | { ok: true; replyId: string; commentId: string }
  | { error: string }
> {
  const message = options.message.trim()
  if (message === "") return { error: "A reply needs a non-empty message." }

  if (options.dryRun === true) {
    // Report the shape of the effect, never the text itself — the caller wrote
    // it and echoing it back costs context for nothing.
    return {
      dryRun: true,
      intended: {
        commentId: options.commentId,
        messageLength: message.length,
        visibility: "public",
      },
    }
  }

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
    // No retry. A retried reply is a double post.
    const created = await client.comments.reply(options.commentId, message)
    return { ok: true, replyId: created.id, commentId: options.commentId }
  } catch (cause) {
    return {
      error: explainGraphError(cause, {
        operation: "reply",
        ...(options.pageId !== undefined && { pageId: options.pageId }),
        ...(tasks !== undefined && { tasks }),
      }),
    }
  }
}
