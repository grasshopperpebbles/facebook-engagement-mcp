---
title: "Facebook's /feed Does Not Return Unpublished Posts — So Your Ads' Comments Are Invisible"
date: 2026-09-08
verified: "Reproduced live 2026-09-03 and again 2026-09-08 on a different Page. The ad-resolution chain was run end to end through a real paused ad on 2026-09-08."
---

<!-- Source of truth for this article is the broadkast content repo,
     under grasshopperpebbles articles/facebook-engagement-mcp-setup/article-dark-posts.md.
     This copy exists so the README can link to something that renders
     code correctly and does not depend on a site being live. -->


# Facebook's `/feed` Does Not Return Unpublished Posts — So Your Ads' Comments Are Invisible

If you run ads on Facebook, the comments you most want to read are the ones you
cannot get to. They are not on your Page. They are on posts that were never
published, which is what most ads run on — and the endpoint Meta's documentation
points you at does not return them.

I did not discover this by reading. I built a comment-triage tool on the
documented behaviour, shipped it through nineteen task reviews and a whole-branch
review, and only found out when I ran it against a real Page. Every one of those
reviews was reasoning about the same sentence in the same document.

**This article is the reproduction and the workaround.** For the app, token and
permission setup that comes before any of it — including the `ads_read` scope the
workaround needs — see the companion piece,
*Reading Facebook Page Comments with the Graph API: App Setup, Tokens, and
Permissions*. And once you can reach an ad's comments, [replying to one has its
own trap](./publish-actions-error.md): a
refusal naming `publish_actions`, a permission removed in 2018, which means you
sent a user token where a Page token was required.

## The short version

```text
Meta's docs:  /{page-id}/feed returns published AND unpublished posts
Reality:      it returns neither more nor less than /{page-id}/posts
Consequence:  no Page-level sweep reaches a single comment on your ads
The only way: ad id -> creative -> effective_object_story_id -> post -> /comments
```

That last line inverts the obvious design. Sweeping the Page is what everyone
builds; it silently misses every ad. Resolving the ad first is the awkward one,
and it is the only one that works.

## Why this is the case that matters

An advertiser can run six variants of a message without putting six posts on the
Page. That is the *point* of a dark post — "dark" as in unlit, not sinister. The
audience meets the post as an ad in their feed, comments there, and the
conversation accumulates somewhere your Page timeline never shows you.

So the comments under your ads are the ones with buying intent, the ones asking
about price and availability, and the ones complaining where your customers can
see it. They are also the ones every Page-based tool I checked cannot see at all.


If you run ads, the comments you care about most are on posts that never appear
in your Page's timeline. Ads frequently run on **unpublished** Page posts —
"dark posts", industry slang for unlit rather than sinister. An advertiser can
run six variants of a message without putting six posts on the Page.

Meta's documentation says `/{page-id}/feed` returns published *and* unpublished
posts, where `/{page-id}/posts` and `/{page-id}/published_posts` return only
published ones. That sentence is the reason most people reach for `/feed`.

**I tested it. It is not true.**

### Make a dark post and see for yourself

You do not need to spend anything, and you do not need to run an ad. What makes
a post dark is being unpublished, not being promoted. One call creates one.

