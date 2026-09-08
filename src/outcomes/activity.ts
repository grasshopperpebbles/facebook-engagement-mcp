import { PageAccessError, type TokenProvider } from "../auth/token-provider.js"
import { createLogger, type Logger, timed } from "../logging/logger.js"
import {
  abbreviateComment,
  commentTextCost,
  createTextBudget,
  MAX_COMMENT_CHARS,
  MAX_RESPONSE_TEXT_CHARS,
  type RenderedComment,
  renderComment,
  type UntrustedText,
  untrustedText,
} from "../render/truncate.js"
import type {
  Campaign,
  Comment,
  MetaClient,
  Page,
  PagesClient,
} from "../vendor/meta-client/index.js"
import { resolveAdPosts } from "./ad-posts.js"
import { explainGraphError } from "./errors.js"
import { type Counts, countThreads, type Group, type GroupBy, groupThreads } from "./grouping.js"
import { assembleThreads, type Thread, type ThreadPost, threadPost } from "./threads.js"
import {
  applyFilter,
  type Filter,
  type StatusBasis,
  type TriagedThread,
  triageThreads,
} from "./triage.js"

export type Target =
  | { kind: "pages" }
  | { kind: "page"; id: string }
  | { kind: "post"; id: string }
  | { kind: "comment"; id: string }
  | { kind: "ad"; id: string }
  | { kind: "campaign"; id: string }
  | { kind: "adAccount"; id: string }

/** An ad account this identity can reach. Orientation only — no spend, no budgets. */
export interface AdAccountSummary {
  id: string
  name?: string
}

/**
 * A campaign, named so a person can choose one without knowing its id.
 *
 * `status` prefers `effective_status`, which accounts for a parent being
 * paused or an account being disabled; `status` alone reports only what was
 * set on the campaign itself. Every campaign is listed regardless of status:
 * a finished campaign's comments are still comments, and which statuses
 * matter is the caller's judgement, not this server's.
 */
export interface CampaignSummary {
  id: string
  name?: string
  status?: string
}

/**
 * `ThreadPost` with its free text moved behind the same untrusted marker as
 * comment text — a post's own message is still public, third-party-reachable
 * content (that is the whole point of the unpublished/ad-backed case), so it
 * gets the same treatment. Nothing in a response may carry a raw `message` key.
 */
export type RenderedPost = Omit<ThreadPost, "message"> & { text?: UntrustedText }

export interface RenderedThread {
  comment: RenderedComment
  replies: RenderedComment[]
  /** Look the body up in the response's top-level `posts`. */
  postId: string
  status: string
  /**
   * Present and true when the response ran out of text budget before this
   * thread: structure is returned, free text is not, and `notes` says how many.
   */
  abbreviated?: true
}

export interface RenderedGroup extends Omit<Group, "threads"> {
  threads: RenderedThread[]
}

export interface CommentActivity {
  target: Target
  filter: Filter
  groupBy: GroupBy
  since: string
  /** Which question `needs_reply` answered. See triage.ts. */
  statusBasis: StatusBasis
  totals: Counts
  /**
   * Every post referenced by a returned thread, once each. Post bodies live
   * here rather than on each thread: repeating a 1,000-character ad body onto
   * every one of its comment threads is the same string a hundred times over.
   */
  posts: RenderedPost[]
  groups: RenderedGroup[]
  /** True when some part of the answer could not be retrieved or was abbreviated. */
  partial: boolean
  notes: string[]
}

export interface RunOptions {
  page?: string | undefined
  post?: string | undefined
  comment?: string | undefined
  ad?: string | undefined
  campaign?: string | undefined
  adAccount?: string | undefined
  filter?: Filter | undefined
  groupBy?: GroupBy | undefined
  since?: string | undefined
  maxThreads?: number | undefined
  today?: Date | undefined
}

export interface Deps {
  client: PagesClient
  tokens: TokenProvider
  logger?: Logger
  meta?: MetaClient
}

