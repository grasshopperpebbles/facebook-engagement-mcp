# facebook-engagement-mcp

An MCP server for triaging comments on a Facebook Page.

Given a Page, a post, or a comment, it assembles reply threads, works out which
still need an answer from the Page, groups them with counts, and returns that
instead of a raw comment array for a model to sort through. Replying and hiding
are available behind an explicit flag. Deleting is not offered at all.

## Pick your language

| | Status | How you run it |
|---|---|---|
| **[Node / TypeScript](./node/)** | **ready — v0.1.13** | A `.mcpb` bundle. Double-click it; Claude Desktop supplies the runtime. **[Start here](./node/README.md)** |
| **[Python](./python/)** | **ready** — all three tools | a Python package (`pip install`). **[Details](./python/README.md)** |
| PHP | not built yet | a PHP package |
| Go | not built yet | a Go module |

**If you are here to use this, you want [Node](./node/README.md).** Marketers
download a `.mcpb` from
[Releases](https://github.com/grasshopperpebbles/facebook-engagement-mcp/releases)
and double-click it — **no git**. See
[Install for marketers](./node/README.md#as-a-claude-desktop-extension-mcpb--marketers-start-here).

**Developers** who need source: one monorepo (`node/`, `python/`, shared
`docs/`). Clone or sparse-checkout under
[From source — developers only](./node/README.md#from-source--developers-only).
There is no separate per-language GitHub repository. Python can also install
from PyPI with no clone.

## Getting a token is the hard part, and it is language-neutral

Whichever implementation you use, the server needs a Meta access token for an
identity that administers the Page, and producing one is a job in itself. That
work is the same for all of them, so it lives at the top level rather than inside
any one language:

| | |
|---|---|
| **[Get your Meta access token](./docs/get-your-token.md)** | **Start here.** Five steps, about fifteen minutes, no code. |
| **[Setup, tokens and permissions](./docs/setup-tokens-permissions.md)** | The full reference: the Meta app, the three-token chain, the `MODERATE` role. |
| **[Dark posts: why `/feed` misses your ads](./docs/dark-posts.md)** | Why no Page-level sweep reaches a comment on an ad, and the route that does. Read this before deciding the tool doesn't work. |
| **[The `publish_actions` error](./docs/publish-actions-error.md)** | A write refused, naming a permission removed in 2018. It is not about permissions. |
| **[The System User token and the 60-day expiry](./docs/system-user-token.md)** | The 60-day exchange is avoidable. Read this before setting up for a team. |

Each page is written from work run against a live Meta app rather than read off
Meta's documentation — which matters here, because two of them exist to correct
it.

## How this repository is laid out

```
node/              the TypeScript implementation and the .mcpb bundle
docs/              Meta setup, tokens and errors — shared by every language
fixtures/          recorded Graph responses — shared by every language
conformance/       the suite every implementation must pass
```

**`docs/` and `fixtures/` are deliberately outside `node/`.** Neither is about
TypeScript: the documents describe Meta's console and Graph's behaviour, and the
fixtures are recorded Graph responses whose shapes were validated against a live
Page. Every implementation's users need the first and every implementation's
tests need the second.

**`conformance/` is how several implementations stay the same thing.** The
TypeScript client is vendored from a private monorepo by script, which works only
because both ends are TypeScript; nothing can vendor into Python. So what holds
the implementations equivalent is not copied code but a shared suite of cases,
each one encoding a rule where the obvious implementation is the wrong one. Every
rule in it is a bug this project actually shipped, and every case has been
watched failing. See [`conformance/README.md`](./conformance/README.md).

## Licence

Apache 2.0 — see [LICENSE](./LICENSE).
