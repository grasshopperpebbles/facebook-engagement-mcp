"""The two write tools.

Both take ``page_id`` as **required**, and that is the fix for a full day lost
(T-26, 2026-09-08). They used to accept it optionally and fall back to the
startup client, which holds the USER token — so a caller supplying only a comment
id got Meta refusing the write by naming ``publish_actions``, a permission
removed in 2018. That message is literally true and entirely misleading: the
fault is the identity, not the permission, and no App Review can grant a dead
scope. Refusing here costs one round trip and says the true thing.

There is no delete tool, deliberately. Admitting one requires a decision record,
not a flag.
"""

from __future__ import annotations

from typing import Any

from ..graph import MetaApiError, PagesClient

NO_PAGE_ID = (
    "A reply is published as the Page, so it needs a Page token. Supply `pageId` for the "
    "Page that owns this comment. Without it the call would go out as the user, which Meta "
    "refuses with a message about `publish_actions` — a permission removed in 2018."
)


async def run_respond_to_comment(
    client: PagesClient, *, comment_id: str, message: str,
    page_id: str | None = None, dry_run: bool = False,
) -> dict[str, Any]:
    text = message.strip()
    if not text:
        return {"error": "A reply needs a non-empty message."}
    if not page_id:
        return {"error": NO_PAGE_ID}

    # Checked AFTER the pageId guard, so a dry run can never report that a call
    # would work when it could not.
    if dry_run:
        # The shape of the effect, never the text itself — the caller wrote it and
        # echoing it back costs context for nothing.
        return {"dryRun": True,
                "intended": {"commentId": comment_id, "messageLength": len(text),
                             "visibility": "public"}}

    try:
        created = await client.reply(comment_id, text)
    except MetaApiError as error:
        return {"error": str(error), "code": error.code}
    return {"ok": True, "replyId": created.get("id"), "commentId": comment_id}


async def run_moderate_comment(
    client: PagesClient, *, comment_id: str, action: str,
    page_id: str | None = None, dry_run: bool = False,
) -> dict[str, Any]:
    if action not in ("hide", "unhide"):
        return {"error": "action must be 'hide' or 'unhide'."}
    if not page_id:
        return {"error": NO_PAGE_ID.replace("A reply is published as", "Hiding acts as")}

    if dry_run:
        return {"dryRun": True, "intended": {"commentId": comment_id, "action": action}}

    try:
        await client.set_hidden(comment_id, action == "hide")
    except MetaApiError as error:
        return {"error": str(error), "code": error.code}
    return {"ok": True, "commentId": comment_id, "action": action}
