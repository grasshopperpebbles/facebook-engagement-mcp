# Documentation

Five articles. Together they are the complete account of getting a Meta app, a
token, and this server working — including the findings that are not in Meta's
documentation because they contradict it.

**This index listed three of them until 2026-09-15**, while `get-your-token.md`
and `system-user-token.md` sat in the same directory, linked from the root
README and reachable by URL but absent from the page whose job is to list them.
Both are the ones a new reader needs first. Recorded rather than quietly fixed
because it is this repository's own failure mode in miniature: the thing that
went stale was the summary, not the material it summarised.

| Read this | When |
|---|---|
| [Get your Meta access token](./get-your-token.md) | **Start here if you just want it working.** The short path to the one thing the installer asks for: five steps, about fifteen minutes, no code. Each step links to the full explanation when it goes wrong. |
| [Setup, tokens and permissions](./setup-tokens-permissions.md) | **Start here.** Creating the Meta app, the three-token chain, the `MODERATE` role, and why a correctly-scoped call can return an empty array. |
| [Dark posts and why `/feed` misses your ads](./dark-posts.md) | You passed a `page` target and a post you can see on the Page is missing. Covers both causes: ads run on unpublished posts that no Page-level edge reaches (and the `ad`/`campaign` route that does), and **two tokens for the same Page are shown different numbers of posts** — a System User's saw three where a personally-granted one saw five, comments included. |
| [The `publish_actions` error](./publish-actions-error.md) | A write was refused naming a permission removed in 2018. It is not about permissions. |
| [The System User token and the 60-day expiry](./system-user-token.md) | You do not want to re-mint a token every 60 days. Verified end to end: a System User token came back `expires_at: 0`, and so did the Page token exchanged from it. Read this before setting up for a team. |

Each was written from work done against a live Meta app and a live Page, and
each carries a `verified:` line in its frontmatter saying exactly what was run
rather than read. Where something is still assumed, they say so.

## Why these are here

The README used to say "there is no URL to link" and carry a `TODO`, because
the articles lived only in a content repo. That left the one thing a new user
actually needs — how to produce a token — pointing at nothing.

They are markdown in the repo rather than posts on a site because the audience
for them is someone reading the README with a terminal open. Code blocks
render, anchors work, and nothing depends on a site being live.

**The story versions live on the GrasshopperPebbles Facebook Page.** Same
material, different job: the Page carries what went wrong and what it cost, and
this carries what to type. If you want to argue with any of it, the Page is the
place — the comments there are also what this server gets pointed at, which is
the most direct dogfooding available to it.
