# facebook-engagement-mcp — Python

An MCP server for triaging comments on a Facebook Page. Same outcome, same tool
shape and the same hard-won rules as the [Node build](../node/README.md), in
Python.

**Start with the token.** Whichever language you use, this needs a Meta access
token for an identity that administers the Page, and producing one is a job in
itself: **[Get your Meta access token](../docs/get-your-token.md)**.

## Install and run

**From PyPI** (no git clone — preferred if you only need to run it):

```bash
pip install facebook-engagement-mcp        # or: uv pip install facebook-engagement-mcp
META_ACCESS_TOKEN=EAA… facebook-engagement-mcp
```

**From this monorepo** (developers only): clone or sparse-checkout `python` +
`docs` — see
[From source — developers only](../node/README.md#from-source--developers-only).
Then install from `python/` (`pip install -e .` / `uv sync`).

It speaks MCP over stdio. To use it from a client, point that client's config at
the `facebook-engagement-mcp` command with `META_ACCESS_TOKEN` in its
environment.

**There is no double-clickable installer for this build, deliberately.** The
`.mcpb` bundle is Node-only because Claude Desktop ships a Node runtime — that is
the entire reason marketers can install without git or a terminal. Python
installs the way Python installs.

## What this build does

The same three tools as Node, held to the same behaviour by the
[conformance suite](../conformance/README.md):

- **`comment_activity`** — the page, post and comment sweeps, plus the ad route
  (`ad` and `campaign`) and both discovery rungs (no target lists Pages and ad
  accounts; `adAccount` lists that account's campaigns by name).
- **`respond_to_comment`** and **`moderate_comment`** — off by default. Set
  `FACEBOOK_ENGAGEMENT_ENABLE_WRITES=true` to register them.

**This section used to say the opposite, and the correction is the point.** Until
2026-09-15 this build had the read path only, and said so here, in the tool
description and on startup — ads unreachable, no orientation, no writes. That was
accurate then. **Nobody re-reads a limitation to check it is still a limitation**,
so a closed gap rots in place and a reader decides against the build on a
weakness that no longer exists. Deleting the old text entirely would lose that;
this paragraph is what replaces it.

### What is still Node-only

- **The `.mcpb` bundle.** Claude Desktop ships a Node runtime, which is why a
  double-clickable installer is possible there and not here.
- **Elicitation on `respond_to_comment`.** Node asks the client to confirm before
  publishing. This build gates writes on the environment variable alone, which is
  the same property that actually matters — **a model cannot set it** — but it is
  one fewer prompt in front of a public reply. Turn writes on deliberately.

## Running against the conformance suite

```bash
cd .. && node conformance/run.mjs python
```

The suite launches this package as a subprocess, speaks MCP to it, and serves
recorded Graph responses from a stub. It drives the **installed** package through
its console script, never the source tree — the `bin` entrypoint bug this project
shipped did not exist in the source, it existed in the packaging, and twelve
reviews of the source approved it.

## Development

```bash
uv sync          # install, including dev dependencies
uv run pytest    # unit tests
uv run ruff check .
```

## Docker

The image exists for CI, the conformance runner and self-hosting. **It is not how
anyone installs this** — that is the `pip install` above.

```bash
docker build -t fbe-python:dev python/     # from the repository root
```

The image installs the built wheel and launches its console script, rather than
copying the source tree and running it in place. That is deliberate: a container
that runs from source tests code the user never receives.

## The rules that are not obvious

Each of these is a defect this project shipped, found against live Graph, usually
after passing review. They are why a port written from a description reintroduces
bugs, and why the conformance suite exists:

| | |
|---|---|
| `/feed` does not return unpublished posts | Meta's documentation says it does. Ads run on unpublished posts, so no Page sweep reaches ad comments — pass an `ad` or `campaign` instead. |
| `pageId` is **required** on both write tools | Optional, it fell back to the user token, and Meta refused by naming a permission removed in 2018. The fault was the identity, not the permission. |
| An ad with no Page post behind it **says so** | Some formats create none. An empty answer would read as "no comments" rather than "not visible here". |
| A comment read with a **user** token returns an **empty array**, not an error | Hence the Page-token exchange in `tokens.py`. The most confusing failure in this API. |
| Threads run **three** levels deep | And `parent` points at the *reply*, not the top-level comment. |
| `answered` means the Page spoke **last** | Not that the Page appears somewhere in the thread. |
| Replies are **not** in time order | A thread is assembled from several calls; concatenating them is not chronological. |
| No author anywhere ⇒ needs a reply | And the reason must not be guessed at — see `triage.py`. |
| `paging.next` may come back on a **different API version** | The version segment is rewritten back to the pin; the cursor is preserved. |
