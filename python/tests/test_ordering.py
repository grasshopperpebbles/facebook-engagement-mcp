"""Thread ordering — T-36, in the language it would be re-broken in."""

from __future__ import annotations

from facebook_engagement_mcp.graph.types import Author, Comment
from facebook_engagement_mcp.outcomes.threads import Thread, in_time_order, last_word


def c(id: str, when: str | None, author: str | None = None) -> Comment:
    return Comment(id=id, created_time=when, author=Author(id=author) if author else None)


def test_orders_by_the_clock_not_by_position():
    # The T-36 shape: second-level replies concatenated ahead of third-level
    # ones, so the Page's older answer lands last by position.
    flattened = [
        c("branch_a", "2026-09-02T11:00:00+0000"),
        c("branch_b", "2026-09-02T13:00:00+0000"),
        c("a_reply", "2026-09-02T12:00:00+0000", "pg1"),
    ]
    assert [x.id for x in in_time_order(flattened)] == ["branch_a", "a_reply", "branch_b"]


def test_last_word_is_the_newest_not_the_final_element():
    thread = Thread(
        comment=c("root", "2026-09-02T10:00:00+0000"),
        replies=[
            c("branch_a", "2026-09-02T11:00:00+0000"),
            c("branch_b", "2026-09-02T13:00:00+0000"),
            c("a_reply", "2026-09-02T12:00:00+0000", "pg1"),
        ],
    )
    assert last_word(thread).id == "branch_b"


def test_an_untimed_comment_keeps_its_incoming_position():
    """Degrading to the order the caller supplied is defensible; inventing an
    order is not."""
    items = [c("first", "2026-09-02T12:00:00+0000"), c("untimed", None),
             c("third", "2026-09-02T11:00:00+0000")]
    assert [x.id for x in in_time_order(items)] == ["third", "untimed", "first"]


def test_last_word_of_a_thread_with_no_replies_is_the_comment():
    assert last_word(Thread(comment=c("only", "2026-09-02T10:00:00+0000"))).id == "only"
