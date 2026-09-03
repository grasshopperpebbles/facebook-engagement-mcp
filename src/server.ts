import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js"
import type { CallToolResult } from "@modelcontextprotocol/sdk/types.js"
import { createEnvTokenProvider } from "./auth/token-provider.js"
import {
  TOOL_NAME as ACTIVITY_TOOL,
  description as activityDescription,
  inputSchema as activityInput,
  runCommentActivity,
} from "./tools/comment-activity.js"
import * as moderate from "./tools/moderate-comment.js"
import * as respond from "./tools/respond-to-comment.js"
import { createPagesClient, type FetchImpl } from "./vendor/meta-client/index.js"

/**
 * Run a tool and shape the outcome for MCP.
 *
 * Nothing may throw past here. An unhandled error becomes a protocol error the
 * model cannot read or act on, whereas a tool error is something it can report
 * or work around.
 */
export async function asToolResult(
  run: () => Promise<{ error: string } | object>,
): Promise<CallToolResult> {
  try {
    const result = await run()
    if ("error" in result) {
      return { content: [{ type: "text", text: String(result.error) }], isError: true }
    }
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] }
  } catch (cause) {
    return {
      content: [
        { type: "text", text: cause instanceof Error ? cause.message : "Unexpected server error." },
      ],
      isError: true,
    }
  }
}

export const SERVER_NAME = "facebook-engagement-mcp"
export const SERVER_VERSION = "0.0.0"

/**
 * Ask the user to confirm a publish.
 *
 * Returns "unsupported" when the client cannot elicit, which the caller treats
 * as a refusal unless `confirmed` was passed explicitly. Treating an
 * unsupported capability as consent would silently delete the gate.
 */
export async function confirmReply(
  server: McpServer,
  commentId: string,
): Promise<boolean | "unsupported"> {
  if (server.server.getClientCapabilities()?.elicitation === undefined) return "unsupported"

  const response = await server.server.elicitInput({
    message: `Publish a public reply to comment ${commentId} as the Page? This is visible immediately and deleting it later does not unpublish what people saw.`,
    requestedSchema: {
      type: "object",
      properties: {
        confirm: { type: "boolean", title: "Publish this reply", description: "Yes, publish it" },
      },
      required: ["confirm"],
    },
  })

  return response.action === "accept" && response.content?.["confirm"] === true
}

export interface ServerOptions {
  accessToken: string
  /** Write tools are not registered unless this is true (architecture §7). */
  enableWrites?: boolean
  /** Injected by tests and evals to serve recorded fixtures. */
  fetchImpl?: FetchImpl
  today?: Date
}

/**
 * Registration only — no logic here. The computation lives in `outcomes/`,
 * where it is unit-testable without a protocol in the way (architecture §5).
 */
export function createServer(options: ServerOptions): McpServer {
  const server = new McpServer({ name: SERVER_NAME, version: SERVER_VERSION })
  const fetchImpl = options.fetchImpl
  const client = createPagesClient({
    accessToken: options.accessToken,
    ...(fetchImpl && { fetchImpl }),
  })
  const tokens = createEnvTokenProvider({
    userToken: options.accessToken,
    ...(fetchImpl && { fetchImpl }),
  })

  server.registerTool(
    ACTIVITY_TOOL,
    {
      title: "Comment activity",
      description: activityDescription,
      inputSchema: activityInput,
      annotations: {
        readOnlyHint: true,
        destructiveHint: false,
        idempotentHint: true,
        openWorldHint: true,
      },
    },
    async (args) =>
      asToolResult(() =>
        runCommentActivity(
          { client, tokens },
          { ...args, ...(options.today && { today: options.today }) },
        ),
      ),
  )

  // Writes ship disabled and enable by explicit configuration (architecture §7).
  if (options.enableWrites === true) {
    server.registerTool(
      respond.TOOL_NAME,
      {
        title: "Respond to comment",
        description: respond.description,
        inputSchema: respond.inputSchema,
        annotations: {
          readOnlyHint: false,
          destructiveHint: false,
          idempotentHint: false,
          openWorldHint: true,
        },
      },
      async (args) =>
        asToolResult(async () => {
          if (args.dryRun !== true) {
            const confirmed = await confirmReply(server, args.commentId)
            if (confirmed === "unsupported" && args.confirmed !== true) {
              return {
                error:
                  "This client cannot show a confirmation prompt. Review the reply with " +
                  "dryRun: true, then call again with confirmed: true.",
              }
            }
            if (confirmed === false) {
              return { error: "The reply was not published: confirmation was declined." }
            }
          }
          return respond.runRespondToComment({ client, tokens }, args)
        }),
    )

    server.registerTool(
      moderate.TOOL_NAME,
      {
        title: "Moderate comment",
        description: moderate.description,
        inputSchema: moderate.inputSchema,
        annotations: {
          readOnlyHint: false,
          destructiveHint: false,
          idempotentHint: true,
          openWorldHint: true,
        },
      },
      async (args) => asToolResult(() => moderate.runModerateComment({ client, tokens }, args)),
    )
  }

  return server
}
