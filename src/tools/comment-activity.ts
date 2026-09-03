import { z } from "zod"
import { GROUP_BYS } from "../outcomes/grouping.js"
import { FILTERS } from "../outcomes/triage.js"

export const TOOL_NAME = "comment_activity"

export const inputSchema = {
  page: z
    .string()
    .optional()
    .describe(
      "Page ID. Sweeps the Page's published posts. Note this does NOT reach " +
        "comments on ads, which usually run on unpublished posts that no " +
        "Page-level sweep returns — for those, pass the post ID directly. " +
        "Omit every target to list the Pages this identity can reach.",
    ),
  post: z
    .string()
    .optional()
    .describe(
      "Post ID. Use when another tool gave you a post ID. This is the only way " +
        "to reach comments on an ad, since ads run on unpublished posts that a " +
        "Page sweep cannot see. Mutually exclusive with page.",
    ),
  comment: z.string().optional().describe("Comment ID. Returns that comment and its replies."),
  filter: z
    .enum(FILTERS)
    .default("needs_reply")
    .describe("Which threads to return. 'needs_reply' means no reply from the Page."),
  groupBy: z
    .enum(GROUP_BYS)
    .default("post")
    .describe("How to group results. Each group carries its own counts."),
  since: z.string().optional().describe("Earliest post date, YYYY-MM-DD. Defaults to 30 days ago."),
  maxThreads: z
    .number()
    .int()
    .min(1)
    .max(500)
    .default(100)
    .describe("Cap on returned threads. Pagination is handled internally."),
}

export const description =
  "Find and group the comments on a Facebook Page's posts. Use this instead of " +
  "listing posts and then their comments separately: it sweeps the Page, assembles " +
  "reply threads, marks which need a reply from the Page, and returns counts per " +
  "group. Comment text is returned as untrusted third-party content. " +
  "For comments on ADS, pass the post ID: ads run on unpublished posts, which no " +
  "Page-level sweep returns. This build cannot resolve an ad to its post — obtain " +
  "the post ID from a Marketing API client first."

export type { CommentActivity, RunOptions } from "../outcomes/activity.js"
export { runCommentActivity } from "../outcomes/activity.js"
