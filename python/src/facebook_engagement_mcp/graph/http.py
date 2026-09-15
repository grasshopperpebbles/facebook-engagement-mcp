"""The Graph transport: URL building, pagination, and the origin pin.

Three behaviours here are not obvious and each comes from a defect found against
live Graph. They are asserted by the conformance suite, so a port that gets them
wrong fails rather than passing quietly.
"""

from __future__ import annotations

import re
from collections.abc import AsyncIterator, Awaitable, Callable
from typing import Any
from urllib.parse import urlparse

import httpx

from ..config import GRAPH_API_VERSION, resolve_graph_origin
from .errors import MetaApiError

DEFAULT_PAGE_SIZE = 100
DEFAULT_MAX_PAGES = 25
DEFAULT_TIMEOUT_S = 30.0

_VERSION_SEGMENT = re.compile(r"^/v\d+\.\d+/")

TokenResolver = Callable[[], Awaitable[str]]


class Transport:
    def __init__(
        self,
        access_token: str | TokenResolver,
        *,
        client: httpx.AsyncClient | None = None,
        graph_origin: str | None = None,
        max_pages: int = DEFAULT_MAX_PAGES,
    ) -> None:
        if isinstance(access_token, str) and not access_token:
            raise ValueError("A Meta access token is required")
        self._access_token = access_token
        # Resolved once and read by BOTH the URL builder and the paging.next pin
        # below. They cannot disagree, which is the point.
        self.graph_origin = graph_origin or resolve_graph_origin()
        self._client = client or httpx.AsyncClient(timeout=DEFAULT_TIMEOUT_S)
        self._max_pages = max_pages

    async def _token(self) -> str:
        token = self._access_token if isinstance(self._access_token, str) else await self._access_token()
        if not token:
            raise ValueError("A Meta access token is required")
        return token

    def _url(self, path: str, params: dict[str, Any] | None = None) -> httpx.URL:
        url = httpx.URL(f"{self.graph_origin}/{GRAPH_API_VERSION}{path}")
        clean = {k: str(v) for k, v in (params or {}).items() if v is not None}
        return url.copy_merge_params(clean) if clean else url

    async def _request(self, url: httpx.URL | str) -> dict[str, Any]:
        # A Bearer header, never an access_token query parameter: query strings
        # are recorded by proxies, CDNs and server logs; headers are not.
        response = await self._client.get(
            url,
            headers={"authorization": f"Bearer {await self._token()}", "accept": "application/json"},
        )
        try:
            body = response.json()
        except ValueError:
            body = None
        if response.status_code >= 400:
            raise MetaApiError.from_body(response.status_code, body)
        return body or {}

    async def get(self, path: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
        return await self._request(self._url(path, params))

    def _next_page_url(self, body: dict[str, Any]) -> str | None:
        """``paging.next`` is a URL taken from a response body and followed with
        the token attached, so it is pinned to one origin — the same one requests
        go to, never a separate setting.

        The version segment is rewritten back to the pin. Meta builds ``next``
        and does not build it on the version you asked for: observed live on
        2026-09-14 (T-07), request 1 went to v25.0 and its ``next`` came back on
        v26.0, so one result set was being assembled across two API versions —
        the single thing pinning exists to prevent. The cursor is preserved
        exactly; only the version segment moves.
        """
        nxt = (body.get("paging") or {}).get("next")
        if not nxt:
            return None
        parsed = urlparse(nxt)
        pinned = urlparse(self.graph_origin)
        if (parsed.scheme, parsed.netloc) != (pinned.scheme, pinned.netloc):
            raise MetaApiError(
                f"Refusing to follow paging.next to a different origin "
                f"({parsed.scheme}://{parsed.netloc}). The access token is only ever sent to "
                f"{self.graph_origin}.",
                status=0,
            )
        path = _VERSION_SEGMENT.sub(f"/{GRAPH_API_VERSION}/", parsed.path)
        return parsed._replace(path=path).geturl()

    async def _pages(self, first: httpx.URL | str) -> AsyncIterator[dict[str, Any]]:
        url: httpx.URL | str | None = first
        for _ in range(self._max_pages):
            if url is None:
                return
            body = await self._request(url)
            yield body
            url = self._next_page_url(body)

    async def get_all(
        self, path: str, *, max_items: int | None = None, params: dict[str, Any] | None = None
    ) -> tuple[list[dict[str, Any]], bool]:
        """Every item across pages, and whether more existed beyond what was returned.

        Returns the truncation flag rather than a bare list on purpose: a short
        list that looks complete is how a triage tool reports a confident zero.
        """
        items: list[dict[str, Any]] = []
        merged = dict(params or {})
        merged.setdefault("limit", DEFAULT_PAGE_SIZE)
        async for body in self._pages(self._url(path, merged)):
            for item in body.get("data") or []:
                items.append(item)
                if max_items is not None and len(items) >= max_items:
                    return items, True
        return items, False

    async def post(self, path: str, params: dict[str, str]) -> dict[str, Any]:
        """Form-encoded POST. Parameters go in the BODY, never the query string:
        comment text is user data and must not reach proxy or CDN logs.

        Never retried, anywhere. A retried reply is a double post, and a comment
        published twice cannot be unpublished from the people who saw it.
        """
        response = await self._client.post(
            self._url(path),
            data=params,
            headers={"authorization": f"Bearer {await self._token()}",
                     "accept": "application/json"},
        )
        try:
            body = response.json()
        except ValueError:
            body = None
        if response.status_code >= 400:
            raise MetaApiError.from_body(response.status_code, body)
        return body or {}

    async def aclose(self) -> None:
        await self._client.aclose()
