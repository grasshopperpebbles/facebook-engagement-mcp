"""Untrusted free text, moved into labelled, delimited fields.

Comment text and author names are written by the public, and this capability
also holds tools that publish and hide. Structural separation does not stop a
model being persuaded by content it legitimately reads — that non-defence is
documented — but it stops the text being mistaken for structure.
"""

from __future__ import annotations

from typing import Any

from ..graph.types import Comment

MAX_COMMENT_CHARS = 1000
#: A display name is free text chosen by whoever wrote the comment, so it is
#: exactly as untrusted as the body. Past any real name, far short of a
#: paragraph of smuggled instructions.
MAX_AUTHOR_NAME_CHARS = 80
#: Total free text one response may carry. The differentiation argument is
#: context economy: triaging 400 comments server-side should cost the groups,
#: not the comments.
MAX_RESPONSE_TEXT_CHARS = 40_000


def untrusted_text(value: str, limit: int = MAX_COMMENT_CHARS) -> dict[str, Any]:
    return {"untrusted": True, "value": value[:limit], "truncated": len(value) > limit}


class TextBudget:
    """A drawdown allowance shared across everything in one response.

    ``take`` is all-or-nothing on purpose: half a comment body is not a useful
    thing to return, and a partially-spent thread would have to be described as
    both complete and abbreviated.
    """

    def __init__(self, total: int = MAX_RESPONSE_TEXT_CHARS) -> None:
        self.remaining = max(0, total)

    def take(self, cost: int) -> bool:
        if cost > self.remaining:
            return False
        self.remaining -= cost
        return True


def comment_text_cost(comment: Comment) -> int:
    body = 0 if comment.message is None else min(len(comment.message), MAX_COMMENT_CHARS)
    name = 0
    if comment.author and comment.author.name is not None:
        name = min(len(comment.author.name), MAX_AUTHOR_NAME_CHARS)
    return body + name


def _base(comment: Comment) -> dict[str, Any]:
    fields = {
        "id": comment.id,
        "createdTime": comment.created_time,
        "likeCount": comment.like_count,
        "replyCount": comment.reply_count,
        "hidden": comment.hidden,
        "canComment": comment.can_comment,
        "canHide": comment.can_hide,
        "permalink": comment.permalink,
        "parentId": comment.parent_id,
    }
    return {k: v for k, v in fields.items() if v is not None}


def render_comment(comment: Comment) -> dict[str, Any]:
    out = _base(comment)
    if comment.message is not None:
        out["text"] = untrusted_text(comment.message)
    if comment.author is not None:
        author: dict[str, Any] = {}
        if comment.author.id is not None:
            author["id"] = comment.author.id
        if comment.author.name is not None:
            author["name"] = untrusted_text(comment.author.name, MAX_AUTHOR_NAME_CHARS)
        out["author"] = author
    return out


def abbreviate_comment(comment: Comment) -> dict[str, Any]:
    """The same comment with all free text withheld.

    Used once a response has spent its budget: the caller still gets the id,
    counts, timestamps and author id — enough to count, group and act on the
    thread — without the body.
    """
    out = _base(comment)
    if comment.author is not None and comment.author.id is not None:
        out["author"] = {"id": comment.author.id}
    return out
