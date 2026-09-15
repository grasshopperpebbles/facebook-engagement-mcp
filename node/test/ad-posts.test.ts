import { describe, expect, it, vi } from "vitest"
import { resolveAdPosts } from "../src/outcomes/ad-posts.js"

/** Minimal MetaClient stub — only the members resolveAdPosts touches. */
const metaStub = (creative: unknown, ads: { id: string }[] = [{ id: "a1" }]) => ({
  ads: { list: vi.fn(async () => ({ items: ads, truncated: false })) },
  adSets: { list: vi.fn(async () => ({ items: [{ id: "as1" }], truncated: false })) },
  creatives: { forAd: vi.fn(async () => creative) },
})

describe("resolveAdPosts", () => {
  it("resolves an ad to its page-backed post id", async () => {
    const meta = metaStub({ effective_object_story_id: "pg1_p2" })
    const result = await resolveAdPosts(meta as never, { ad: "a1" })

    expect(result.postIds).toEqual(["pg1_p2"])
  })

  it("falls back to object_story_id when the effective id is absent", async () => {
    const meta = metaStub({ object_story_id: "pg1_p3" })
    const result = await resolveAdPosts(meta as never, { ad: "a1" })

    expect(result.postIds).toEqual(["pg1_p3"])
  })

  it("notes an ad format that produces no Page post rather than returning zero silently", async () => {
    // Some ad formats have no Page post object at all, so their comments are
    // unreachable from the Page side. Saying so beats an empty answer.
    const meta = metaStub({})
    const result = await resolveAdPosts(meta as never, { ad: "a1" })

    expect(result.postIds).toEqual([])
    expect(result.notes.join(" ")).toMatch(/no Page post/i)
  })

  it("resolves a campaign to the distinct posts behind its ads", async () => {
    const meta = metaStub({ effective_object_story_id: "pg1_p2" }, [{ id: "a1" }, { id: "a2" }])
    const result = await resolveAdPosts(meta as never, { campaign: "c1" })

    expect(result.postIds).toEqual(["pg1_p2"])
  })

  it("requires exactly one of ad or campaign", async () => {
    const meta = metaStub({})

    await expect(resolveAdPosts(meta as never, {})).rejects.toThrow(/ad or campaign/i)
  })
})
