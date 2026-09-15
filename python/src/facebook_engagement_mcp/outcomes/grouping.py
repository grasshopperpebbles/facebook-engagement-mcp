from __future__ import annotations

from typing import Any

from .triage import TriagedThread


def count_threads(threads: list[TriagedThread]) -> dict[str, int]:
    return {
        "threads": len(threads),
        # Top-level comments plus their replies.
        "comments": sum(1 + len(t.thread.replies) for t in threads),
        "needsReply": sum(1 for t in threads if t.status == "needs_reply"),
        "hidden": sum(1 for t in threads if t.status == "hidden"),
    }


def _key_for(thread: TriagedThread, group_by: str) -> tuple[str, str]:
    """Group key and human label.

    Labels never carry untrusted text — not comment bodies and not author
    display names, which are free text chosen by whoever wrote the comment and
    would otherwise put attacker-authored strings into a structural field.
    """
    if group_by == "post":
        # `is_published` is None when the post node was never read; saying
        # nothing beats asserting either state.
        suffix = " (unpublished — ad-backed)" if thread.thread.post.is_published is False else ""
        return thread.thread.post.id, f"Post {thread.thread.post.id}{suffix}"
    if group_by == "author":
        author = thread.thread.comment.author
        if not author or author.id is None:
            return "unknown", "Author not returned by Graph"
        return author.id, f"Author {author.id}"
    if group_by == "status":
        return thread.status, thread.status.replace("_", " ")
    if group_by == "day":
        created = thread.thread.comment.created_time
        if not created:
            return "unknown", "Date not returned by Graph"
        return created[:10], created[:10]
    return "all", "All threads"


def group_threads(threads: list[TriagedThread], group_by: str) -> list[dict[str, Any]]:
    buckets: dict[str, dict[str, Any]] = {}
    for thread in threads:
        key, label = _key_for(thread, group_by)
        buckets.setdefault(key, {"key": key, "label": label, "threads": []})["threads"].append(thread)

    groups = [
        {"key": b["key"], "label": b["label"], "counts": count_threads(b["threads"]),
         "threads": b["threads"]}
        for b in buckets.values()
    ]
    # Most waiting customers first — this is opened to find work.
    groups.sort(key=lambda g: (g["counts"]["needsReply"], g["counts"]["threads"]), reverse=True)
    return groups
