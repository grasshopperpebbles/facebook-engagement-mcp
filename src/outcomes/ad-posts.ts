import type { MetaClient } from "../vendor/meta-client/index.js"

/**
 * Resolve an ad or campaign to the Page posts behind it.
 *
 * Ad → creative → `effective_object_story_id` → Page post. This exists only in
 * the in-repo server, which already has a Marketing client; the standalone
 * takes a post ID directly.
 */
export async function resolveAdPosts(
  meta: MetaClient,
  target: { ad?: string | undefined; campaign?: string | undefined },
): Promise<{ postIds: string[]; notes: string[] }> {
  const notes: string[] = []
  let adIds: string[]

  if (target.ad !== undefined) {
    adIds = [target.ad]
  } else if (target.campaign !== undefined) {
    const { items: adSets } = await meta.adSets.list(target.campaign, { maxItems: 100 })
    const collected: string[] = []
    for (const adSet of adSets) {
      const { items } = await meta.ads.list(adSet.id, { maxItems: 100 })
      collected.push(...items.map((ad) => ad.id))
    }
    adIds = collected
  } else {
    throw new Error("Give an ad or campaign to resolve.")
  }

  const postIds = new Set<string>()
  for (const adId of adIds) {
    const creative = await meta.creatives.forAd(adId)
    const postId = creative.effective_object_story_id ?? creative.object_story_id
    if (postId === undefined) {
      // Some formats never produce a Page post, so their comments are
      // unreachable from the Page side. An empty answer would look like
      // "no comments" rather than "not visible here".
      notes.push(
        `Ad ${adId} has no Page post behind it, so its comments cannot be read from the Page. ` +
          "Some ad formats do not create a Page post object.",
      )
      continue
    }
    postIds.add(postId)
  }

  return { postIds: [...postIds], notes }
}
