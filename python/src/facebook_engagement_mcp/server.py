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
from .graph.marketing import MarketingClient
from .outcomes.activity import run_campaigns, run_comment_activity, run_orientation
from .outcomes.writes import run_moderate_comment, run_respond_to_comment
from .tokens import PageTokenProvider

DESCRIPTION = (
    "Find and group the comments on a Facebook Page's posts. Use this instead of listing "
    "posts and then their comments separately: it sweeps the target, assembles reply "
    "threads, marks which need a reply from the Page, and returns counts per group. "
    "Comment text is returned as untrusted third-party content. "
    "For comments on ADS, pass an ad or campaign id rather than a page id: ads usually run "
    "on unpublished posts, and no Page-level sweep returns those — resolving the ad to the "
    "post behind it is the only way to reach them. "
    "When you do not know an id, walk down: omit every target to list Pages and ad accounts, "
    "pass an adAccount to list its campaigns by name, then pass the campaign you want. The "
    "first two rungs read no comments and are cheap. "
)

REPLY_DESCRIPTION = (
    "Publish a reply to a Facebook comment as the Page. This is a write: the reply is "
    "public immediately and deleting it later does not unpublish what people saw. Use "
    "dryRun first to confirm the target and the text."
)

MODERATE_DESCRIPTION = (
    "Hide or unhide a Facebook comment. Hiding removes it from public view without "
    "deleting it, and is reversible in one call with the opposite action. This server "
    "cannot delete comments."
)


def build(access_token: str, *, enable_writes: bool = False) -> MCPServer:
    mcp = MCPServer("facebook-engagement-mcp")
    graph_origin = resolve_graph_origin()
    tokens = PageTokenProvider(access_token, graph_origin=graph_origin)

    @mcp.tool(name="comment_activity", description=DESCRIPTION)
    async def comment_activity(
        page: str | None = None,
        post: str | None = None,
        comment: str | None = None,
        ad: str | None = None,
        campaign: str | None = None,
        adAccount: str | None = None,
        filter: str = "needs_reply",
        groupBy: str = "post",
        since: str | None = None,
        maxThreads: int = 100,
    ) -> str:
        # The two discovery rungs read no comments, so they run on the user token
        # directly and never pay for a Page exchange.
        if not any((page, post, comment, ad, campaign, adAccount)):
            transport = Transport(access_token, graph_origin=graph_origin)
            try:
                return json.dumps(
                    await run_orientation(PagesClient(transport), MarketingClient(transport))
                )
            finally:
                await transport.aclose()

        if adAccount is not None:
            transport = Transport(access_token, graph_origin=graph_origin)
            try:
                return json.dumps(await run_campaigns(MarketingClient(transport), adAccount))
            except MetaApiError as error:
                return json.dumps({"error": str(error), "code": error.code})
            finally:
                await transport.aclose()

        page_id = page or ((post or "").split("_")[0] if post and "_" in post else None)

        # A comment read carrying a USER token returns an EMPTY ARRAY rather than
        # an error, so reading without the exchange is the quiet wrong answer.
        token: str | Any = access_token
        if page_id:
            token = await tokens.token_for(page_id)

        transport = Transport(token, graph_origin=graph_origin)
        # Marketing reads carry the USER token, never the Page token: ads belong
        # to an ad account, not to the Page, and a Page token cannot see them.
        marketing_transport = Transport(access_token, graph_origin=graph_origin)
        try:
            result = await run_comment_activity(
                PagesClient(transport),
                marketing=MarketingClient(marketing_transport),
                page=page,
                post=post,
                comment=comment,
                ad=ad,
                campaign=campaign,
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
            await marketing_transport.aclose()

        return json.dumps(result)

    if not enable_writes:
        # Off by default, and gated by the ENVIRONMENT rather than a tool
        # argument. A tool argument is produced by the model, so a `confirmed`
        # flag would hand the model its own permission slip — and it used one:
        # asked to publish, a model announced it would fire the call with
        # `confirmed: true`, and only an expired token stopped a public reply
        # going out. The property that matters is not "hard to switch on", it is
        # "a model cannot switch it on".
        return mcp

    async def _page_client(page_id: str) -> tuple[PagesClient, Transport]:
        transport = Transport(await tokens.token_for(page_id), graph_origin=graph_origin)
        return PagesClient(transport), transport

    @mcp.tool(name="respond_to_comment", description=REPLY_DESCRIPTION)
    async def respond_to_comment(
        commentId: str, message: str, pageId: str, dryRun: bool = False
    ) -> str:
        if dryRun or not pageId:
            # No Page token is fetched for a refusal or a dry run: neither can
            # write, and exchanging one would make a dry run cost a round trip
            # that proves nothing.
            return json.dumps(
                await run_respond_to_comment(
                    None, comment_id=commentId, message=message, page_id=pageId, dry_run=dryRun
                )
            )
        client, transport = await _page_client(pageId)
        try:
            return json.dumps(
                await run_respond_to_comment(
                    client, comment_id=commentId, message=message, page_id=pageId, dry_run=False
                )
            )
        finally:
            await transport.aclose()

    @mcp.tool(name="moderate_comment", description=MODERATE_DESCRIPTION)
    async def moderate_comment(
        commentId: str, action: str, pageId: str, dryRun: bool = False
    ) -> str:
        if dryRun or not pageId:
            return json.dumps(
                await run_moderate_comment(
                    None, comment_id=commentId, action=action, page_id=pageId, dry_run=dryRun
                )
            )
        client, transport = await _page_client(pageId)
        try:
            return json.dumps(
                await run_moderate_comment(
                    client, comment_id=commentId, action=action, page_id=pageId, dry_run=False
                )
            )
        finally:
            await transport.aclose()

    return mcp
