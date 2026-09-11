import { readFileSync } from "node:fs"
import { join } from "node:path"
import { describe, expect, it, vi } from "vitest"
import { createEnvTokenProvider } from "../src/auth/token-provider.js"
import { createLogger } from "../src/logging/logger.js"
import { runCommentActivity } from "../src/outcomes/activity.js"
import { MAX_COMMENT_CHARS, MAX_RESPONSE_TEXT_CHARS } from "../src/render/truncate.js"
import { createPagesClient } from "../src/vendor/meta-client/index.js"

const root = join(import.meta.dirname, "..")
const fixture = (name: string) => readFileSync(join(root, "fixtures", `${name}.json`), "utf8")

/** Serves the fixture matching each Graph path. */
const fixtureFetch = () =>
  vi.fn(async (url: string) => {
    const { pathname, searchParams } = new URL(url)
    if (pathname.endsWith("/me/accounts")) {
      const wantsToken = searchParams.get("fields")?.includes("access_token")
      return new Response(fixture(wantsToken ? "page-credentials" : "pages"), { status: 200 })
    }
    if (pathname.endsWith("/feed")) return new Response(fixture("feed"), { status: 200 })
    if (pathname.endsWith("/pg1_p1/comments")) {
      return new Response(fixture("comments-p1"), { status: 200 })
    }
    if (pathname.endsWith("/pg1_p2/comments")) {
      return new Response(fixture("comments-p2"), { status: 200 })
    }
    if (pathname.endsWith("/pg1_p2_c1/comments")) {
      return new Response(fixture("replies"), { status: 200 })
    }
    return new Response(JSON.stringify({ data: [] }), { status: 200 })
  })

function deps(fetchImpl = fixtureFetch()) {
  return {
    client: createPagesClient({ accessToken: "fixture", fetchImpl: fetchImpl as never }),
    tokens: createEnvTokenProvider({ userToken: "fixture", fetchImpl: fetchImpl as never }),
  }
}

/** A minimal shape for the rendered groups these assertions reach into. */
interface RenderedGroupish {
  key: string
  label: string
  threads: {
    postId: string
    abbreviated?: true
    comment: { id: string; createdTime?: string; text?: { value: string } }
  }[]
}

const threadIds = (groups: RenderedGroupish[]) =>
  groups.flatMap((g) => g.threads).map((t) => t.comment.id)

/** Serves the standard fixtures but with per-post comment bodies substituted. */
const commentsFetch = (byPost: Record<string, unknown[]>) =>
  vi.fn(async (url: string) => {
    const { pathname, searchParams } = new URL(url)
    if (pathname.endsWith("/me/accounts")) {
      const wantsToken = searchParams.get("fields")?.includes("access_token")
      return new Response(fixture(wantsToken ? "page-credentials" : "pages"), { status: 200 })
    }
    if (pathname.endsWith("/feed")) return new Response(fixture("feed"), { status: 200 })
    for (const [postId, data] of Object.entries(byPost)) {
      if (pathname.endsWith(`/${postId}/comments`)) {
        return new Response(JSON.stringify({ data }), { status: 200 })
      }
    }
    return new Response(JSON.stringify({ data: [] }), { status: 200 })
  })

/**
 * The two posts carry comments in opposite date order to the order Graph
 * returns them in: p1 is read first but holds the oldest comments.
 */
const datedFetch = () =>
  commentsFetch({
    pg1_p1: [
      {
        id: "pg1_p1_c1",
        message: "old",
        created_time: "2026-08-01T10:00:00+0000",
        comment_count: 0,
      },
      {
        id: "pg1_p1_c2",
        message: "old",
        created_time: "2026-08-02T10:00:00+0000",
        comment_count: 0,
      },
    ],
    pg1_p2: [
      {
        id: "pg1_p2_c1",
        message: "new",
        created_time: "2026-08-29T10:00:00+0000",
        comment_count: 0,
      },
      {
        id: "pg1_p2_c2",
        message: "new",
        created_time: "2026-08-30T10:00:00+0000",
        comment_count: 0,
      },
    ],
  })

