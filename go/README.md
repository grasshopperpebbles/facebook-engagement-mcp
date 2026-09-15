# facebook-engagement-mcp — Go

An MCP server that finds and groups the comments on a Facebook Page's posts, and
on the **ads** running off them — which is the harder half, because ads usually
run on unpublished posts and no Page-level sweep returns those.

This is the Go implementation. It is behaviourally identical to the
[Node](../node/README.md) and [Python](../python/README.md) ones, and the
[conformance suite](../conformance/README.md) is what holds that true: one
runner, ten cases, six entries, 60/60.

**If you are here to use this rather than to read it, you want
[Node](../node/README.md).** It ships as a `.mcpb` bundle you double-click, and
Claude Desktop supplies the runtime. This package is for a Go developer who wants
the server as a module.

## Install

```bash
go install github.com/grasshopperpebbles/facebook-engagement-mcp/go/cmd/facebook-engagement-mcp@latest
```

Requires Go 1.25 or newer. The `/go` in the path is not a typo — the module lives
in a subdirectory of the repository, so its module path carries the directory.

## Configure

Two environment variables, one of them required:

| | |
|---|---|
| `META_ACCESS_TOKEN` | **Required.** A user or System User access token. See **[Get your Meta access token](../docs/get-your-token.md)** — five steps, about fifteen minutes, no code. |
| `FACEBOOK_ENGAGEMENT_ENABLE_WRITES` | Set to `true` to enable replying and hiding. Anything else and the two write tools are not registered at all, so a model cannot call what it would only be refused. |

In Claude Desktop's `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "facebook-engagement": {
      "command": "facebook-engagement-mcp",
      "env": {
        "META_ACCESS_TOKEN": "your-token",
        "FACEBOOK_ENGAGEMENT_ENABLE_WRITES": "false"
      }
    }
  }
}
```

The token needs `pages_read_engagement`, `pages_read_user_content` and — for the
write tools — `pages_manage_engagement`, plus the **MODERATE** role on the Page.
For ad and campaign targets it also needs `ads_read`. The full chain is in
**[Setup, tokens and permissions](../docs/setup-tokens-permissions.md)**.

## The three tools

- **`comment_activity`** — the read path. Omit every target to list the Pages and
  ad accounts this identity can reach; pass `adAccount` to list its campaigns by
  name; pass `page`, `post`, `comment`, `ad` or `campaign` to read comments. The
  first two rungs read no comments and are cheap.
- **`respond_to_comment`** — publish a reply as the Page. Requires `pageId`. Use
  `dryRun` first.
- **`moderate_comment`** — hide or unhide. Requires `pageId`. This server cannot
  delete comments.

## Limitations you should read before deciding it does not work

**1. Some ad formats create no Page post at all.** Their comments are unreachable
from the Page side by any route, because there is no post id to sweep, reply to
or moderate. The tool says so in `notes` rather than returning a confident zero.

**2. Some Page tokens are shown fewer posts than others, and the comments go with
them.** Two Page access tokens for the same Page, issued by the same app,
returned different post lists on 2026-09-15: one exchanged from a **Business
System User** token saw three posts; one exchanged from a **personally-granted**
user token saw five, and read the missing two and their comments without
complaint. The missing posts are absent from `/feed`, `/posts`,
`/published_posts` and `include_hidden=true`, and cannot be read by id either.

This matters because the System User token is the one that never expires, and the
setup guide recommends it for exactly that reason. It is also the one observed
seeing less. **If you run a scheduler or another posting tool alongside this
server, check the `notes` on a `page` sweep** — what you get otherwise is a
shorter list and no error.

The server detects it where it can: the photos inside an unreadable post stay
reachable and name the post they belong to, so a `page` sweep probes them and
reports refusals in `notes` with `partial: true`. **The count is a minimum** — a
text-only post leaves no photo behind — so the absence of that note is not a
promise that nothing is missing.

The rule Meta is applying is **not established**, and an earlier version of this
entry named the wrong one: it said a post is not returned to an app other than
the one that published it, on a five-for-five correlation. Two things differed
between those tokens and only one was tested. Comments on **ads** are unaffected
either way — an `ad` or `campaign` target resolves the post through the Marketing
API rather than through the Page's feed.

**3. Comment text is untrusted.** Comment bodies and author display names are
written by the public, and this server also holds tools that publish and hide.
Every piece of free text is returned inside an `untrusted` marker and truncated,
and no group label ever carries any of it. That stops the text being mistaken for
structure. **It does not stop a model being persuaded by content it legitimately
reads**, and nothing here claims otherwise.

## Development

```bash
go build ./...
go vet ./...
go test ./... -race
```

Against the conformance suite, from the repository root:

```bash
go build -o go/bin/facebook-engagement-mcp ./go/cmd/facebook-engagement-mcp
docker build -t fbe-go:dev go/
node conformance/run.mjs go go-native
```

Two entries, deliberately. The container catches packaging defects the native run
hides, and the native run is fast enough to develop against. Running one
implementation two ways has already been worth more than the reason it was done
for — see the conformance README.

## Docker

```bash
docker build -t fbe-go:dev .
```

Two stages: the first compiles a static binary, the second runs it on
`distroless/static` as a non-root user. **Not how anyone installs this** — that
is `go install` — but the image builds the binary and runs the binary, never
`go run` over the source. This project shipped an entrypoint defect that did not
exist in the source at all; it existed in the packaging, and twelve reviews of
the source approved the line.

## The rules that are not obvious

Each of these is a defect this project shipped, found against live Graph, usually
after passing review. They are why a port written from a description reintroduces
bugs, and why the conformance suite exists:

| | |
|---|---|
| `/feed` does not return unpublished posts | Meta's documentation says it does. Ads run on unpublished posts, so no Page sweep reaches ad comments — pass an `ad` or `campaign` instead. |
| `pageId` is **required** on both write tools | Optional, it fell back to the user token, and Meta refused by naming a permission removed in 2018. The fault was the identity, not the permission. |
| An ad with no Page post behind it **says so** | Some formats create none. An empty answer would read as "no comments" rather than "not visible here". |
| A comment read with a **user** token returns an **empty array**, not an error | Hence the Page-token exchange in `internal/graph/tokens.go`. The most confusing failure in this API. |
| Threads run **three** levels deep | And `parent` points at the *reply*, not the top-level comment. |
| `answered` means the Page spoke **last** | Not that the Page appears somewhere in the thread. |
| Replies are **not** in time order | A thread is assembled from several calls; concatenating them is not chronological. |
| No author anywhere ⇒ needs a reply | And the reason must not be guessed at — see `internal/outcomes/triage.go`. |
| `paging.next` may come back on a **different API version** | The version segment is rewritten back to the pin; the cursor is preserved. |

### Two more that are specific to Go

| | |
|---|---|
| **Every optional Graph field is a pointer** | Go has no `undefined`, so a value-typed author is indistinguishable from an absent one — and `statusBasis` turns entirely on whether an author was present anywhere. The same trap recurs wherever zero is meaningful: `comment_count`, `is_hidden`, `can_hide`. |
| **Ranging a map is randomised** | Group order and the de-duplicated post list both keep an insertion order alongside the map. Without it a response reshuffles itself run to run and cannot be diffed — a defect neither TypeScript nor Python can have. |
