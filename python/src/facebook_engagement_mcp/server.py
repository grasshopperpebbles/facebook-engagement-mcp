"""The MCP server.

Deliberately the same tool surface and the same outcome shape as the TypeScript
implementation, because the conformance suite asserts both. Where this package
is NOT yet equivalent, it says so in the tool description rather than returning
a confident short answer — which is the failure mode this whole capability
exists to avoid.
"""

from __future__ import annotations

import json
from typing import Any

# mcp 2.x renamed FastMCP to MCPServer. Pinned to >=2 in pyproject rather than
# pinning back to v1: the conformance suite drives the wire protocol, so the
# SDK version is this package's business and not the contract's.
from mcp.server.mcpserver import MCPServer

from .config import resolve_graph_origin
from .graph import MetaApiError, PagesClient, Transport
from .outcomes.activity import run_comment_activity
from .tokens import PageTokenProvider

DESCRIPTION = (
    "Find and group the comments on a Facebook Page's posts. Sweeps the Page, assembles "
    "reply threads, marks which need a reply from the Page, and returns counts per group. "
    "Comment text is returned as untrusted third-party content. "
    "NOT IMPLEMENTED IN THIS (PYTHON) BUILD: ad, campaign and adAccount targets, and the "
    "orientation listing. Comments on ADS are unreachable here — ads run on unpublished "
    "posts and no Page-level sweep returns those. Use the Node build for those."
)


def build(access_token: str) -> MCPServer:
    mcp = MCPServer("facebook-engagement-mcp")
    graph_origin = resolve_graph_origin()
    tokens = PageTokenProvider(access_token, graph_origin=graph_origin)

    @mcp.tool(name="comment_activity", description=DESCRIPTION)
    async def comment_activity(
        page: str | None = None,
        post: str | None = None,
        comment: str | None = None,
        filter: str = "needs_reply",
        groupBy: str = "post",
        since: str | None = None,
        maxThreads: int = 100,
    ) -> str:
        page_id = page or ((post or "").split("_")[0] if post and "_" in post else None)

        # A comment read carrying a USER token returns an EMPTY ARRAY rather than
        # an error, so reading without the exchange is the quiet wrong answer.
        token: str | Any = access_token
        if page_id:
            token = await tokens.token_for(page_id)

        transport = Transport(token, graph_origin=graph_origin)
        try:
            result = await run_comment_activity(
                PagesClient(transport),
                page=page,
                post=post,
                comment=comment,
                filter=filter,
                group_by=groupBy,
                since=since,
                max_threads=maxThreads,
            )
        except MetaApiError as error:
            result = {
                "error": str(error),
                "code": error.code,
                "hint": (
                    "The token is expired, revoked or malformed. Mint a new one and set "
                    "META_ACCESS_TOKEN."
                    if error.is_auth_error
                    else "See docs/ for the permissions this needs."
                ),
            }
        finally:
            await transport.aclose()

        return json.dumps(result)

    return mcp