/** Ad accounts listed during orientation. */
const MAX_AD_ACCOUNTS = 50
/** Campaigns listed for one account. Overflow is reported, never silently cut. */
export const MAX_CAMPAIGNS = 100

const DEFAULT_SINCE_DAYS = 30
/** Posts scanned per Page sweep. Pagination is internal; no cursor is exposed. */
const MAX_POSTS = 50
/**
 * Comments read per post before triage.
 *
 * Deliberately *not* `maxThreads`: spec §3 makes `maxThreads` a returned-volume
 * cap, not a page size. Using it as one would mean `maxThreads: 5` fetched five
 * comments from each of fifty posts and then triaged that arbitrary slice,
 * which answers a different question from "the five newest threads that need a
 * reply."
 */
const MAX_COMMENTS_PER_POST = 200
/** Replies read per comment thread when sweeping. Internal, like the above. */
const MAX_REPLIES_PER_COMMENT = 25
/** Replies read when one comment *is* the target — the drill-down is the point. */
const MAX_REPLIES_FOR_COMMENT_TARGET = 100
/**
 * Share of the response text budget post bodies may take, so a wall of ad copy
 * cannot starve the comments, which are the actual answer.
 */
const POST_TEXT_BUDGET = Math.floor(MAX_RESPONSE_TEXT_CHARS / 4)

function isoDaysAgo(days: number, today: Date): string {
  const date = new Date(today.getTime() - days * 24 * 60 * 60 * 1000)
  return date.toISOString().slice(0, 10)
}

/**
 * Ad accounts for the orientation call, degraded to a note when they cannot be
 * read.
 *
 * Most tokens carry no `ads_read` — it is needed only for ad and campaign
 * targets — and orientation is the call a model makes before it knows what it
 * needs. Failing the whole answer would hide the Pages the token *can* reach
 * behind a permission it may never use, so a refusal is reported alongside the
 * Pages rather than instead of them.
 */
async function listAdAccounts(
  deps: Deps,
): Promise<{ accounts?: AdAccountSummary[]; notes: string[] }> {
  if (deps.meta === undefined) return { notes: [] }
  try {
    const { items } = await deps.meta.accounts.list({ maxItems: MAX_AD_ACCOUNTS })
    return {
      accounts: items.map((account) => ({
        id: account.id,
        ...(account.name !== undefined && { name: account.name }),
      })),
      notes: [],
    }
  } catch (cause) {
    return {
      notes: [`Ad accounts unavailable: ${explainGraphError(cause, { operation: "ad read" })}`],
    }
  }
}

/**
 * `effective_status` in preference to `status`: a campaign set ACTIVE inside a
 * paused ad set or a disabled account is not running, and only the effective
 * value says so.
 */
function toCampaignSummary(campaign: Campaign): CampaignSummary {
  const status = campaign.effective_status ?? campaign.status
  return {
    id: campaign.id,
    ...(campaign.name !== undefined && { name: campaign.name }),
    ...(status !== undefined && { status }),
  }
}

function resolveTarget(options: RunOptions): Target | { error: string } {
  const given = (["page", "post", "comment", "ad", "campaign", "adAccount"] as const).filter(
    (k) => options[k] !== undefined,
  )
  if (given.length > 1) {
    return { error: `Give one target only; received ${given.join(" and ")}.` }
  }
  if (options.adAccount !== undefined) return { kind: "adAccount", id: options.adAccount }
  if (options.page !== undefined) return { kind: "page", id: options.page }
  if (options.post !== undefined) return { kind: "post", id: options.post }
  if (options.comment !== undefined) return { kind: "comment", id: options.comment }
  if (options.ad !== undefined) return { kind: "ad", id: options.ad }
  if (options.campaign !== undefined) return { kind: "campaign", id: options.campaign }
  return { kind: "pages" }
}

/**
 * Page posts carry composite ids of the form `{page-id}_{post-id}`, so the
 * Page can usually be recovered from the post alone. Best effort: the format
 * is not guaranteed, and callers may pass an id of another shape.
 *
 * Real Facebook Page ids are numeric, but this server's own fixtures use
 * readable stand-ins (`pg1_p2`), so this only checks for a non-empty prefix
 * before the first underscore rather than requiring digits — a stricter
 * check would silently defeat itself against this repo's own test data.
 */
