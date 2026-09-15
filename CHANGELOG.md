# Changelog

Keep-a-Changelog format. Versions follow semver.

## [0.1.12] — 2026-09-15

### Added

- **Every answer now says which identity read it.** `comment_activity` returns
  `identity` — the token's `type`, the app it belongs to, and whether it never
  expires — on the orientation response and on every sweep.

  This exists because two Page access tokens for the same Page, issued by the
  same app, do not return the same thing: a Business System User's was shown
  three posts where a personally-granted user token was shown five, and returned
  no author for comments the other identified. Neither difference announces
  itself, so an answer that cannot name its own identity cannot explain why it
  is short.

  **Both token kinds were always supported** — the exchange is identical. What
  was missing was saying which one you gave it.

  It is reported on *every* answer rather than only degraded ones, because the
  degradation is not always detectable, and a field that appears only on bad
  days teaches you to read its absence as good news.

- **A warning when it plausibly cost you something**, gated the other way: added
  only when a System User token produced a response that actually lost
  something — an unreadable post, or a thread whose last word carried no author.
  A caveat on every response is boilerplate by the third one, and this one needs
  believing on the day it matters.

- `pages.identity()` in the vendored Meta client. Returns `valid: false` rather
  than throwing for a token Graph will not debug: an unusable token is an
  answer, not a crash.

## [0.1.11] — 2026-09-15

### Fixed

- **0.1.10 blamed the wrong thing, and 0.1.9 stated a limitation that is not
  true.** Both are corrected here. The behaviour of the code is unchanged in
  0.1.10's case; what it says about the behaviour is not.

  A personally-granted Page token for the **same app** as the Business System
  User one returned **five** posts where the System User's returned three, read
  the two missing posts and their comments without complaint, and returned
  `from: { name, id }` for a person's comments that the System User token
  returned with no author at all.

  So: a post is **not** withheld from apps other than the one that published it
  (0.1.10), and Facebook does **not** simply withhold author information for
  people (0.1.9). Both were conclusions drawn from comparisons in which the
  token was never held still.

  What is observed, and all that is claimed: **some Page access tokens are shown
  fewer posts than others on the same Page, and identify fewer of the
  commenters.** A token exchanged from a Business System User has been seen to
  do both. If posts or authors are missing, try a token granted by a person who
  administers the Page.

  This matters because the setup guide recommends the System User token, for the
  good reason that it does not expire. It is also the identity observed seeing
  less.

  The unreadable-posts check added in 0.1.10 is unchanged and was never wrong —
  it reports posts these credentials cannot read, found through the photos
  inside them. Only its explanation moved.

- **Known limitations** rewritten accordingly, and `docs/dark-posts.md` with it,
  including a retitle: the article is no longer about what another app
  published.

## [0.1.10] — 2026-09-15

### Added

- **The answer now says when this Page holds posts your credentials cannot
  read.** Facebook does not return a post to an app other than the one that
  published it — confirmed by reading one Page with two Page tokens belonging to
  different apps and checking both lists against the publishing tool's own
  records: five posts for five, the two published through another app visible
  only to that app's token. There is no error and no gap; the list is simply
  shorter. So if you schedule through Buffer, Hootsuite, Later, Business Suite
  or your own tooling, those posts and their comments were silently absent.

  A `page` sweep now reads the Page's photos, which stay reachable when the post
  holding them does not and which name that post, then asks whether each named
  post is actually readable. Refusals are reported in `notes` with `partial:
  true`. **The count is a minimum** — a text-only post leaves no photo behind
  and cannot be detected at all, so the absence of this note is not a promise
  that nothing is missing.

  Absence from the feed is deliberately not treated as evidence on its own: a
  Page's cover photo names a post id the feed does not list it under, and
  counting that produced the right total for the wrong reason before the check
  was made to probe readability instead.

- `photos.forPage` in the vendored Meta client. It requests no image data; it
  exists to name posts, not to render them.

### Changed

- **Known limitations** gained the above as a *cannot do at all*, and lost the
  claim that pagination has never run against real Graph — that stopped being
  true on 2026-09-14, when 101 comments were read back through `paging.next`
  across more than one request on the pinned API version.

## [0.1.9] — 2026-09-15

### Fixed

- **A thread is no longer reported as answered when nobody is identified.**
  Facebook returns author information for a comment written by a **Page** and
  withholds it for one written by a **person** — confirmed against live Graph
  across three Pages and three people, including a person with no role on the
  Page and no connection to the app. That makes a Page's own comments the only
  dependable source of an author id, so a batch carrying no author at all is a
  batch in which the Page has replied to nothing. The fallback used to read
  "somebody replied, so it is handled": a person asked, another person answered,
  and the thread came back `answered` while the Page had never spoken — and it
  did that precisely when the Page was behind on everything, which is what this
  tool is for. Every thread in such a batch is now listed as needing a reply,
  with a note saying why and warning that which person wrote which comment
  cannot be reported. A single Page-authored comment restores identity triage,
  and `answered` with it.

  The ordinary case is unaffected and was never wrong: `answered` only ever
  needs to identify the Page, and the Page always carries its own author id.