Do it by hand the first time. `dark-post-test.sh` in
[Reader downloads](#reader-downloads) runs this whole sequence — create, hunt,
confirm, delete — in about a minute, and it is the right tool once you know what
it is doing. But the finding here is a negative result, and a script that hands
you one is worth less than watching the queries come back empty yourself.

Note the script **deletes the post at the end**. If you are creating one to
attach an ad to, do it manually and keep the id.

```bash
curl -s -X POST -H "Authorization: Bearer $PAGE_TOKEN" \
  -d "message=Verification post. Unpublished; safe to delete." \
  -d "published=false" \
  "https://graph.facebook.com/v25.0/$PAGE_ID/feed"
```

That needs `pages_manage_posts` on top of the four permissions above — a
permission a read-only comments integration should not otherwise carry, so add
it, test, and consider removing it afterwards.

Confirm the post exists and is genuinely unpublished:

```bash
curl -s -H "Authorization: Bearer $PAGE_TOKEN" \
  "https://graph.facebook.com/v25.0/$POST_ID?fields=id,is_published,permalink_url"
```

You will get `"is_published": false`. Now go looking for it:

| Query | Returns the dark post? |
|---|---|
| `/{page-id}/feed` | **No** |
| `/{page-id}/feed?is_published=false` | No |
| `/{page-id}/feed?include_hidden=true` | No |
| `/{page-id}/posts` | No |
| `/{page-id}/posts?is_published=false` | No |
| `/{page-id}/published_posts` | No |
| `/{page-id}/promotable_posts` | Field does not exist |

`/feed` and `/posts` returned byte-identical id lists on both Pages I tried.
Whatever `/feed` gives you over `/posts`, it is not dark posts.

Delete it when you are done — and note *which id* you delete, because this
caught me out on 2026-09-08.

For a **text** post, the `{page-id}_{post-id}` composite works:

```bash
curl -s -X DELETE "https://graph.facebook.com/v25.0/$POST_ID?access_token=$PAGE_TOKEN"
```

For a **photo** post — which is what you need if the ad has to pass Instagram
validation — the composite is refused:

```json
{"error":{"message":"(#10) Application does not have permission for this action",
          "type":"OAuthException","code":10}}
```

That message is a lie of omission. Nothing is wrong with your permissions. Delete
it by the **photo id** instead — the id `/{page-id}/photos` returned when you
created it — and it succeeds immediately:

```bash
curl -s -X DELETE "https://graph.facebook.com/v25.0/$PHOTO_ID?access_token=$PAGE_TOKEN"
# {"success":true}
```

I first hit this while an ad still referenced the post and assumed that was the
cause. It was not: after deleting the campaign the composite still refused, and
the photo id still worked. Worth stating plainly, because a permissions-shaped
error sends you to the App Dashboard, which is the one place the answer is not.

### Building a test ad on a dark post

To prove the resolution chain you need an ad that exists. Here is the whole
sequence, including the four defaults that reject a dark post and the two
things that cost real money if you skip them. Walked end to end 2026-09-08.

**Before you start, know which credential each step uses.** Almost every error
below is really "wrong token" or "wrong method" wearing a disguise:

| Step | Explorer method | Explorer token |
|---|---|---|
| Create the unpublished post | **POST** | **Page** |
| Read the post back, hunt for it in `/feed` | GET | Page |
| List ad accounts, campaigns, ads; resolve the ad | GET | **User** |

Switching token or method and forgetting to switch back is the single most
common way to lose ten minutes here. Two real errors it produces:

| Error | What it actually means |
|---|---|
| `(#100) The parameter special_ad_categories is required` | Method still on POST. Graph is telling you what it needs to *create* a campaign, because you asked it to. |
| `(#100) The parameter creative is required` | Same, for creating an ad. |
| `(#100) Unsupported get request` on `act_.../campaigns` | Token still on the Page. A Page token cannot read an ad account. |

#### In Ads Manager

Ads Manager is at **adsmanager.facebook.com**. It may not appear in the
Facebook menu at all if the account has never run an ad, even though the ad
account exists and `/me/adaccounts` returns it — go direct.

Select the ad account first; a personal ad account appears under **Other
assets** rather than inside a business portfolio, and it can still run ads on a
Page owned by a portfolio.

Then **Create**, and four defaults to change:

1. **Objective: Engagement**, then **Manual engagement campaign**. The
   "tailored" setup will not let you attach an existing post.
2. **Ad set → Conversion location: "On your ad."** It defaults to **Message
   destinations**, which is the Messenger flow and offers no existing post.
3. **Ad set → Engagement type: "Interactions."** It defaults to **Video
   views**, and a video goal rejects a text or photo post with *"This post isn't
   compatible with the current campaign objective. Please enter an ID for a post
   that has a video."* That error names the post; the problem is the goal.
4. **Ad → Ad creative → Change post → Enter post ID**, and paste the
   `{page-id}_{post-id}` from your unpublished post.

**A text-only post fails Instagram validation:** *"Post uses only text. Post must
contain an image or video on Instagram."* If an Instagram profile is linked to
the Page you cannot unlink it in the ad, and Advantage+ placements cannot be
switched off in-line, so the practical fix is to give the post an image —
create it through `/{page-id}/photos` with `published=false` instead of
`/{page-id}/feed`.

#### The two that cost money

**Toggle the ad off at all three levels.** Campaign, ad set and ad each carry
their own switch, and the header reads **In draft** with the toggle off when a
level is safe. Turning off the campaign alone is not enough — the ad below it
stays on.

**Publishing requires a payment method on the ad account**, even for something
that will never deliver. There is no way around it: the ad object is not created
until Meta has a card. Set an account spending limit before you publish if that
makes you happier, and delete the campaign once you have read what you needed.

### What actually works

The post is invisible to every listing. Its comments are not:

```bash
curl -s -H "Authorization: Bearer $PAGE_TOKEN" \
  "https://graph.facebook.com/v25.0/$POST_ID/comments?fields=id,message,from"
```

That answers normally. So the capability is real and the **discovery** is what
is missing. To read comments on an ad you must already hold the post id, and
getting one means going through the Marketing API:

```text
ad id
  → /{ad-id}?fields=creative{effective_object_story_id}
      → the Page post id
          → /{post-id}/comments
```

Which inverts how most people would build this. Sweeping the Page is the
obvious design and it silently misses every ad. Resolving the ad to its post is
the awkward design and it is the only one that works.

**That chain needs `ads_read`**, added in [Step 1](#step-1-create-the-app-then-grant-the-permission-separately).
It is a separate App Review item from the Page permissions, and a token without
it fails at the very first hop — `/{ad-id}` — before any Page is involved.

**And you do not have to start from an ad id you looked up by hand.** The same
Marketing API that resolves an ad also lists what you are running:
`/me/adaccounts` gives the accounts this identity can reach, and
`/{account-id}/campaigns` gives the campaigns in one, by name. That turns "paste
a 17-digit id out of Ads Manager" into "list the campaigns, pick the one called
Spring Menu" — which matters more than it sounds, because the person who wants
to read the comments is rarely the person who knows the id.

**Two honest caveats.** My dark post was created through the API with
`published=false`. A post created by Ads Manager as an ad creative may be a
different object — I have not tested that, and it is the one thing that could
partly rescue the documented behaviour. And separately, some ad formats never
create a Page post at all; those comments are unreachable by any Page-based
route. If you build tooling here, make it distinguish "no comments" from "not
visible from here", because they look identical and mean opposite things.

## It works — verified end to end, 2026-09-08

Everything above is a negative result plus a proposed route. On 2026-09-08 I ran
the route itself, through the MCP server this work came out of, against a real
paused ad:

```json
{"operation":"read_comments",
 "pageId":"570933859669409",
 "postId":"570933859669409_122253830690055926",
 "ok":true}
```

Ad id in, creative resolved, `effective_object_story_id` giving the unpublished
post, Page token exchanged, comments edge read, one comment returned and triaged.
The chain holds.

Three things that had been assumptions until that run:

- **`from` is returned** on a comment on a dark post, so "has the Page replied"
  can be answered by author identity rather than the weaker "did anyone reply".
- **`from.id` is the Page id** for a comment the Page itself wrote — which is
  how you keep a Page's own replies out of your complaint counts.
- **`can_hide` is `false`** on the Page's own comment, `true` on a third party's.

None of those are documented. All three are now checked.

**What is still not proven:** every live comment I have read on a dark post was
written by the Page itself, because a paused ad reaches nobody — a real visitor
comment requires the ad to actually deliver. So triage of a stranger's comment,
on a dark post, remains untested. I would rather say that than imply a
completeness I have not earned.

## Reader downloads

Two scripts, both of which answer questions this article raises:

| File | What it does |
|---|---|
| `get-page-token.sh` | Exchanges a user token via `/me/accounts`, prints each Page's id, name and `tasks`, and flags which ones lack `MODERATE`. Never prints a token. |
| `check-permissions.sh` | Calls `debug_token` and diffs the granted scopes against the four permissions above, naming what is missing. |
| `.env.example` | Placeholders only. |

```bash
chmod 700 get-page-token.sh check-permissions.sh
export META_APP_ID=... META_APP_SECRET=... META_USER_TOKEN=...
./check-permissions.sh
./get-page-token.sh
```

Both scripts redact tokens from their own output on purpose. If you modify them,
keep that property — a token in a terminal is a token in your shell history.

## Related reading

- *Reading Facebook Page Comments with the Graph API: App Setup, Tokens, and
  Permissions* — the app, token and permission setup this article assumes,
  including the `ads_read` scope and where it actually lives.

- Adding the Use Case Didn't Add the Permission
- The Agent Told Me How to Protect My Token. It Was Wrong.
- The Page the API Couldn't See
- Business, Creator, and the Link That Isn't a Link
- The First Real Post
- The Field That Didn't Exist
- One Page Is Not a Platform

## Summary

- **`/{page-id}/feed` does not return unpublished posts.** Tested on two Pages,
  five days apart. `/feed` and `/posts` returned byte-identical id lists both
  times. `is_published=false` and `include_hidden=true` change nothing, and
  `/promotable_posts` does not exist.
- **Ads run on unpublished posts**, so no Page-level sweep reaches a single
  comment on your advertising.
- **The comments are readable** — the `/comments` edge on such a post answers
  normally. You just have to already hold the post id.
- **Getting one means the Marketing API**: ad → `creative{effective_object_story_id}`
  → post. That needs `ads_read`, which is a separate App Review item from the
  Page permissions.
- **You do not need to know an ad id by hand.** `/me/adaccounts` and
  `/{account-id}/campaigns` list what you are running, by name.
- **Some ad formats create no Page post at all** — dynamic creative built
  entirely in Ads Manager. Those comments are unreachable by any Page-based
  route, and no workaround changes that.
- **Verified end to end on 2026-09-08**, against a real ad on a real dark post.

---

*Part of the GrasshopperPebbles build-in-public series.*
