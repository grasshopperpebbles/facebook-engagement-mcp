"""The Marketing API surface — read-only, and only as much of it as ad comments need.

**This is not a convenience over the Page sweep. It is the only route to a
comment on an ad.** `/feed` does not return unpublished posts (verified against a
live Page on 2026-09-03, against Meta's documentation saying it does) and ads run
on unpublished posts, so a build without this surface cannot reach a comment on
an ad at all — which is the capability the product headlines.

Writes are deliberately absent. Campaign creation and budget mutation spend real
money and no outcome server has a reason to reach them.
"""

from __future__ import annotations

from dataclasses import dataclass

from .http import Transport

ACCOUNT_FIELDS = "id,name"
CAMPAIGN_FIELDS = "id,name,status,effective_status,objective"
AD_SET_FIELDS = "id,name"
AD_FIELDS = "id,name"
AD_CREATIVE_FIELDS = "id,effective_object_story_id,object_story_id"


@dataclass(frozen=True)
class AdAccount:
    id: str
    name: str | None = None


@dataclass(frozen=True)
class Campaign:
    id: str
    name: str | None = None
    status: str | None = None
    effective_status: str | None = None
    objective: str | None = None


class MarketingClient:
    def __init__(self, transport: Transport) -> None:
        self._transport = transport

    async def ad_accounts(self, *, max_items: int = 100) -> list[AdAccount]:
        items, _ = await self._transport.get_all(
            "/me/adaccounts", max_items=max_items, params={"fields": ACCOUNT_FIELDS}
        )
        return [AdAccount(id=i["id"], name=i.get("name")) for i in items]

    async def campaigns(self, account_id: str, *, max_items: int = 100
                        ) -> tuple[list[Campaign], bool]:
        items, truncated = await self._transport.get_all(
            f"/{account_id}/campaigns", max_items=max_items, params={"fields": CAMPAIGN_FIELDS}
        )
        return [
            Campaign(
                id=i["id"],
                name=i.get("name"),
                status=i.get("status"),
                # `effective_status` is the one worth showing — it accounts for
                # the ad set and account above it. A paused campaign reports
                # PAUSED; `CAMPAIGN_PAUSED` was invented when the fixtures were
                # hand-written and never existed (corrected 2026-09-08).
                effective_status=i.get("effective_status"),
                objective=i.get("objective"),
            )
            for i in items
        ], truncated

    async def ad_sets(self, campaign_id: str, *, max_items: int = 100) -> list[str]:
        items, _ = await self._transport.get_all(
            f"/{campaign_id}/adsets", max_items=max_items, params={"fields": AD_SET_FIELDS}
        )
        return [i["id"] for i in items]

    async def ads(self, ad_set_id: str, *, max_items: int = 100) -> list[str]:
        items, _ = await self._transport.get_all(
            f"/{ad_set_id}/ads", max_items=max_items, params={"fields": AD_FIELDS}
        )
        return [i["id"] for i in items]

    async def creative_for_ad(self, ad_id: str) -> dict[str, str] | None:
        body = await self._transport.get(
            f"/{ad_id}", {"fields": f"creative{{{AD_CREATIVE_FIELDS}}}"}
        )
        return body.get("creative")
