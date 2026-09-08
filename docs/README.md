# Documentation

Three articles. Together they are the complete account of getting a Meta app,
a token, and this server working — including the two findings that are not in
Meta's documentation because they contradict it.

| Read this | When |
|---|---|
| [Setup, tokens and permissions](./setup-tokens-permissions.md) | **Start here.** Creating the Meta app, the three-token chain, the `MODERATE` role, and why a correctly-scoped call can return an empty array. |
| [Dark posts and why `/feed` misses your ads](./dark-posts.md) | You passed a `page` target and got no ad comments. Explains why no Page-level edge reaches them, and the `ad`/`campaign` route that does. |
| [The `publish_actions` error](./publish-actions-error.md) | A write was refused naming a permission removed in 2018. It is not about permissions. |

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
