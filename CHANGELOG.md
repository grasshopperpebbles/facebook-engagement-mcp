# Changelog

Keep-a-Changelog format. Versions follow semver.

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
