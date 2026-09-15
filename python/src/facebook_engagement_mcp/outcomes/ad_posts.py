"""Resolve an ad or campaign to the Page posts behind it."""

from __future__ import annotations

from ..graph.marketing import MarketingClient


async def resolve_ad_posts(
    marketing: MarketingClient, *, ad: str | None = None, campaign: str | None = None
) -> tuple[list[str], list[str]]:
    """Ad → creative → ``effective_object_story_id`` → Page post.

    Returns the post ids and any notes. A post reached this way is one no
    Page-level sweep would have found, which is the entire reason this path
    exists (T-02).
    """
    notes: list[str] = []

    if ad is not None:
        ad_ids = [ad]
    elif campaign is not None:
        ad_ids = []
        for ad_set_id in await marketing.ad_sets(campaign):
            ad_ids.extend(await marketing.ads(ad_set_id))
    else:
        raise ValueError("Give an ad or campaign to resolve.")

    post_ids: list[str] = []
    for ad_id in ad_ids:
        creative = await marketing.creative_for_ad(ad_id) or {}
        post_id = creative.get("effective_object_story_id") or creative.get("object_story_id")
        if not post_id:
            # Some formats never produce a Page post, so their comments are
            # unreachable from the Page side. An empty answer would look like
            # "no comments" rather than "not visible here", and a marketer told
            # zero stops looking.
            notes.append(
                f"Ad {ad_id} has no Page post behind it, so its comments cannot be read from "
                "the Page. Some ad formats do not create a Page post object."
            )
            continue
        if post_id not in post_ids:
            post_ids.append(post_id)

    return post_ids, notes
