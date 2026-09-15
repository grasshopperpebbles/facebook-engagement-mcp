"""``comment_activity`` — find and group the comments that need a reply."""

from __future__ import annotations

from datetime import UTC, datetime, timedelta
from typing import Any

from ..graph import MetaApiError, PagesClient
from ..graph.types import Comment
from .grouping import count_threads, group_threads
from .render import (
    TextBudget,
    abbreviate_comment,
    comment_text_cost,
    render_comment,
    untrusted_text,
)
from .threads import Thread, ThreadPost, assemble_threads, in_time_order, last_word, thread_post
from .triage import apply_filter, triage_threads

MAX_POSTS = 100
MAX_COMMENTS_PER_POST = 400
MAX_REPLIES_PER_COMMENT = 200


def _page_id_from_post_id(post_id: str) -> str | None:
    return post_id.split("_")[0] if "_" in post_id else None


async def run_comment_activity(
    client: PagesClient,
    *,
    page: str | None = None,
    post: str | None = None,
    comment: str | None = None,
    filter: str = "needs_reply",
    group_by: str = "post",
    since: str | None = None,
    max_threads: int = 100,
) -> dict[str, Any]:
    notes: list[str] = []
    partial = False
    # UTC rather than local: `since` is sent to Graph, whose timestamps are UTC,
    # and a local-midnight default silently shifts the window by a day for
    # anyone east or west of it.
    since = since or (datetime.now(UTC).date() - timedelta(days=30)).isoformat()

    if page is not None:
        target = {"kind": "page", "id": page}
        page_id: str | None = page
    elif post is not None:
        target = {"kind": "post", "id": post}
        page_id = _page_id_from_post_id(post)
    elif comment is not None:
        target = {"kind": "comment", "id": comment}
        page_id = None
    else:
        raise ValueError(
            "Pass a page, post or comment id. Ad and campaign targets are not implemented "
            "in the Python package yet — see python/README.md."
        )

    threads: list[Thread] = []
    posts: list[ThreadPost] = []

    if comment is not None:
        replies = await client.replies(comment, max_items=MAX_REPLIES_PER_COMMENT)
        root = Comment(id=comment)
        threads = [Thread(comment=root, replies=replies, post=ThreadPost(id="unknown"))]
    else:
        if page is not None:
            posts = [thread_post(p) for p in await client.feed(page, since=since, max_items=MAX_POSTS)]
        else:
            posts = [ThreadPost(id=post)]  # type: ignore[arg-type]

        for p in posts:
            try:
                comments = await client.comments_for_post(p.id, max_items=MAX_COMMENTS_PER_POST)
                replies_by_comment_id: dict[str, list[Comment]] = {}

                for c in comments:
                    if not (c.reply_count or 0):
                        continue
                    items = await client.replies(c.id, max_items=MAX_REPLIES_PER_COMMENT)

                    # A reply can carry replies of its own. Confirmed against
                    # live Graph on 2026-09-11 (T-11): a third level is accepted
                    # and `parent` points at the REPLY, not at the top-level
                    # comment. Stopping after one level meant a visitor's answer
                    # to the Page's reply was never read — and since triage asks
                    # who spoke last, an unread last word made a waiting customer
                    # look handled.
                    #
                    # Flattened into one thread deliberately: the unit a person
                    # acts on is the conversation, not the nesting. `reply_count`
                    # keeps this from costing a call per reply.
                    #
                    # Sorted, because flattening destroys the order (T-36). Each
                    # call returns its own replies chronologically, but
                    # concatenating puts every second-level reply ahead of every
                    # third-level one regardless of when it was written. Triage
                    # no longer reads the end of this array, and it is sorted
                    # anyway: the array is what a person reads, and a
                    # conversation rendered out of order is wrong on its own
                    # terms.
                    deeper: list[Comment] = []
                    for reply in items:
                        if not (reply.reply_count or 0):
                            continue
                        deeper.extend(await client.replies(reply.id, max_items=MAX_REPLIES_PER_COMMENT))
                    replies_by_comment_id[c.id] = in_time_order([*items, *deeper])

                threads.extend(assemble_threads(p, comments, replies_by_comment_id))
            except MetaApiError as error:
                # One failing post must not fail the whole answer — it becomes a
                # labelled gap instead.
                partial = True
                notes.append(f"Comments for post {p.id} could not be read: {error}")

    triaged, basis = triage_threads(threads, page_id)
    filtered = apply_filter(triaged, filter)

    # Sort before capping. Comments arrive oldest-first and post by post, so an
    # unsorted slice keeps the oldest threads from the first few posts while the
    # note below claims the newest — dropping exactly the comments a triage tool
    # exists to surface.
    capped = sorted(filtered, key=lambda t: t.thread.comment.created_time or "", reverse=True)[
        :max_threads
    ]
    if len(capped) < len(filtered):
        partial = True
        notes.append(f"{len(filtered)} threads matched; the {len(capped)} most recent are shown.")

    if basis == "reply_count" and page_id is not None:
        # T-39, with the wording T-45 corrected. The cause of a missing author is
        # NOT established, so this must not name one — an earlier version said
        # Facebook withholds `from` for comments written by a person, which was
        # overturned by T-42 the same day it shipped.
        notes.append(
            "No comment in this batch carried an author, so the Page cannot be identified in "
            "any thread and none can be shown as answered. Every thread is listed as needing a "
            "reply, which may over-report: some may already be handled. Author information is "
            "missing for reasons that depend on the credential rather than on who wrote the "
            "comment."
        )
    elif basis == "reply_count":
        notes.append(
            "Comment authors were not returned by Graph, so 'needs reply' means nobody replied "
            "at all, not that the Page has not replied."
        )

    # Counted on the LAST WORD, because that is what triage actually read.
    without_authors = sum(
        1 for t in capped if not (last_word(t.thread).author and last_word(t.thread).author.id)
    )
    if basis == "author_identity" and without_authors:
        notes.append(
            f"{without_authors} of these threads end with a comment carrying no author, so they "
            "are listed as needing a reply because the last word could not be shown to be the "
            "Page's. Some comments come back without author information and some do not, and "
            "which happens depends on the credential rather than on who wrote them; a thread "
            "here may be waiting or may already be handled."
        )

    budget = TextBudget()
    rendered: dict[int, dict[str, Any]] = {}
    abbreviated = 0
    for t in capped:
        cost = comment_text_cost(t.thread.comment) + sum(
            comment_text_cost(r) for r in t.thread.replies
        )
        if budget.take(cost):
            rendered[id(t)] = {
                "comment": render_comment(t.thread.comment),
                "replies": [render_comment(r) for r in t.thread.replies],
                "postId": t.thread.post.id,
                "status": t.status,
            }
        else:
            abbreviated += 1
            rendered[id(t)] = {
                "comment": abbreviate_comment(t.thread.comment),
                "replies": [abbreviate_comment(r) for r in t.thread.replies],
                "postId": t.thread.post.id,
                "status": t.status,
                "abbreviated": True,
            }
    if abbreviated:
        partial = True
        notes.append(
            f"{abbreviated} threads are returned without their text because the response "
            "reached its text budget. Their structure, counts and ids are complete."
        )

    referenced = {t.thread.post.id for t in capped}
    rendered_posts = []
    for p in posts:
        if p.id not in referenced:
            continue
        entry: dict[str, Any] = {"id": p.id}
        if p.is_published is not None:
            entry["isPublished"] = p.is_published
        if p.permalink:
            entry["permalink"] = p.permalink
        if p.message:
            entry["text"] = untrusted_text(p.message)
        rendered_posts.append(entry)

    return {
        "target": target,
        "filter": filter,
        "groupBy": group_by,
        "since": since,
        "statusBasis": basis,
        "totals": count_threads(capped),
        "posts": rendered_posts,
        "groups": [
            {
                "key": g["key"],
                "label": g["label"],
                "counts": g["counts"],
                "threads": [rendered[id(t)] for t in g["threads"]],
            }
            for g in group_threads(capped, group_by)
        ],
        "partial": partial,
        "notes": notes,
    }
