"""The user-token → Page-token exchange.

This exists because of the single most confusing failure in this API: **a
comment read with a USER token returns an empty array, not an error** (T-02). A
server that skipped the exchange would report "no comments" on a Page full of
them, and nothing would say why.
"""

from __future__ import annotations

from .graph import PagesClient, Transport


class PageTokenProvider:
    def __init__(self, user_token: str, *, graph_origin: str | None = None) -> None:
        self._user_token = user_token
        self._graph_origin = graph_origin
        self._cache: dict[str, str] = {}

    async def token_for(self, page_id: str) -> str:
        if page_id in self._cache:
            return self._cache[page_id]

        transport = Transport(self._user_token, graph_origin=self._graph_origin)
        try:
            credentials = await PagesClient(transport).credentials()
        finally:
            await transport.aclose()

        for credential in credentials:
            if credential.id == page_id and credential.access_token:
                self._cache[page_id] = credential.access_token
                return credential.access_token

        # Two different answers, kept apart. Folding them into one is how a
        # confident wrong diagnosis got out once already: administration blamed
        # for a Page that was plainly reachable.
        known = [c.id for c in credentials]
        if page_id in known:
            raise RuntimeError(
                f"Page {page_id} was returned by /me/accounts with no access_token. That is a "
                "withheld token, not a missing Page — usually a granular permission scoped to "
                "other Pages."
            )
        raise RuntimeError(
            f"Page {page_id} is not among the Pages this token reaches ({', '.join(known) or 'none'})."
        )
