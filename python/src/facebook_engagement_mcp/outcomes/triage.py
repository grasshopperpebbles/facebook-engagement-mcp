"""Which threads still need an answer from the Page."""

from __future__ import annotations

from dataclasses import dataclass

from .threads import Thread, last_word

FILTERS = ("needs_reply", "unanswered", "hidden", "all")
GROUP_BYS = ("post", "author", "status", "day", "none")


@dataclass
class TriagedThread:
    thread: Thread
    status: str


def status_basis_for(threads: list[Thread]) -> str:
    """Which question the status actually answers.

    ``author_identity`` — no reply was authored by the Page. The real question.
    ``reply_count`` — nobody replied at all. A weaker, different question, used
    when Graph did not return ``from``. A degraded answer that looks identical to
    a confident one is the defect this label exists to prevent.
    """
    has_author = any(
        (t.comment.author and t.comment.author.id)
        or any(r.author and r.author.id for r in t.replies)
        for t in threads
    )
    return "author_identity" if has_author else "reply_count"


def triage_threads(threads: list[Thread], page_id: str | None) -> tuple[list[TriagedThread], str]:
    basis = "reply_count" if page_id is None else status_basis_for(threads)
    triaged: list[TriagedThread] = []

    for thread in threads:
        if thread.comment.hidden is True:
            triaged.append(TriagedThread(thread, "hidden"))
            continue

        if basis == "author_identity" and page_id is not None:
            # The Page must have the LAST word, not merely a word. `.some(isPage)`
            # reads visitor -> Page -> visitor as answered and hides a customer
            # who is waiting (T-11). "Last" is by the clock, not by array
            # position (T-36).
            spoke_last = last_word(thread).author
            status = "answered" if spoke_last and spoke_last.id == page_id else "needs_reply"
            triaged.append(TriagedThread(thread, status))
            continue

        if page_id is not None:
            # `reply_count` WITH a known Page: nobody is answered (T-39).
            #
            # This used to read "somebody replied, so it is handled", which
            # inverts the product's answer — nothing here can tell whether the
            # somebody was the Page.
            #
            # The reason first recorded for this fix was overturned within the
            # day and is worth carrying as a warning rather than repeating: it
            # said Graph returns `from` for a Page and withholds it for a person,
            # therefore a batch with no author means the Page has replied to
            # nothing. Every observation in that sample shared one uncontrolled
            # variable, the token (T-42/T-45). So the cause is NOT established
            # and nothing here may claim one.
            #
            # What the branch rests on now does not need the cause: with no
            # author anywhere, no thread can be shown to have ended with the
            # Page, so none can be called answered. That is the over-surfacing
            # direction. Say plainly that it is the CAUTIOUS answer, not the
            # accurate one — some of these may be handled.
            triaged.append(TriagedThread(thread, "needs_reply"))
            continue

        # No Page id at all: "somebody replied" is the only question the data
        # supports.
        triaged.append(TriagedThread(thread, "answered" if thread.replies else "needs_reply"))

    return triaged, basis


def apply_filter(threads: list[TriagedThread], filter_name: str) -> list[TriagedThread]:
    if filter_name in ("needs_reply", "unanswered"):
        return [t for t in threads if t.status == "needs_reply"]
    if filter_name == "hidden":
        return [t for t in threads if t.status == "hidden"]
    return list(threads)
