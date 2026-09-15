from __future__ import annotations

from typing import Any

from .types import Author, Comment, PageCredential, PagePost


def normalize_comment(raw: dict[str, Any]) -> Comment:
    author = raw.get("from")
    return Comment(
        id=raw["id"],
        message=raw.get("message"),
        created_time=raw.get("created_time"),
        author=Author(id=author.get("id"), name=author.get("name")) if author else None,
        like_count=raw.get("like_count"),
        reply_count=raw.get("comment_count"),
        hidden=raw.get("is_hidden"),
        can_comment=raw.get("can_comment"),
        can_hide=raw.get("can_hide"),
        permalink=raw.get("permalink_url"),
        parent_id=(raw.get("parent") or {}).get("id"),
    )


def normalize_post(raw: dict[str, Any]) -> PagePost:
    return PagePost(
        id=raw["id"],
        message=raw.get("message"),
        created_time=raw.get("created_time"),
        permalink=raw.get("permalink_url"),
        # Defaulting to True matches the TypeScript client. `/feed` never
        # returns an unpublished post at all (T-02), so this default describes
        # what the edge can actually contain.
        is_published=raw.get("is_published", True),
    )


def normalize_credential(raw: dict[str, Any]) -> PageCredential:
    return PageCredential(
        id=raw["id"],
        name=raw.get("name"),
        access_token=raw.get("access_token"),
        tasks=list(raw.get("tasks") or []),
    )
