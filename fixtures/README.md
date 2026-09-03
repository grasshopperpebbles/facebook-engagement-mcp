# Fixtures

**Shape-validated against a live Facebook Page on 2026-09-03.** Values are
synthetic — ids, names and message text are invented so the tests read clearly
and no real person's data is committed — but the *shapes* are no longer guesses.

## What was checked, and how

A development-mode Meta app was created, a user token minted with
`pages_show_list`, `pages_read_engagement`, `pages_read_user_content` and
`pages_manage_engagement`, exchanged for a Page token via `/me/accounts`, and
the same field selections this client requests were read back from
`/me/accounts`, `/{page-id}/feed`, `/{post-id}/comments` and
`/{comment-id}/comments`.

The hand-authored fixtures turned out to be substantially correct. Two gaps
were found and fixed:

- **`permalink_url` on comments.** Graph returns it; the comment fixtures
  omitted it. Added to `comments-p1.json`, `comments-p2.json` and
  `replies.json`.
- **Top-level `paging`.** Every real list response carries `paging.cursors`,
  even for a single page. No fixture had it. Added to all six. Note this does
  not change behaviour — the transport keys off `paging.next`, which is absent
  in both the old fixtures and a real single-page response — but a fixture that
  omits a key reality always sends is a fixture that can hide a bug.

Everything else matched field-for-field: the Page, feed and comment field sets
were exactly what Graph returned.

## What is now settled, and what is not

**Settled: `from` is returned for comments authored by someone other than the
Page.** This was the design spec's largest open question, because it decides
whether "needs a reply" means *the Page has not replied* or merely *nobody
replied*. A third-party comment came back with
`from: { name, id }` where the id differs from the Page id, so `statusBasis`
resolves to `author_identity` in practice and the degraded `reply_count` path
is a fallback rather than the normal case.

Also confirmed live: `can_hide` is returned, and is `true` on a third party's
comment and `false` on the Page's own — the server's `canHide` is meaningful,
not decorative.

**Not settled: unpublished, ad-backed posts.** The Page used for validation had
no dark posts, so `is_published: false` is still an unexercised path against
live Graph. `feed.json` keeps a synthetic unpublished post because that case is
the server's reason to exist and must stay covered — but its shape is inferred
from the published posts beside it, not observed.

**Not settled: pagination.** Every live response fitted in one page, so
`paging.next` and the truncation path have still only been exercised against
fixtures.