- The note reporting threads without author information now counts a thread's
  **last word** rather than requiring every comment in it to be anonymous, which
  is what triage actually reads. It was silent on the commonest real shape.

## [0.1.8] — 2026-09-14

### Fixed

- **A thread with two branches could report `answered` while a customer was
  waiting.** `comment_activity` assembles a thread from several Graph calls and
  concatenated them as *every second-level reply, then every third-level one* —
  regardless of when any of it was written. Triage then read the last element as
  the most recent thing said. Where the Page had answered one branch and a
  visitor later commented on another, the Page's older reply sorted last and the
  thread was marked handled. The replies are now sorted by `created_time`, so
  the conversation reads in the order it happened, and "who spoke last" is
  answered from the timestamps rather than from array position. A reply Graph
  timed incompletely keeps its incoming position rather than sorting to one end.

  This is the same defect as 0.1.6, in the same direction, inside the code
  0.1.6's fix was written into: a fix inherits the invariants of the code it
  lands in, and "these replies are in time order" was never written down.

> **This file skipped 0.1.3 through 0.1.7.** Those versions shipped — see the
> tagged commits — and were never written up here. The entries below resume at
> 0.1.2. Left as a visible gap rather than reconstructed from commit messages,
> which would be a guess at what the releases meant to their author.

## [0.1.2] — 2026-09-09

### Changed

- `support` now points at the setup guide rather than the issue tracker.
  **Watched in Claude Desktop:** the extension page renders exactly one clickable
  link — the ↗ beside the title — and it takes its URL from `support`.
  `documentation` is not surfaced anywhere, and a URL inside a field
  `description` renders as plain body text that has to be copied by hand. So the
  only link the product offered its user was a GitHub issue tracker, while the
  one thing they cannot proceed without — how to mint a token — was unclickable
  text. For a marketer, the setup guide *is* the support channel.

## [0.1.1] — 2026-09-09

Documentation links only. No behaviour changed; the server, its tools and its
permissions are identical to 0.1.0.

### Added

- The manifest carries `documentation`, `homepage`, `support` and `repository`,
  which the MCPB spec defines and this manifest had never set.
- The install form's access-token field names the setup guide by URL. It is the
  one required field, asking a non-technical installer for the one thing they
  cannot produce by reading the form, and it previously described what a token
  *is* and stopped there.
- Two manifest tests: the token description must contain the `documentation`
  URL, and every in-repo `blob/main/*.md` link in the manifest must resolve to a
  file in the working tree.

### Why this is a version bump and not an amendment to 0.1.0

Two different bundles were built claiming `0.1.0` — the one installed on
2026-09-08 and the one packed on 2026-09-09 with the links in it. That is the
exact thing `manifest.version` exists to prevent, and it is also what leaves
Claude Desktop offering only *Uninstall* rather than *Update*, since it sees a
version it already has. A bundle must be traceable to a build.

## [0.1.0] — 2026-09-03

**Superseded in part.** Two claims below were true of what shipped and are not
true of the product now, and are left in place rather than edited: a record of
what was believed at release is worth more than a tidy one.

- *"Reads `/{page-id}/feed`, so comments on unpublished ad-backed posts are
  included"* — **false, and it was false on the day.** `/feed` does not return
  unpublished posts; tested against a live Page on 2026-09-03. Ad comments are
  reached by resolving an ad to the post behind it, which is why the `ad` and
  `campaign` targets listed under *Not included* were restored the same day.
- *"or an explicit `confirmed: true` on clients that cannot prompt"* — removed
  on 2026-09-08. A tool argument is produced by the model, so that flag let the
  model hold its own permission slip, and it used one. A client that cannot
  prompt is now refused unless a person sets the override in the install form.



Initial release. Not published to npm.

### Added

- `comment_activity` — comment threads for a Page, post or comment, triaged by
  whether they need a reply, grouped with per-group counts. Reads
  `/{page-id}/feed`, so comments on unpublished ad-backed posts are included.
- `respond_to_comment` — publish a reply as the Page. Off by default; requires
  confirmation via MCP elicitation, or an explicit `confirmed: true` on clients
  that cannot prompt.
- `moderate_comment` — hide or unhide a comment. Off by default.
- A `TokenProvider` seam exchanging a user token for per-Page tokens.
- Structured logging to stderr on a field allow-list; comment bodies are never
  logged.
- The Meta Graph client, vendored from the (private) gpp-mcp monorepo by
  `scripts/extract.mjs`.

### Not included

- Comment deletion. Irreversible, and it destroys content authored by someone
  other than the operator.
- Ad and campaign targets, which need a Marketing API client.
- Instagram.
