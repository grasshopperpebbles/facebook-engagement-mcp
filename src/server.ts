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
import { createMetaClient, createPagesClient, type FetchImpl } from "./vendor/meta-client/index.js"

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
export const SERVER_VERSION = "0.1.3"

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
  /**
   * Publish without a confirmation prompt on a client that cannot show one.
   *
   * Set from the environment, never from a tool argument. This used to be a
   * `confirmed` boolean in the tool's input schema, which meant the model
   * granted its own permission — and it did: asked to publish on a client
   * without elicitation, Claude Desktop's model announced it would "fire it
   * with dryRun: false and confirmed: true". An environment variable is
   * something a person sets and a model cannot reach, which is the only
   * property that matters here.
   */
  allowUnconfirmedWrites?: boolean
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

  // Resolves an ad or campaign to the Page post behind it. That is the only
  // route to comments on an ad: ads run on unpublished posts, and no
  // Page-level edge returns those — verified 2026-09-03, contrary to Meta's
  // documentation. A `page` target sweeps published posts only.
  const meta = createMetaClient({
    accessToken: options.accessToken,
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
          { client, tokens, meta },
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
            if (confirmed === "unsupported" && options.allowUnconfirmedWrites !== true) {
              return {
                error:
                  "This client cannot show a confirmation prompt, so this reply was not " +
                  "published. Publishing as the Page is public immediately and cannot be " +
                  "taken back, so it needs a person's approval rather than a model's. Use a " +
                  "client that supports MCP elicitation, or — if you are automating this " +
                  "deliberately — set FACEBOOK_ENGAGEMENT_ALLOW_UNCONFIRMED_WRITES=true in " +
                  "the server's environment, which only someone with access to that " +
                  "environment can do.",
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
