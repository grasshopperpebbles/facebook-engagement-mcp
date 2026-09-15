"""Thread assembly and ordering.

``in_time_order`` and ``last_word`` exist because of T-36. A thread is built
from several Graph calls and the concatenation is NOT chronological, so "who
spoke last" cannot be read off the end of an array.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime

from ..graph.types import Comment, PagePost


@dataclass
class ThreadPost:
    id: str
    #: ``None`` when it is not known. Only a ``/feed`` sweep reads the post node,
    #: so a post reached by id has no published state to report, and an asserted
    #: default would mislabel a boosted published post as ad-backed. Omission
    #: says "not checked"; it never means "published".
    is_published: bool | None = None
    message: str | None = None
    permalink: str | None = None


@dataclass
class Thread:
    comment: Comment
    replies: list[Comment] = field(default_factory=list)
    post: ThreadPost = field(default_factory=lambda: ThreadPost(id="unknown"))


def thread_post(post: PagePost) -> ThreadPost:
    return ThreadPost(
        id=post.id,
        is_published=post.is_published,
        message=post.message,
        permalink=post.permalink,
    )


def comment_time(comment: Comment) -> float | None:
    """Milliseconds for a Graph timestamp, or ``None`` when missing or unparseable.

    Never raises. A malformed timestamp costs one comment its place in the
    ordering, never the whole answer.
    """
    if not comment.created_time:
        return None
    try:
        return datetime.strptime(comment.created_time, "%Y-%m-%dT%H:%M:%S%z").timestamp()
    except ValueError:
        try:
            return datetime.fromisoformat(comment.created_time).timestamp()
        except ValueError:
            return None


def in_time_order(comments: list[Comment]) -> list[Comment]:
    """Comments in the order they were written.

    Stable, and deliberately so: a comment Graph timed incompletely keeps its
    incoming position rather than sorting to one end. Degrading to the order the
    caller supplied is defensible; inventing an order is not.
    """
    timed = [(comment_time(c), index, c) for index, c in enumerate(comments)]
    if any(t is None for t, _, _ in timed):
        # With any unparseable timestamp, sorting the whole list would move
        # comments that DO have times past ones that do not, which is inventing
        # an order. Sort only among the timed ones, in place.
        known = sorted(
            [(t, i, c) for t, i, c in timed if t is not None], key=lambda row: (row[0], row[1])
        )
        result = list(comments)
        for slot, (_, _, comment) in zip([i for t, i, _ in timed if t is not None], known):
            result[slot] = comment
        return result
    return [c for _, _, c in sorted(timed, key=lambda row: (row[0], row[1]))]


def last_word(thread: Thread) -> Comment:
    """The most recent thing said in this thread — the comment itself when nobody replied.

    **By the clock, not by array position, and that distinction is the whole of
    T-36.** Triage asks who spoke last. Answering with ``replies[-1]`` is the
    same question only while the array is in time order, and the array is built
    by concatenating every second-level reply with every third-level one: give a
    thread two branches and the Page's older answer on the first lands last,
    behind a visitor's newer comment on the second. The thread then reports
    ``answered`` with a customer waiting.
    """
    ordered = in_time_order(thread.replies)
    return ordered[-1] if ordered else thread.comment


def assemble_threads(
    post: ThreadPost, comments: list[Comment], replies_by_comment_id: dict[str, list[Comment]]
) -> list[Thread]:
    return [
        Thread(comment=c, replies=replies_by_comment_id.get(c.id, []), post=post) for c in comments
    ]
