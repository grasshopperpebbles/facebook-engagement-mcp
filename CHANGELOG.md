# Changelog

Keep-a-Changelog format. Versions follow semver.

## [0.1.0] — 2026-09-03

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
