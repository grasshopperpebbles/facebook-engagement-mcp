---
title: "Reading Facebook Page Comments with the Graph API: App Setup, Tokens, and Permissions"
date: 2026-09-02
verified: "Steps 1-5 walked against a live Page, 2026-09-03. The ads_read additions of 2026-09-07 are NOT verified live — no ad has been resolved yet."
---

<!-- Source of truth for this article is the broadkast content repo,
     under grasshopperpebbles articles/facebook-engagement-mcp-setup/article.md.
     This copy exists so the README can link to something that renders
     code correctly and does not depend on a site being live. -->


# Reading Facebook Page Comments with the Graph API: App Setup, Tokens, and Permissions

You have a Facebook Page. You have an access token. You call
`/{post-id}/comments` and Graph returns this:

```json
{ "data": [] }
```

There are comments on that post. You can see them in another tab. The API is
not broken, your token is not expired, and nothing in the response tells you
what is wrong.

This article is about that gap — the Meta app configuration that sits between
"I have a token" and "I can read comments" — and about the three or four other
places it bites before you get a working call. It exists because I kept hitting
them while building an [MCP](https://modelcontextprotocol.io/) server for Page
comment triage, and because Meta's own documentation covers each piece
separately and the join between them nowhere.

> **What this article is and isn't.** Everything below about endpoints,
> permissions and token types is drawn from
> [Meta's current Graph API documentation](https://developers.facebook.com/docs/graph-api/)
> and checked against it as of September 2026. **Steps 1 through 5 were then
> walked start to finish against a real Page**, and three things turned up that
> the documentation does not mention — the app-name restriction in Step 1, the
> stale grant in Step 2, and the answer to the `from` question further down.
> Where the documentation is still ambiguous, I say so rather than guessing.

## The short version

Four things have to line up, and each fails differently:

```text
1. APP        use case selected  ≠  permission granted
2. PAGE       admin in the UI    ≠  reachable by the API
3. TOKEN      user token         ≠  Page token   ← this is the empty-array one
4. ROLE       Page access        ≠  MODERATE task
```

If you only remember one thing: **comment reads through a user access token
return an empty array, not an error.** You need a Page access token, and you get
one by exchanging your user token through `/me/accounts`.

## Why comments are the hard case

Reading a Page's own posts is easy. Reading the *comments* on those posts is not,
because comments are content written by other people. Meta treats that as a
meaningfully higher bar — `pages_read_user_content` is described as covering
"posts, comments, and ratings by users or other Pages," and it requires App
Review in a way that basic Page reads do not.

That distinction is the reason so many "it works in Graph API Explorer but not in
my app" threads exist. The Explorer runs with permissions you granted yourself,
against a Page you administer, in development mode. Production is a different
question.

## Step 1: Create the app, then grant the permission separately

Create an app at [developers.facebook.com](https://developers.facebook.com/), and
add **Facebook Login for Business** as the product.
[Facebook Login for Business](https://developers.facebook.com/documentation/facebook-login/facebook-login-for-business)
lets you define a configuration naming the token type, assets and permissions
your app needs, which your users then consent to as one bundle.

**You cannot call it what it is.** Meta rejects its own brand names in app names
— `Facebook`, `Meta`, `Insta`, `Gram`, `Book`, `FB` and near-variants all bounce.
So the obvious name for a Facebook comments integration is not available to you,
and you find that out only after typing it.

Name it for the job instead: *Comment Engagement*, *Page Engagement*,
*Comment Triage*. This is worth a moment's thought rather than a placeholder,
because the name is one of the things a reviewer weighs later — App Review asks
whether your stated use case justifies reading and managing content other people
wrote, and a name that plainly describes comment moderation helps that case. The
name has no technical effect; your code only ever sees the App ID.

**On reusing an old app.** If you have a dormant app lying around, reusing it is
tempting — its Page connections may already work, and adding permissions neither
invalidates existing tokens nor disturbs ones already through review. But only
do it if you know what it was for. An app whose configuration and review history
you cannot account for is a bad foundation for a client-facing integration, and
if *Meta* disabled it rather than you, re-enabling may not be stable. Start
fresh unless you can say what the old app was doing.

Here is the trap. Selecting a **use case** in the App Dashboard does not by itself
grant the permissions inside it. The use case describes what your app is for; the
permissions still have to be added, and a token minted before you added them
carries the old scopes. You get an `invalid_scope` error on a permission you can
see listed on the screen in front of you.

I wrote this one up when it happened:
Adding the Use Case Didn't Add the Permission.

**Add these four:**

| Permission | What it covers | App Review |
|---|---|---|
| `pages_show_list` | Which Pages this identity manages | Yes, for production |
| `pages_read_engagement` | Page content and metadata | Yes, for production |
| `pages_read_user_content` | **Comments and posts written by other people** | Yes |
| `pages_manage_engagement` | Create, edit and delete comments | Yes |

Note the dependency: Meta's reference lists `pages_manage_engagement` as
requiring `pages_read_user_content` and `pages_show_list`. You cannot cherry-pick
the write permission and skip the read one.

### And a fifth for ads — which is not on that page at all

`ads_read` is the permission that lets you resolve an ad to the post behind it,
and it is the one the [Comments on ads](#comments-on-ads-where-the-documentation-is-wrong)
section below depends on entirely. **You will not find it in the permissions
list you just used.** It belongs to the Marketing API, a different product
surface, and the Pages use case will never show it however far you scroll.

It needs a second use case on the same app. Walked on screen, 2026-09-08:

1. **App Dashboard → Add Use Case**, and filter by **Ads and monetization**.
2. Choose **Measure ad performance data with Marketing API** — the read-only
   one. *Create & manage ads with Marketing API* is its read/write sibling and
   carries `ads_management`, which can create and pause campaigns. Reading
   comments never needs that, and it is a harder App Review case to argue.
3. **Save.** The dialog warns that not every use case can share an app; this
   pair can. Both then appear under *App customization and requirements*.
4. Open **Customize the Measure ad performance data with Marketing API use
   case**. `ads_read` is already listed there, at **Ready for testing** — which
   means development mode works on your own assets without App Review, exactly
   as with the Pages permissions. `ads_management` sits beside it; leave it
   alone.

So there is nothing to *add* at step 4 — the use case brings it. What remains
is minting a token that carries it, which is the trap from the top of this
section: a token issued before the use case existed does not gain the scope.
Re-mint from **Tools → Graph API Explorer**.

Skip all of this and everything else in the article still works. Only the ad
path stops — and a token that cannot read your ad account is a smaller thing to
hand out, which is a fair reason to leave it off a token that will only ever
sweep Pages.

## Step 2: Confirm the Page is actually reachable

`GET /me/accounts` returns the Pages this identity manages. It is also where a
whole class of confusion starts, because **the Pages the API returns and the
Pages you administer in the UI are not always the same set.**

Business Portfolio structure, Page ownership, and how a Page was created all
affect what enumerates. I've hit two distinct versions of this:

- The Page the API Couldn't See —
  Graph returned five Pages; I administer seven, and one of the missing ones was
  open in the next tab. Enumeration and access are different questions.
- Business, Creator, and the Link That Isn't a Link —
  the account types look linked in the UI and are not linked in the way the API
  means.

Check what Graph actually sees before debugging anything downstream:

```bash
curl -s -H "Authorization: Bearer $USER_TOKEN" \
  "https://graph.facebook.com/v25.0/me/accounts?fields=id,name,tasks"
```

If your Page is not in that list, no amount of permission tuning will help. Fix
the ownership or the role first.

### The grant goes stale, and "all future Pages" does not mean what it says

This is the one that cost me the most time, and I have not seen it documented
anywhere.

My app's consent screen offered two choices:

> **Opt in to all current and future Pages** — This will give the app access to
> your current Pages, in addition to any Pages you create in the future.
>
> **Opt in to current Pages only**

The first was already selected. And `/me/accounts` still returned five Pages
when I administer seven — with the Page I actually wanted to test on among the
missing two.

So "all current and future Pages" describes the *option*, not the *grant*. The
grant is a snapshot taken when you authorise. Pages that arrive afterwards — or
that you gain a role on afterwards — are not retroactively added, whatever the
radio button says.

The fix is to re-authorise, and the only lever for that is blunt:

1. In the Graph API Explorer, open the **User or Page** dropdown and choose
   **Uninstall the app**. There is no per-Page control here; it is all or
   nothing.
2. Click **Generate Access Token** again. Because the app is no longer
   installed, the full consent flow runs rather than silently reusing the old
   grant.
3. Choose **Opt in to current Pages only** if you want to see the list — that
   radio button is also the only place Facebook will show you exactly which
   Pages the app can be given.

After that, seven Pages. Same account, same permissions, same everything —
only the grant was refreshed.

Two things follow. If a Page is missing, re-authorise before you go hunting
through Business Settings for an ownership problem you may not have. And if you
are building for other people, assume their grant is stale too: a Page they
added last week may be invisible to you, with no error to explain it.

## Step 3: The token chain

There are three tokens, and people conflate the first two with the third
constantly.

```text
short-lived user token   (~1 hour, from Login or the Explorer)
        │
        │  GET /oauth/access_token?grant_type=fb_exchange_token
        ▼
long-lived user token    (~60 days)
        │
        │  GET /me/accounts?fields=access_token
        ▼
Page access token        (does not expire, if derived from a long-lived user token)
        │
        └─►  this is the one that reads comments
```

### Which token does which job — and why the permission list changes

Permissions are granted to the **user** token. A Page token is *derived* from
it and inherits nothing automatically: what it can do is the intersection of the
scopes on the user token and the **role** you hold on that Page.

That is why the Graph API Explorer shows you a different permission list when
you switch **User or Page** from your user to a Page. Nothing changed about your
app; you are looking at a different token.

| Call | Token | What gates it |
|---|---|---|
| `/me/accounts` — which Pages exist | User | `pages_show_list` |
| `/me/adaccounts`, `/{account}/campaigns`, resolving an ad to its post | **User** | `ads_read` |
| `/{post-id}/comments` — reading comments | **Page** | `pages_read_user_content` + the Page role |
| Replying, hiding | **Page** | `pages_manage_engagement` + `MODERATE` on the Page |
| Creating an unpublished post | **Page** | `pages_manage_posts` |

Two of those trip people up.

**`ads_read` is a user-token scope, not a Page one.** The Marketing API knows
nothing about Pages; you resolve an ad with the user token and only then switch
to the Page token to read the comments on the post you resolved. A single ad
query therefore uses *both* tokens, in that order.

**`pages_show_list` is meaningless on a Page token.** It answers "which Pages
does this identity manage", which a Page token cannot ask — it already is one
Page. Do not go looking for it there.

**Exchange short-lived for long-lived:**

```bash
curl -s "https://graph.facebook.com/v25.0/oauth/access_token\
?grant_type=fb_exchange_token\
&client_id=$APP_ID\
&client_secret=$APP_SECRET\
&fb_exchange_token=$SHORT_LIVED_TOKEN"
```

Per
[Meta's long-lived token guide](https://developers.facebook.com/docs/facebook-login/guides/access-tokens/get-long-lived),
Page access tokens derived from a long-lived user token do not carry an
expiration date — they "only expire or are invalidated under certain conditions."
That is a meaningful operational difference: you refresh the user token, not the
Page tokens.

### Sixty days later, it breaks

Worth saying plainly, because the token chain above reads like a setup step and
is actually a recurring chore. **A long-lived user token lasts about 60 days.**
There is no refresh token and no warning. It works, and then one day a call fails
and someone has to walk the chain again.

That arithmetic is fine for one person and unpleasant for a team: every person
with their own token is another re-mint every two months, each one done by
whoever owns the app, and each expiry breaks somebody's tooling mid-task.

The way out — if your Pages live in a business portfolio — is a **System User**:
a non-human identity in Business settings, whose tokens are not on the 60-day
clock. That turns a recurring chore into a one-off.

**I have not run one through this stack.** Meta documents System User tokens as
long-lived or non-expiring depending on how they are issued; that is a claim I am
repeating rather than one I have tested, and this article's whole point is the
difference between those two things. If you try it, verify the same way as any
other token: `/me/accounts` returns the Pages you expect, and `debug_token` shows
the scopes and the expiry.

**Then get the Page token:**

```bash
curl -s -H "Authorization: Bearer $LONG_LIVED_USER_TOKEN" \
  "https://graph.facebook.com/v25.0/me/accounts?fields=id,name,access_token,tasks"
```

The `access_token` in each entry is that Page's token. **This is the token that
reads comments.** Using the user token here is what produces the empty array at
the top of this article.

One thing worth saying plainly, because an AI assistant told me the wrong version
of it and I believed it for longer than I should have: put the token in an
`Authorization: Bearer` header, not an `access_token=` query parameter. Query
strings get recorded by proxies, CDNs and server logs; headers do not. I wrote
that up in
The Agent Told Me How to Protect My Token. It Was Wrong.

## Step 4: Verify the permissions you actually got

Do not trust the App Dashboard screen. Ask the token what it carries:

```bash
curl -s "https://graph.facebook.com/v25.0/debug_token\
?input_token=$TOKEN_TO_CHECK\
&access_token=$APP_ID|$APP_SECRET"
```

The `scopes` array in the response is the truth. If `pages_read_user_content` is
missing from it, comment reads will not work no matter what the dashboard shows,
because the token was minted before the permission was added.

The `check-permissions.sh` script in [Reader downloads](#reader-downloads) does
this and diffs the result against the four permissions above.

**If you added `ads_read`, check for it here too.** It is the permission most
likely to be missing without you noticing, because nothing about a Page sweep
fails without it — you simply find, later and confusingly, that you cannot get
at the comments on your ads. `check-permissions.sh` does not look for it; the
`scopes` array in the `debug_token` response above does.

If you would rather click than curl, the same answer is in **Tools → Access
Token Debugger**, which lists the token's scopes and its expiry on one screen.
Both menu items live under **Tools** in the developer site header, alongside the
Graph API Explorer.

## Step 5: Check the MODERATE task before you try to write

`/me/accounts` returns a `tasks` array per Page — the roles this identity holds
on it. Replying to or hiding a comment needs **`MODERATE`**.

This matters because the failure mode is bad. Without it you get a generic
permissions error that names neither the missing role nor the Page, and you go
hunting through app configuration for a problem that is actually a Page role
assigned in Business Settings.

Check first:

```bash
curl -s -H "Authorization: Bearer $USER_TOKEN" \
  "https://graph.facebook.com/v25.0/me/accounts?fields=id,name,tasks" \
  | python3 -m json.tool
```

If `tasks` for your Page does not include `MODERATE`, fix the role, not the code.

## Reading the comments

With a Page token in hand:

```bash
curl -s -H "Authorization: Bearer $PAGE_TOKEN" \
  "https://graph.facebook.com/v25.0/$POST_ID/comments\
?fields=id,message,created_time,from,like_count,comment_count,is_hidden,can_comment,can_hide,permalink_url,parent\
&filter=toplevel&order=chronological"
```

`filter=toplevel` returns top-level comments; `filter=stream` flattens replies
into the same list. Replies to a specific comment come from that comment's own
comments edge: `/{comment-id}/comments`.

### Hiding and unhiding is one call, not two

This surprised me. There is no hide endpoint and no unhide endpoint. There is one
`POST` with a boolean:

```bash
# hide
curl -s -X POST -H "Authorization: Bearer $PAGE_TOKEN" \
  -d "is_hidden=true" "https://graph.facebook.com/v25.0/$COMMENT_ID"

# unhide — same endpoint, same field
curl -s -X POST -H "Authorization: Bearer $PAGE_TOKEN" \
  -d "is_hidden=false" "https://graph.facebook.com/v25.0/$COMMENT_ID"
```

Graph answers `{"success": true}`. Worth noting: it can also answer HTTP 200 with
`{"success": false}`, which is not a success. Check the body, not the status code.

## Comments on ads: a separate article

Ads usually run on **unpublished** posts, and — contrary to Meta's
documentation — no Page-level edge returns those. `/feed` does not. Nor does
`/posts`, `/published_posts`, `is_published=false`, or `include_hidden=true`.
So none of the setup above, done perfectly, reaches a single comment on your
advertising.

That finding, its reproduction, and the only route that does work — ad →
creative → `effective_object_story_id` → post — used to live in this article and
now has its own:
**[Facebook's /feed Does Not Return Unpublished Posts — So Your Ads' Comments Are
Invisible](./dark-posts.md)**.

It assumes the setup on this page, plus the `ads_read` scope from
[Step 1](#and-a-fifth-for-ads--which-is-not-on-that-page-at-all).


## The fields that work but aren't documented

Two fields in the request above are load-bearing, undocumented, and — I can now
say, having checked — actually returned.

**`from`** — the comment's author. It is *not* listed among the Comment node's
fields in the current v26.0 reference, which had me expecting it might be
withheld for comments written by other people. It isn't. A comment posted by a
personal account on a Page I administer came back as:

```json
{ "from": { "name": "Les Green", "id": "28452515681102242" }, "can_hide": true }
```

That `id` is app-scoped — not the person's real Facebook id — but it is stable
and, crucially, it differs from the Page's own id. Which is the whole point.

This is not a detail. If you cannot tell who wrote a reply, you cannot answer
*"has our Page already responded to this?"* — only the weaker *"did anyone
reply?"* Those are different questions, and a moderation queue that confuses
them will tell you a waiting customer has been handled.

**`can_hide`** — also absent from the field list, also returned, and meaningful:
`true` on the third party's comment above, `false` on a comment posted by the
Page itself. You cannot hide yourself.

**Build for absence anyway.** Both fields work today and neither is documented,
which means neither is promised. My own tooling computes "needs a reply" two
ways — from author identity when `from` is present, and from a bare reply count
when it isn't — and reports which one it used. That felt like over-engineering
until I realised the failure mode is silent: the weaker answer looks exactly
like the stronger one.

Requesting a field Graph does not recognise fails the entire call, not just that
field. And a field that exists in your mocks but not in the API produces a
different, more annoying class of bug — I wrote about that in
The Field That Didn't Exist.

Design for absence: treat every field except `id` as optional, and make your code
say which question it actually answered.

## Development mode vs App Review

This is the wall, and it is worth understanding before you build anything on a
deadline.

**Development mode** grants your app unreviewed permissions for Pages you hold a
role on. That is enough to build, test and verify shapes. It is not enough to
serve anyone else.

**App Review** is required for `pages_read_user_content` and
`pages_manage_engagement` on any Page outside your own app. Meta asks for
specific examples of why your app needs to read user-generated content, and to
manage comments on behalf of other users, on Pages they own. A vague answer gets
rejected.

If you are an agency planning to manage client Pages, App Review is the long pole
in your schedule. Start it before you need it.

One more structural point that bit me: a token, an app and a Page each scope
differently, and "I have access" is not one fact but several. I wrote about the
version of this that produces error 368 in
One Page Is Not a Platform.

## Pin the API version

Do not build URLs against an unversioned host and do not scatter version strings
through your code. Put it in one place.

As of September 2026, `v26.0` is current (released 2026-07-29) and `v25.0` is
available until 2028-07-28. I pin `v25.0` deliberately — a version that has been
in the field for months has known behaviour, and the newest one does not. Review
the pin on a schedule, not on a whim.

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

- [Facebook's /feed Does Not Return Unpublished Posts — So Your Ads' Comments Are Invisible](./dark-posts.md)
  — the companion piece. If you run ads, read it; this configuration alone does
  not reach those comments.
- [Meta Told Me I Needed a Permission That Died in 2018](./publish-actions-error.md)
  — what to do when a write is refused for `publish_actions`. Everything on this
  page can be correct and a write still fail, because the token you send decides
  whether you are publishing as the Page or as yourself.

The setup above is the resolution to a series of things that went wrong first:

- Adding the Use Case Didn't Add the Permission
- The Agent Told Me How to Protect My Token. It Was Wrong.
- The Page the API Couldn't See
- Business, Creator, and the Link That Isn't a Link
- The First Real Post
- The Field That Didn't Exist
- One Page Is Not a Platform

## Summary

- A **user access token returns an empty array** for comments. You need a Page
  access token from `/me/accounts`. This single fact explains most "the API
  returns nothing" reports.
- Selecting a **use case is not granting a permission**, and a token minted
  before you added a permission does not carry it. Verify with `debug_token`.
- **You cannot name the app after the platform.** `Facebook`, `Meta` and their
  near-variants are rejected. Name it for the job it does — the name is also
  something App Review weighs later.
- Comments on ads live on **unpublished posts**, and **no Page-level edge
  returns them** — the subject of its own article, linked above. If you run ads,
  the setup on this page is necessary and not sufficient.
- **Hiding and unhiding are one endpoint** with an `is_hidden` boolean — and a
  200 response can still say `success: false`.
- `pages_read_user_content` and `pages_manage_engagement` both require **App
  Review** for Pages outside your own app. Development mode covers Pages you
  personally administer, and nothing more.
- Writes need the **`MODERATE` task** on the Page, which is a Page role, not an
  app permission. Check `tasks` before you debug your app config.
- `from` and `can_hide` **are returned** — I checked against a live Page — but
  neither is documented, so neither is promised. Compute "needs a reply" both
  ways and report which basis you used, because the weaker answer looks exactly
  like the stronger one.
- **"All current and future Pages" is a snapshot, not a subscription.** If a
  Page is missing from `/me/accounts`, uninstall the app in the Graph API
  Explorer and re-authorise before assuming an ownership problem.

---

*Part of the GrasshopperPebbles build-in-public series. This article is the
configuration reference; the finding about ad comments has its own piece, linked
above.*