function pageIdFromPostId(postId: string): string | undefined {
  const [prefix, rest] = postId.split("_")
  return prefix !== undefined && prefix.length > 0 && rest !== undefined && rest.length > 0
    ? prefix
    : undefined
}

/**
 * Milliseconds for a Graph timestamp such as `2026-08-30T10:00:00+0000`, or
 * `undefined` when it is missing or unparseable. Never throws: a malformed
 * timestamp must cost that thread its ordering, not the whole answer.
 */
function commentTime(comment: Comment): number | undefined {
  if (comment.createdTime === undefined) return undefined
  const parsed = Date.parse(comment.createdTime)
  return Number.isNaN(parsed) ? undefined : parsed
}

/** Newest first; threads with no usable timestamp last. */
function newestFirst(a: TriagedThread, b: TriagedThread): number {
  const left = commentTime(a.comment)
  const right = commentTime(b.comment)
  if (left === undefined) return right === undefined ? 0 : 1
  if (right === undefined) return -1
  return right - left
}

function renderPost(post: ThreadPost): RenderedPost {
  const { message, ...rest } = post
  if (message === undefined) return rest

  return { ...rest, text: untrustedText(message, MAX_COMMENT_CHARS) }
}

function abbreviatePost(post: ThreadPost): RenderedPost {
  const { message, ...rest } = post
  void message
  return rest
}

const renderThread = (thread: TriagedThread): RenderedThread => ({
  comment: renderComment(thread.comment),
  replies: thread.replies.map(renderComment),
  postId: thread.post.id,
  status: thread.status,
})

/**
 * Structure without free text. Enough to count, group, and act on the thread —
 * or to come back for it with a `comment` target — without spending budget the
 * response no longer has.
 */
const abbreviateThread = (thread: TriagedThread): RenderedThread => ({
  comment: abbreviateComment(thread.comment),
  replies: [],
  postId: thread.post.id,
  status: thread.status,
  abbreviated: true,
})

/**
 * The Page whose access token should read this target's comments.
 *
 * `ad` and `campaign` targets are resolved to their posts *before* this runs,
 * because a resolved `effective_object_story_id` is exactly the
 * `{page-id}_{post-id}` composite the derivation understands. Deriving it
 * afterwards, as an earlier version did, left ad and campaign reads on the
 * user token — which Graph answers with empty `data`, not an error.
 */
function pageIdForTarget(target: Target, posts: ThreadPost[]): string | undefined {
  switch (target.kind) {
    case "page":
      return target.id
    case "post":
      return pageIdFromPostId(target.id)
    case "ad":
    case "campaign": {
      const first = posts[0]
      return first === undefined ? undefined : pageIdFromPostId(first.id)
    }
    default:
      return undefined
  }
}

/**
 * Read comment activity for a Page, post, or single comment.
 *
 * A Page target reads `/feed`, so unpublished ad-backed posts are included —
 * that is the point of the server. Failures on individual posts degrade the
 * answer to partial rather than failing it (tool-design checklist item 7).
 */
export async function runCommentActivity(
  deps: Deps,
  options: RunOptions,
): Promise<
  | CommentActivity
  | { pages: Page[]; adAccounts?: AdAccountSummary[]; notes?: string[] }
  | { campaigns: CampaignSummary[]; truncated: boolean }
  | { error: string }
