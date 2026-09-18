"""T-55 — Meta's (#100) on a comments edge means no comments, not a failed read.

The cross-language behaviour (the sweep stays non-partial and still reports the
sibling post's comments) is held by the conformance case
``an-absent-comments-edge-is-no-comments``. These are the unit-level cases for
the predicate itself, in the language it would be re-broken in.
"""

from __future__ import annotations

from facebook_engagement_mcp.graph.errors import MetaApiError, is_absent_comments_edge

UNSUPPORTED = (
    "Unsupported get request. Object with ID 'pg1_p2' does not exist, cannot be "
    "loaded due to missing permissions, or does not support this operation"
)


def test_code_100_is_an_absent_edge():
    assert is_absent_comments_edge(MetaApiError(UNSUPPORTED, status=400, code=100, subcode=33))


def test_the_wording_alone_is_enough_when_no_code_arrives():
    # Graph has been observed returning this shape without a code on some edges,
    # and a predicate that only reads the code would call it a read failure.
    assert is_absent_comments_edge(MetaApiError(UNSUPPORTED, status=400))


def test_a_permission_error_is_not_an_absent_edge():
    # The whole point: this one MUST stay a labelled gap. Treating it as "no
    # comments" would tell a marketer their post is quiet when their token is
    # simply not allowed to read it.
    assert not is_absent_comments_edge(
        MetaApiError("(#200) Permissions error", status=403, code=200)
    )


def test_an_ordinary_exception_is_not_an_absent_edge():
    assert not is_absent_comments_edge(RuntimeError(UNSUPPORTED))
