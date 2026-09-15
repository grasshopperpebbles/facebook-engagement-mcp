"""The Pages and comments surface."""

from __future__ import annotations

from .http import Transport
from .normalize import normalize_comment, normalize_credential, normalize_post
from .types import Comment, PageCredential, PagePost

POST_FIELDS = "id,message,created_time,permalink_url,is_published"
COMMENT_FIELDS = (
    "id,message,created_time,from,like_count,comment_count,is_hidden,"
    "can_comment,can_hide,permalink_url,parent"
)


class PagesClient:
    def __init__(self, transport: Transport) -> None:
        self._transport = transport

    async def credentials(self) -> list[PageCredential]:
        """The Pages this identity administers, with a Page token for each.

        A row with no token is KEPT rather than dropped. Dropping it folds two
        different answers — "no such Page" and "that Page returned no token" —
        into one, and the folding is what let a confident wrong diagnosis out
        once already: administration blamed for a Page that was plainly
        reachable.
        """
        items, _ = await self._transport.get_all(
            "/me/accounts", params={"fields": "id,name,access_token,tasks"}
        )
        return [normalize_credential(item) for item in items]

    async def feed(self, page_id: str, *, since: str | None = None, max_items: int | None = None
                   ) -> list[PagePost]:
        """The Page's published posts.

        This does NOT reach comments on ads. `/feed` does not return unpublished
        posts — tested against a live Page on 2026-09-03, against Meta's
        documentation saying it does — and ads run on unpublished posts, so no
        Page-level sweep reaches them (T-02). Resolving the ad to the post behind
        it is the only route, and it is not implemented in this package yet; see
        the README.
        """
        params: dict[str, object] = {"fields": POST_FIELDS}
        if since:
            params["since"] = since
        items, _ = await self._transport.get_all("/" + page_id + "/feed", max_items=max_items, params=params)
        return [normalize_post(item) for item in items]

    async def comments_for_post(self, post_id: str, *, max_items: int | None = None) -> list[Comment]:
        items, _ = await self._transport.get_all(
            f"/{post_id}/comments",
            max_items=max_items,
            params={"fields": COMMENT_FIELDS, "filter": "toplevel", "order": "chronological"},
        )
        return [normalize_comment(item) for item in items]

    async def replies(self, comment_id: str, *, max_items: int | None = None) -> list[Comment]:
        items, _ = await self._transport.get_all(
            f"/{comment_id}/comments",
            max_items=max_items,
            params={"fields": COMMENT_FIELDS, "order": "chronological"},
        )
        return [normalize_comment(item) for item in items]

    async def reply(self, comment_id: str, message: str) -> dict[str, object]:
        return await self._transport.post(f"/{comment_id}/comments", {"message": message})

    async def set_hidden(self, comment_id: str, hidden: bool) -> dict[str, object]:
        """Hide and unhide are one endpoint and one boolean, which is why this is
        one method rather than two."""
        return await self._transport.post(f"/{comment_id}", {"is_hidden": str(hidden).lower()})