/** Only the MetaClient members an ad/campaign resolution touches. */
const metaStub = (postIds: string[]) => {
  let call = 0
  return {
    ads: {
      list: vi.fn(async () => ({
        items: postIds.map((_, i) => ({ id: `a${i}` })),
        truncated: false,
      })),
    },
    adSets: { list: vi.fn(async () => ({ items: [{ id: "as1" }], truncated: false })) },
    creatives: {
      forAd: vi.fn(async () => ({ effective_object_story_id: postIds[call++ % postIds.length] })),
    },
  }
}

/** Every character of untrusted free text anywhere in a response. */
function renderedTextChars(value: unknown): number {
  if (Array.isArray(value)) return value.reduce<number>((sum, v) => sum + renderedTextChars(v), 0)
  if (value === null || typeof value !== "object") return 0
  const record = value as Record<string, unknown>
  if (record["untrusted"] === true && typeof record["value"] === "string") {
    return record["value"].length
  }
  return Object.values(record).reduce<number>((sum, v) => sum + renderedTextChars(v), 0)
}

describe("runCommentActivity", () => {
  it("lists Pages when no target is given", async () => {
    const result = await runCommentActivity(deps(), {})

    expect(result).toMatchObject({ pages: [{ id: "pg1", name: "Test Shop", category: "Retail" }] })
  })

  it("never returns a Page access token in the Page listing", async () => {
    const result = await runCommentActivity(deps(), {})

    expect(JSON.stringify(result)).not.toContain("PAGE-TOKEN-FIXTURE")
  })

  it("includes comments on unpublished ad-backed posts for a page target", async () => {
    // The differentiating capability: every competing server reads the
    // published feed and cannot see these at all.
    const result = await runCommentActivity(deps(), { page: "pg1", filter: "all" })

    const ids = JSON.stringify(result)
    expect(ids).toContain("pg1_p2_c1")
  })

  it("labels which post is unpublished when grouping by post", async () => {
    const result = await runCommentActivity(deps(), { page: "pg1", filter: "all", groupBy: "post" })

    const group = (result as { groups: { key: string; label: string }[] }).groups.find(
      (g) => g.key === "pg1_p2",
    )
    expect(group!.label).toContain("unpublished")
  })

  it("defaults to threads needing a reply", async () => {
    const result = await runCommentActivity(deps(), { page: "pg1" })
    const activity = result as { filter: string; totals: { needsReply: number } }

    expect(activity.filter).toBe("needs_reply")
    // c1 on p1 has no reply; c1 on p2 was answered by the Page; c2 is hidden.
    expect(activity.totals.needsReply).toBe(1)
  })

  it("states the basis the status was computed from", async () => {
    const result = await runCommentActivity(deps(), { page: "pg1" })

    expect((result as { statusBasis: string }).statusBasis).toBe("author_identity")
  })

  it("scopes to one post when given a post target", async () => {
    const result = await runCommentActivity(deps(), { post: "pg1_p2", filter: "all" })
    const activity = result as { totals: { threads: number } }

    expect(activity.totals.threads).toBe(2)
  })

  it("resolves a Page token for a post target whose id carries a derivable Page prefix", async () => {
    const fetchImpl = fixtureFetch()
    const result = await runCommentActivity(deps(fetchImpl), { post: "pg1_p2", filter: "all" })

    expect(result).not.toHaveProperty("error")

    const credentialsCall = fetchImpl.mock.calls.find(([url]) => {
      const { pathname, searchParams } = new URL(url as string)
      return (
        pathname.endsWith("/me/accounts") && searchParams.get("fields")?.includes("access_token")
      )
    })
    expect(credentialsCall).toBeDefined()

    const commentsCall = fetchImpl.mock.calls.find(([url]) =>
      (url as string).includes("/pg1_p2/comments"),
    )
    expect(commentsCall).toBeDefined()
    const headers = (commentsCall as [string, RequestInit])[1].headers as Record<string, string>
    expect(headers.authorization).toBe("Bearer PAGE-TOKEN-FIXTURE")

    // The Page was resolved, so this must not carry the user-token caveat.
    const activity = result as { notes: string[] }
    expect(activity.notes.join(" ")).not.toContain("user access token")
  })

  it("falls back to the user token and says so when a post id has no derivable Page prefix", async () => {
    const result = await runCommentActivity(deps(), { post: "12345", filter: "all" })

    expect(result).not.toHaveProperty("error")
    const activity = result as { notes: string[] }
    expect(activity.notes.join(" ")).toContain("user access token")
  })

  it("falls back to the user token, rather than erroring, when the derived Page is inaccessible", async () => {
    // "pg9" is not in the fixture credential map, so forPage("pg9") throws
    // PageAccessError; the derivation must swallow that and fall back.
    const result = await runCommentActivity(deps(), { post: "pg9_p1", filter: "all" })

    expect(result).not.toHaveProperty("error")
    const activity = result as { notes: string[] }
    expect(activity.notes.join(" ")).toContain("user access token")
  })

  it("returns comment text in a labelled untrusted field, never as `message`", async () => {
    const result = await runCommentActivity(deps(), { page: "pg1", filter: "all" })
    const serialized = JSON.stringify(result)

    expect(serialized).toContain('"untrusted":true')
    expect(serialized).not.toContain('"message"')
  })

  it("rejects two targets rather than silently choosing one", async () => {
    const result = await runCommentActivity(deps(), { page: "pg1", post: "pg1_p1" })

    expect(result).toHaveProperty("error")
    expect((result as { error: string }).error).toMatch(/one target/i)
  })

  it("returns an error rather than throwing when the user token cannot be resolved", async () => {
    // TokenProvider is a pluggable seam — a database- or OAuth-backed provider
    // can reject here, and an unhandled rejection would escape the tool.
    const tokens = {
      forUser: async () => {
        throw new Error("token store unavailable")
      },
      forPage: async () => {
        throw new Error("token store unavailable")
      },
      tasksFor: async () => [],
    }
    const result = await runCommentActivity({ ...deps(), tokens }, { comment: "c1" })

    expect(result).toHaveProperty("error")
  })

  it("labels a partial answer when one post's comments fail", async () => {
    const fetchImpl = vi.fn(async (url: string) => {
      const { pathname, searchParams } = new URL(url)
      if (pathname.endsWith("/me/accounts")) {
        const wantsToken = searchParams.get("fields")?.includes("access_token")
        return new Response(fixture(wantsToken ? "page-credentials" : "pages"), { status: 200 })
      }
      if (pathname.endsWith("/feed")) return new Response(fixture("feed"), { status: 200 })
      if (pathname.endsWith("/pg1_p2/comments")) {
        return new Response(JSON.stringify({ error: { message: "boom", code: 1 } }), {
          status: 500,
        })
      }
      return new Response(fixture("comments-p1"), { status: 200 })
    })
    const result = await runCommentActivity(deps(fetchImpl), { page: "pg1", filter: "all" })

    expect(result).not.toHaveProperty("error")
    expect((result as { partial: boolean }).partial).toBe(true)
    expect((result as { notes: string[] }).notes.join(" ")).toContain("pg1_p2")
  })

  it("explains a permission failure instead of passing Graph's message through", async () => {
    const fetchImpl = vi.fn(
      async () =>
        new Response(
          JSON.stringify({ error: { message: "(#200) Permissions error", code: 200 } }),
          {
            status: 403,
          },
        ),
    )
    const result = await runCommentActivity(deps(fetchImpl), { page: "pg1" })

    expect((result as { error: string }).error).toContain("pages_read")
  })

  it("logs the operation without logging comment text", async () => {
    const lines: string[] = []
    const logger = createLogger({ sink: (line) => lines.push(line) })
    await runCommentActivity({ ...deps(), logger }, { page: "pg1", filter: "all" })

    expect(lines.length).toBeGreaterThan(0)
    expect(lines.join(" ")).toContain("read_comments")
    expect(lines.join(" ")).not.toContain("Ireland")
  })

  it("never returns a Page access token for a page target, where the exchange runs", async () => {
    // The Page listing needs no token exchange, so asserting only on it left
    // the path that actually holds Page credentials unchecked.
    const result = await runCommentActivity(deps(), { page: "pg1", filter: "all" })

    expect(result).not.toHaveProperty("error")
    expect(JSON.stringify(result)).not.toContain("PAGE-TOKEN-FIXTURE")
  })

  it("keeps the newest threads, not the oldest, when the cap bites", async () => {
    // Comments arrive oldest-first (order: chronological) and post by post, so
    // an unsorted slice kept the oldest threads from the first post while the
    // note claimed "the most recent are shown".
    const result = await runCommentActivity(deps(datedFetch()), {
      page: "pg1",
      filter: "all",
      maxThreads: 2,
    })
    const activity = result as { totals: { threads: number }; groups: RenderedGroupish[] }

    expect(activity.totals.threads).toBe(2)
    expect(threadIds(activity.groups).sort()).toEqual(["pg1_p2_c1", "pg1_p2_c2"])
  })

  it("orders every returned thread newest first", async () => {
    const result = await runCommentActivity(deps(datedFetch()), { page: "pg1", filter: "all" })
    const groups = (result as { groups: RenderedGroupish[] }).groups
    const times = groups
      .flatMap((g) => g.threads)
      .map((t) => t.comment.createdTime)
      .filter((t): t is string => t !== undefined)

    // Grouping reshuffles, so compare within the group that holds both posts'
    // threads: `none` puts every thread in one ordered list.
    const flat = await runCommentActivity(deps(datedFetch()), {
      page: "pg1",
      filter: "all",
      groupBy: "none",
    })
    const ordered = (flat as { groups: RenderedGroupish[] }).groups[0]!.threads.map(
      (t) => t.comment.createdTime,
    )

    expect(times).toHaveLength(4)
    expect(ordered).toEqual([...ordered].sort().reverse())
  })

  it("sorts threads with no timestamp last rather than throwing", async () => {
    const fetchImpl = commentsFetch({
      pg1_p1: [
        { id: "pg1_p1_c1", message: "undated", comment_count: 0 },
        {
          id: "pg1_p1_c2",
          message: "dated",
          created_time: "2026-08-30T10:00:00+0000",
          comment_count: 0,
        },
      ],
      pg1_p2: [],
    })
    const result = await runCommentActivity(deps(fetchImpl), {
      page: "pg1",
      filter: "all",
      groupBy: "none",
    })

    expect(result).not.toHaveProperty("error")
    const ordered = (result as { groups: RenderedGroupish[] }).groups[0]!.threads.map(
      (t) => t.comment.id,
    )
    expect(ordered).toEqual(["pg1_p1_c2", "pg1_p1_c1"])
  })

  it("reads an ad target's comments with a Page token, not the user token", async () => {
    // Meta answers /comments with empty `data` for a user token, so this
    // returning zero in production is the failure mode, not an error.
    const fetchImpl = fixtureFetch()
    const result = await runCommentActivity(
      { ...deps(fetchImpl), meta: metaStub(["pg1_p2"]) as never },
      { ad: "a1", filter: "all" },
    )

    expect(result).not.toHaveProperty("error")
    const commentsCall = fetchImpl.mock.calls.find(([url]) =>
      (url as string).includes("/pg1_p2/comments"),
    )
    expect(commentsCall).toBeDefined()
    const headers = (commentsCall as [string, RequestInit])[1].headers as Record<string, string>
    expect(headers.authorization).toBe("Bearer PAGE-TOKEN-FIXTURE")

    expect(JSON.stringify(result)).toContain("pg1_p2_c1")
    expect((result as { notes: string[] }).notes.join(" ")).not.toContain("user access token")
  })

  it("reads a campaign target's comments with a Page token too", async () => {
    const fetchImpl = fixtureFetch()
    const result = await runCommentActivity(
      { ...deps(fetchImpl), meta: metaStub(["pg1_p2"]) as never },
      { campaign: "cmp1", filter: "all" },
    )

    expect(result).not.toHaveProperty("error")
    const commentsCall = fetchImpl.mock.calls.find(([url]) =>
      (url as string).includes("/pg1_p2/comments"),
    )
    const headers = (commentsCall as [string, RequestInit])[1].headers as Record<string, string>
    expect(headers.authorization).toBe("Bearer PAGE-TOKEN-FIXTURE")
    expect((result as { totals: { threads: number } }).totals.threads).toBe(2)
  })

  it("says so when a campaign spans more Pages than one token can read", async () => {
    const result = await runCommentActivity(
      { ...deps(), meta: metaStub(["pg1_p2", "pg9_p1"]) as never },
      { campaign: "cmp1", filter: "all" },
    )

    const activity = result as { partial: boolean; notes: string[] }
    expect(activity.partial).toBe(true)
    expect(activity.notes.join(" ")).toMatch(/only one Page access token per call/i)
  })

  it("explains an ad target when the Marketing client is not configured", async () => {
    const result = await runCommentActivity(deps(), { ad: "a1" })

    expect((result as { error: string }).error).toMatch(/Marketing API access/i)
  })

  it("explains a campaign target when the Marketing client is not configured", async () => {
    const result = await runCommentActivity(deps(), { campaign: "cmp1" })

    expect((result as { error: string }).error).toMatch(/Marketing API access/i)
  })

  it("hoists each post body to the response instead of repeating it per thread", async () => {
    const result = await runCommentActivity(deps(), { page: "pg1", filter: "all" })
    const serialized = JSON.stringify(result)
    const activity = result as {
      posts: { id: string; text?: { value: string } }[]
      groups: RenderedGroupish[]
    }

    // Two threads hang off pg1_p2; its body must appear exactly once.
    expect(serialized.split("Dark post used in an ad")).toHaveLength(2)
    expect(activity.posts.map((p) => p.id).sort()).toEqual(["pg1_p1", "pg1_p2"])
    for (const thread of activity.groups.flatMap((g) => g.threads)) {
      expect(thread).not.toHaveProperty("post")
      expect(typeof thread.postId).toBe("string")
    }
  })

  it("holds the whole response inside its rendered-text budget", async () => {
    // 60 maximum-length comments is 60,000 characters of body alone. The
    // shipped version returned all of it.
    const fetchImpl = commentsFetch({
      pg1_p1: Array.from({ length: 60 }, (_, i) => ({
        id: `pg1_p1_c${i}`,
        message: "x".repeat(MAX_COMMENT_CHARS),
        created_time: `2026-08-${String(1 + (i % 28)).padStart(2, "0")}T10:00:00+0000`,
        from: { id: `u${i}`, name: "A Customer" },
        comment_count: 0,
      })),
      pg1_p2: [],
    })
    const result = await runCommentActivity(deps(fetchImpl), { page: "pg1", filter: "all" })
    const activity = result as {
      partial: boolean
      notes: string[]
      groups: RenderedGroupish[]
      totals: { threads: number }
    }

    expect(activity.totals.threads).toBe(60)
    expect(renderedTextChars(result)).toBeLessThanOrEqual(MAX_RESPONSE_TEXT_CHARS)
    expect(activity.partial).toBe(true)
    expect(activity.notes.join(" ")).toMatch(/without their text/i)

    const threads = activity.groups.flatMap((g) => g.threads)
    const abbreviated = threads.filter((t) => t.abbreviated === true)
    expect(abbreviated.length).toBeGreaterThan(0)
    // Structure survives even without the text, so the caller can come back
    // for a specific thread.
    expect(abbreviated[0]!.comment.id).toMatch(/^pg1_p1_c/)
    expect(abbreviated[0]!.comment).not.toHaveProperty("text")
    expect(threads.some((t) => t.abbreviated === undefined)).toBe(true)
  })

  it("spends the budget on the newest threads", async () => {
    const fetchImpl = commentsFetch({
      pg1_p1: Array.from({ length: 60 }, (_, i) => ({
        id: `pg1_p1_c${i}`,
        message: "x".repeat(MAX_COMMENT_CHARS),
        // c0 is oldest, c59 newest.
        created_time: `2026-08-${String(1 + i).padStart(2, "0")}T10:00:00+0000`,
        comment_count: 0,
      })),
      pg1_p2: [],
    })
    const result = await runCommentActivity(deps(fetchImpl), {
      page: "pg1",
      filter: "all",
      groupBy: "none",
    })
    const threads = (result as { groups: RenderedGroupish[] }).groups[0]!.threads

    expect(threads[0]!.abbreviated).toBeUndefined()
    expect(threads.at(-1)!.abbreviated).toBe(true)
  })

  it("does not assert a published state it never read", async () => {
    // A boosted published post would be labelled "unpublished — ad-backed",
    // and an ad post fetched by id would be labelled organic.
    const result = await runCommentActivity(deps(), { post: "pg1_p2", filter: "all" })
    const activity = result as { posts: { id: string }[]; groups: { label: string }[] }

    expect(activity.posts[0]).not.toHaveProperty("isPublished")
    expect(activity.groups[0]!.label).toBe("Post pg1_p2")
  })

  it("says that `since` was not applied to a post target", async () => {
    // `since` is echoed on every response but only reaches Graph for a page
    // sweep, so an echoed value could be read as a filter that ran.
    const result = await runCommentActivity(deps(), {
      post: "pg1_p2",
      filter: "all",
      since: "2026-08-01",
    })

    expect((result as { since: string }).since).toBe("2026-08-01")
    expect((result as { notes: string[] }).notes.join(" ")).toContain(
      "not applied to this post target",
    )
  })

  it("does not use maxThreads as the per-post comment page size", async () => {
    // Spec §3: maxThreads is a returned-volume cap, not a page size. Using it
    // as one meant maxThreads:1 read one comment per post and triaged that.
    const fetchImpl = fixtureFetch()
    await runCommentActivity(deps(fetchImpl), { page: "pg1", filter: "all", maxThreads: 1 })

    const call = fetchImpl.mock.calls.find(([url]) => (url as string).includes("/pg1_p1/comments"))
    const limit = new URL((call as [string, RequestInit])[0]).searchParams.get("limit")
    expect(Number(limit)).toBeGreaterThan(1)
  })

  it(
    "notes when some threads in the final set lack author identity, " +
      "without downgrading the basis",
    async () => {
      // pg1_p1 gets a second comment with no `from` at all — Graph does not
      // always return author identity for every comment even when it does for
      // others. `basis` is computed once for the whole batch (triage.ts), so
      // this single missing author must not flip it to `reply_count`; it must
      // instead surface as a note qualifying the batch-wide confidence claim.
      const fetchImpl = vi.fn(async (url: string) => {
        const { pathname, searchParams } = new URL(url)
        if (pathname.endsWith("/me/accounts")) {
          const wantsToken = searchParams.get("fields")?.includes("access_token")
          return new Response(fixture(wantsToken ? "page-credentials" : "pages"), { status: 200 })
        }
        if (pathname.endsWith("/feed")) return new Response(fixture("feed"), { status: 200 })
        if (pathname.endsWith("/pg1_p1/comments")) {
          return new Response(
            JSON.stringify({
              data: [
                {
                  id: "pg1_p1_c1",
                  message: "Do you ship to Ireland?",
                  created_time: "2026-08-30T10:00:00+0000",
                  from: { id: "u1", name: "A Customer" },
                  comment_count: 0,
                },
                {
                  id: "pg1_p1_c2",
                  message: "Anonymous question",
                  created_time: "2026-08-30T10:05:00+0000",
                  comment_count: 0,
                },
              ],
            }),
            { status: 200 },
          )
        }
        if (pathname.endsWith("/pg1_p2/comments")) {
          return new Response(fixture("comments-p2"), { status: 200 })
        }
        if (pathname.endsWith("/pg1_p2_c1/comments")) {
          return new Response(fixture("replies"), { status: 200 })
        }
        return new Response(JSON.stringify({ data: [] }), { status: 200 })
      })

      const result = await runCommentActivity(deps(fetchImpl), { page: "pg1", filter: "all" })
      const activity = result as { statusBasis: string; notes: string[] }

      expect(activity.statusBasis).toBe("author_identity")
      expect(activity.notes.join(" ")).toContain("carried no author information")
    },
  )
})