> {
  const target = resolveTarget(options)
  if ("error" in target) return target

  const { filter = "needs_reply", groupBy = "post", maxThreads = 100, today = new Date() } = options
  const since = options.since ?? isoDaysAgo(DEFAULT_SINCE_DAYS, today)
  const notes: string[] = []
  const logger = deps.logger ?? createLogger()
  let partial = false

  // No target: what this identity can reach at all. Two cheap list calls, no
  // posts and no comments — this is the call a model makes to find its feet
  // before spending anything, and the ad accounts belong in it because ads are
  // the only route to comments on unpublished posts.
  if (target.kind === "pages") {
    let pages: Page[]
    try {
      pages = (await deps.client.pages.list({ maxItems: 100 })).items
    } catch (cause) {
      return { error: explainGraphError(cause, { operation: "read" }) }
    }

    const ads = await listAdAccounts(deps)
    return {
      pages,
      ...(ads.accounts !== undefined && { adAccounts: ads.accounts }),
      ...(ads.notes.length > 0 && { notes: ads.notes }),
    }
  }

  // An ad account names its campaigns, so a person can pick one by name rather
  // than pasting an id out of Ads Manager. Deliberately reads no comments: it
  // is the rung between "what can I reach" and the sweep, and it is only worth
  // walking speculatively while it stays cheap.
  if (target.kind === "adAccount") {
    if (deps.meta === undefined) {
      return {
        error: "Ad and campaign targets need Marketing API access, which is not configured.",
      }
    }
    try {
      const { items, truncated } = await deps.meta.campaigns.list(target.id, {
        maxItems: MAX_CAMPAIGNS,
      })
      return { campaigns: items.map(toCampaignSummary), truncated }
    } catch (cause) {
      return { error: explainGraphError(cause, { operation: "ad read" }) }
    }
  }

  let posts: ThreadPost[] = []
  let threads: Thread[] = []

  // Ad and campaign targets resolve to their Page posts first, before a token
  // is chosen: the resolved post ids are the only thing that names the Page,
  // and comments read with a user token come back as empty `data` rather than
  // an error — a silent zero on the exact capability this tool headlines.
  // This runs on the Marketing client, which is user-token scoped by design.
  if (target.kind === "ad" || target.kind === "campaign") {
    if (deps.meta === undefined) {
      return {
        error: "Ad and campaign targets need Marketing API access, which is not configured.",
      }
    }
    try {
      const resolved = await resolveAdPosts(deps.meta, { [target.kind]: target.id })
      notes.push(...resolved.notes)
      if (resolved.notes.length > 0) partial = true
      posts = resolved.postIds.map((id) => ({ id }))
    } catch (cause) {
      return { error: explainGraphError(cause, { operation: "read" }) }
    }
  }

  // Comment reads need a Page token: a user token returns empty data, which
  // reads as "no comments" rather than "wrong token" — the whole reason the
  // Page-token exchange exists. A page target names its Page explicitly; every
  // other target's Page is recovered from a `{page-id}_{post-id}` composite
  // best-effort. Any derivation failure — an id with no usable prefix, or a
  // derived Page this identity cannot reach — falls back to the user token
  // silently rather than erroring; `usedUserToken` lets the caller be told
  // about it below instead.
  let pageId: string | undefined
  let usedUserToken = false
  let accessToken: string

  if (target.kind === "page") {
    pageId = target.id
    try {
      accessToken = await deps.tokens.forPage(target.id)
    } catch (cause) {
      if (cause instanceof PageAccessError) return { error: cause.message }
      return { error: explainGraphError(cause, { operation: "read", pageId: target.id }) }
    }
  } else {
    const derived = pageIdForTarget(target, posts)
    let derivedToken: string | undefined
    if (derived !== undefined) {
      try {
        derivedToken = await deps.tokens.forPage(derived)
      } catch {
        derivedToken = undefined
      }
    }
    if (derivedToken !== undefined) {
      accessToken = derivedToken
      pageId = derived
    } else {
      // Guarded like the page-target branch above: `forUser` is documented as
      // a pluggable seam (token-provider.ts) and a database- or OAuth-backed
      // implementation could reject here, which must become a returned error
      // rather than an unhandled rejection escaping the tool.
      try {
        accessToken = await deps.tokens.forUser()
        usedUserToken = true
      } catch (cause) {
        if (cause instanceof PageAccessError) return { error: cause.message }
        return { error: explainGraphError(cause, { operation: "read" }) }
      }
    }
  }
  const client = deps.client.withToken(accessToken)

  // A campaign can run ads on several Pages, and one request carries one Page
  // token. This is a real limitation, not a degradation to paper over.
  if (pageId !== undefined && (target.kind === "ad" || target.kind === "campaign")) {
    const spanned = new Set(
      posts
        .map((post) => pageIdFromPostId(post.id))
        .filter((id): id is string => id !== undefined && id !== pageId),
    )
    if (spanned.size > 0) {
      partial = true
      notes.push(
        `This ${target.kind} runs on ${spanned.size + 1} Pages, but comments can be read with ` +
          `only one Page access token per call. Only posts belonging to Page ${pageId} were read ` +
          "with a Page token; posts on the other Pages return no comments rather than an error, " +
          "so an empty result for them is not evidence there are none. Query those Pages " +
          "separately.",
      )
    }
  }

  try {
    if (target.kind === "page") {
      const feed = await client.posts.list(target.id, { maxItems: MAX_POSTS, since })
      posts = feed.items.map(threadPost)
      if (feed.truncated) {
        partial = true
        notes.push(`More than ${MAX_POSTS} posts exist since ${since}; older posts were not read.`)
      }
    } else if (target.kind === "post") {
      // No post node is fetched, so its published state is unknown and is
      // omitted rather than asserted. See ThreadPost.
      posts = [{ id: target.id }]
    }
  } catch (cause) {
    return {
      error: explainGraphError(cause, {
        operation: "read",
        ...(pageId !== undefined && { pageId }),
      }),
    }
  }

  if (target.kind === "comment") {
    try {
      const root = await client.comments.get(target.id)
      const { items } = await client.comments.replies(target.id, {
        maxItems: MAX_REPLIES_FOR_COMMENT_TARGET,
      })
      threads = [{ comment: root, replies: items, post: { id: "unknown" } }]
    } catch (cause) {
      return {
        error: explainGraphError(cause, {
          operation: "read",
          ...(pageId !== undefined && { pageId }),
        }),
      }
    }
  } else {
    for (const post of posts) {
      try {
        const { items: comments } = await timed(
          logger,
          { operation: "read_comments", ...(pageId !== undefined && { pageId }), postId: post.id },
          () => client.comments.forPost(post.id, { maxItems: MAX_COMMENTS_PER_POST }),
        )
        const repliesByCommentId = new Map<string, Comment[]>()

        for (const comment of comments) {
          if ((comment.replyCount ?? 0) === 0) continue
          const { items } = await client.comments.replies(comment.id, {
            maxItems: MAX_REPLIES_PER_COMMENT,
          })
          repliesByCommentId.set(comment.id, items)
        }

        threads.push(...assembleThreads({ post, comments, repliesByCommentId }))
      } catch (cause) {
        // One failing post must not fail the whole answer — it becomes a
        // labelled gap instead.
        partial = true
        notes.push(
          `Comments for post ${post.id} could not be read: ` +
            explainGraphError(cause, {
              operation: "read",
              ...(pageId !== undefined && { pageId }),
            }),
        )
      }
    }
  }

  const { threads: triaged, basis } = triageThreads(threads, pageId)
  const filtered = applyFilter(triaged, filter)
  // Sort before capping. Comments arrive oldest-first (`order: chronological`)
  // and post by post, so an unsorted slice keeps the oldest threads from the
  // first few posts while the note below claims the newest — dropping exactly
  // the comments a triage tool exists to surface.
  const capped = [...filtered].sort(newestFirst).slice(0, maxThreads)
  if (capped.length < filtered.length) {
    partial = true
    notes.push(`${filtered.length} threads matched; the ${capped.length} most recent are shown.`)
  }
  if (basis === "reply_count") {
    notes.push(
      "Comment authors were not returned by Graph, so 'needs reply' means nobody replied at " +
        "all, not that the Page has not replied.",
    )
  }
  // `basis` is computed once for the whole batch (triage.ts): one comment
  // anywhere carrying an author id is enough to call the batch
  // "author_identity". An individual thread whose own comments carry no
  // author id is still evaluated by the identity path and, finding no
  // Page-authored reply, comes out `needs_reply` — safe in that it
  // over-surfaces rather than hides a waiting customer, but the batch-wide
  // confidence claim does not hold for it. Say so.
  const withoutAuthors = capped.filter(
    (t) => t.comment.author?.id === undefined && t.replies.every((r) => r.author?.id === undefined),
  ).length
  if (basis === "author_identity" && withoutAuthors > 0) {
    notes.push(
      `${withoutAuthors} of these threads carried no author information, so they are listed as ` +
        "needing a reply because it could not be confirmed the Page had replied.",
    )
  }

  if (usedUserToken) {
    notes.push(
      "Comments were read with the user access token because the Page could not be determined " +
        "from this target. Meta returns no comments for a user token, so an empty result here " +
        "may mean the wrong token rather than no comments. Pass a page target to avoid this.",
    )
  }

  // `since` is a Graph-side filter on the `/feed` sweep and nothing else. It is
  // echoed on every response, so every response that did not apply it has to
  // say so — otherwise the echoed value reads as a filter that ran.
  if (target.kind !== "page") {
    notes.push(
      `'since' (${since}) filters the post sweep of a page target only. It was not applied to ` +
        `this ${target.kind} target, so comments older than that date can appear below.`,
    )
  }

  // Render inside a shared character budget. Post bodies are hoisted here and
  // deduplicated; threads reference them by id.
  const budget = createTextBudget(MAX_RESPONSE_TEXT_CHARS)

  const uniquePosts = new Map<string, ThreadPost>()
  for (const thread of capped) {
    if (!uniquePosts.has(thread.post.id)) uniquePosts.set(thread.post.id, thread.post)
  }

  let postSpend = 0
  let abbreviatedPosts = 0
  const renderedPosts: RenderedPost[] = []
  for (const post of uniquePosts.values()) {
    const cost = post.message === undefined ? 0 : Math.min(post.message.length, MAX_COMMENT_CHARS)
    if (cost === 0 || (postSpend + cost <= POST_TEXT_BUDGET && budget.take(cost))) {
      postSpend += cost
      renderedPosts.push(renderPost(post))
    } else {
      abbreviatedPosts += 1
      renderedPosts.push(abbreviatePost(post))
    }
  }

  // Threads are already newest-first, so the budget buys the most recent ones.
  // Once one thread does not fit, every later thread is abbreviated too: a
  // response that cherry-picked short threads out of the middle would be
  // harder to reason about than a clean prefix.
  let exhausted = false
  let abbreviatedThreads = 0
  const rendered = new Map<TriagedThread, RenderedThread>()
  for (const thread of capped) {
    const cost =
      commentTextCost(thread.comment) +
      thread.replies.reduce((sum, reply) => sum + commentTextCost(reply), 0)
    if (!exhausted && budget.take(cost)) {
      rendered.set(thread, renderThread(thread))
    } else {
      exhausted = true
      abbreviatedThreads += 1
      rendered.set(thread, abbreviateThread(thread))
    }
  }

  if (abbreviatedThreads > 0) {
    partial = true
    notes.push(
      `${abbreviatedThreads} of the ${capped.length} threads below are listed without their ` +
        `text: the response reached its ${MAX_RESPONSE_TEXT_CHARS}-character text budget. They ` +
        "are marked `abbreviated`. Lower maxThreads, narrow the filter, or read one with a " +
        "comment target to see them.",
    )
  }
  if (abbreviatedPosts > 0) {
    partial = true
    notes.push(
      `${abbreviatedPosts} post bodies were omitted to stay within the response text budget.`,
    )
  }

  return {
    target,
    filter,
    groupBy,
    since,
    statusBasis: basis,
    totals: countThreads(capped),
    posts: renderedPosts,
    groups: groupThreads(capped, groupBy).map((group) => ({
      ...group,
      threads: group.threads.map((thread) => rendered.get(thread) ?? abbreviateThread(thread)),
    })),
    partial,
    notes,
  }
}
