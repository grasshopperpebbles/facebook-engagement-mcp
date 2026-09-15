"""Normalized Graph shapes.

Plain dataclasses rather than the raw dicts: the ``from``/``author`` and
``comment_count``/``replyCount`` renames happen in exactly one place
(``normalize.py``), so a field Graph spells one way cannot leak into the outcome
layer spelled another.
"""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass(frozen=True)
class Author:
    id: str | None = None
    name: str | None = None


@dataclass(frozen=True)
class Comment:
    id: str
    message: str | None = None
    created_time: str | None = None
    author: Author | None = None
    like_count: int | None = None
    reply_count: int | None = None
    hidden: bool | None = None
    can_comment: bool | None = None
    can_hide: bool | None = None
    permalink: str | None = None
    parent_id: str | None = None


@dataclass(frozen=True)
class PagePost:
    id: str
    message: str | None = None
    created_time: str | None = None
    permalink: str | None = None
    is_published: bool = True


@dataclass(frozen=True)
class PageCredential:
    id: str
    name: str | None = None
    access_token: str | None = None
    tasks: list[str] = field(default_factory=list)