/**
 * Threads deeper than two levels, confirmed against live Graph on 2026-09-11
 * (T-11): a reply can itself carry replies, with `parent` pointing at the
 * reply rather than at the top-level comment.
 *
 * The sweep used to stop after one level, so a visitor's answer to the Page's
 * reply was never fetched — and a thread whose customer is still waiting came
 * back as `answered`. That is the one outcome `activity.ts` says this must
 * never produce.
 */
const threeLevelFetch = () =>
  vi.fn(async (url: string) => {
    const { pathname, searchParams } = new URL(url)
    if (pathname.endsWith("/me/accounts")) {
      const wantsToken = searchParams.get("fields")?.includes("access_token")
      return new Response(fixture(wantsToken ? "page-credentials" : "pages"), { status: 200 })
    }
    const json = (data: unknown) => new Response(JSON.stringify({ data }), { status: 200 })
    if (pathname.endsWith("/pg1/feed")) return json([{ id: "pg1_deep", is_published: true }])
    if (pathname.endsWith("/pg1_deep/comments")) {
      return json([
        {
          id: "c_top",
          message: "Visitor asks a question",
          from: { id: "visitor", name: "A Visitor" },
          comment_count: 1,
        },
      ])
    }
    if (pathname.endsWith("/c_top/comments")) {
      return json([
        {
          id: "c_page",
          message: "The Page answers",
          from: { id: "pg1", name: "Page" },
          comment_count: 1,
          parent: { id: "c_top" },
        },
      ])
    }
    if (pathname.endsWith("/c_page/comments")) {
      return json([
        {
          id: "c_back",
          message: "Visitor comes back, still waiting",
          from: { id: "visitor", name: "A Visitor" },
          comment_count: 0,
          parent: { id: "c_page" },
        },
      ])
    }
    return json([])
  })

describe("threads deeper than two levels", () => {
  it("fetches the replies of a reply that reports its own replies", async () => {
    const fetchImpl = threeLevelFetch()
    await runCommentActivity(deps(fetchImpl), { page: "pg1", filter: "all" })

    const asked = fetchImpl.mock.calls.map(([url]) => new URL(url as string).pathname)
    expect(asked.some((p) => p.endsWith("/c_page/comments"))).toBe(true)
  })

  it("does not report a thread answered when the visitor spoke last", async () => {
    // The failure this exists for: the Page replied, the customer replied
    // again, and the tool said the thread was handled.
    const result = await runCommentActivity(deps(threeLevelFetch()), {
      page: "pg1",
      filter: "needs_reply",
    })

    expect(JSON.stringify(result)).toContain("still waiting")
  })
})
