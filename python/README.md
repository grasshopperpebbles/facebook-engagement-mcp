# facebook-engagement-mcp — Python

An MCP server for triaging comments on a Facebook Page. Same outcome, same tool
shape and the same hard-won rules as the [Node build](../node/README.md), in
Python.

**Start with the token.** Whichever language you use, this needs a Meta access
token for an identity that administers the Page, and producing one is a job in
itself: **[Get your Meta access token](../docs/get-your-token.md)**.

## Install and run

```bash
pip install facebook-engagement-mcp        # or: uv pip install facebook-engagement-mcp
META_ACCESS_TOKEN=EAA… facebook-engagement-mcp
```

It speaks MCP over stdio. To use it from a client, point that client's config at
the `facebook-engagement-mcp` command with `META_ACCESS_TOKEN` in its
environment.

**There is no double-clickable installer for this build, deliberately.** The
`.mcpb` bundle is Node-only because Claude Desktop ships a Node runtime — that is
the entire reason it can install without a prerequisite. Python installs the way
Python installs.

## What this build does NOT do

Stated plainly and up front, because the failure this whole capability exists to
prevent is a confident short answer:

- **No ad, campaign or `adAccount` targets.** Those resolve an ad through the
  Marketing API to the Page post behind it, and that surface is not ported. **So
  comments on ads are unreachable here.** Ads run on unpublished posts and no
  Page-level sweep returns those, so there is no workaround within this build —
  use the Node one. The tool description says so too, where a model will see it.
- **No orientation rung.** Calling with no target is an error here rather than a
  listing of Pages and ad accounts.
- **No write tools.** `respond_to_comment` and `moderate_comment` are Node-only;
  this server is read-only and says so on startup.

What *is* here is the Page/post/comment read path, and it is held to the same
behaviour as Node by the [conformance suite](../conformance/README.md).

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
| `/feed` does not return unpublished posts | Meta's documentation says it does. Ads run on unpublished posts, so no Page sweep reaches ad comments. |
| A comment read with a **user** token returns an **empty array**, not an error | Hence the Page-token exchange in `tokens.py`. The most confusing failure in this API. |
| Threads run **three** levels deep | And `parent` points at the *reply*, not the top-level comment. |
| `answered` means the Page spoke **last** | Not that the Page appears somewhere in the thread. |
| Replies are **not** in time order | A thread is assembled from several calls; concatenating them is not chronological. |
| No author anywhere ⇒ needs a reply | And the reason must not be guessed at — see `triage.py`. |
| `paging.next` may come back on a **different API version** | The version segment is rewritten back to the pin; the cursor is preserved. |
