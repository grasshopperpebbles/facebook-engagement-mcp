---
title: "Reading Facebook Page Comments with the Graph API: App Setup, Tokens, and Permissions"
date: 2026-09-02
verified: "Steps 1-5 walked against a live Page, 2026-09-03. ads_read / business_management / Explorer-first non-developer walkthrough verified and updated 2026-09-15."
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
>
> **How to follow this page if you are not a developer.** You do not need a
> terminal. Almost every check uses two browser tools under **Tools** in the
> [Meta developer site](https://developers.facebook.com/) header:
> **Graph API Explorer** (run queries, mint tokens) and **Access Token
> Debugger** (see scopes and expiry). Where a `curl` block appears, it is
> optional — the same step is described as clicks first.

## The short version

Four things have to line up, and each fails differently:

```text
1. APP        use case selected  ≠  permission granted
2. PAGE       admin in the UI    ≠  reachable by the API
3. TOKEN      user token         ≠  Page token   ← this is the empty-array one
4. ROLE       Page access        ≠  MODERATE task
```

If you only remember one thing: **comment reads through a user access token
return an empty array, not an error.** You need a Page access token. In Graph
API Explorer you get one by opening **User or Page** and selecting your Page
under **Page Access Tokens** (after a user token with the right scopes). The
API equivalent is exchanging a user token through `/me/accounts`.

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

### Where the Page permissions live — **Manage everything on your Page**

`pages_read_user_content` (and the other Page scopes in the table below) are
**not** under Ads and monetization. They belong to the Pages use case:

1. **App Dashboard → Add Use Case**.
2. Filter or browse under **Pages** (category names in the picker vary slightly;
   look for Page management, not ads).
3. Choose **Manage everything on your Page**.
4. **Save.** The use case appears under *App customization and requirements*.
5. Open **Customize the Manage everything on your Page use case**. Add the
   permissions from the table below if they are not already listed — including
   **`pages_read_user_content`**. Some defaults (`pages_show_list`, often
   `business_management`) may already be present; do not assume the comment
   scopes are among them.

Selecting this use case still does **not** put those permissions on an existing
token. After the dashboard lists them (Ready for testing is enough in
development mode), remint with **Get User Access Token** and tick them in that
dialog.

**Add these four — all of them, in the same token grant:**

| Permission | What it covers | App Review |
|---|---|---|
| `pages_show_list` | Which Pages this identity manages | Yes, for production |
| `pages_read_engagement` | Page content and metadata | Yes, for production |
| `pages_read_user_content` | **Comments and posts written by other people** | Yes |
| `pages_manage_engagement` | Create, edit and delete comments | Yes |

When you **Get User Access Token** in Graph API Explorer, tick **all four in
that one permissions dialog** (plus `business_management` and optionally
`ads_read` below). Do not treat `pages_read_user_content` as something to add
later — it is required for reading comments, and it is easy to confuse with
`pages_read_engagement`. They are different scopes. Engagement alone lets you
see Page-owned content; **user content** is what covers comments other people
wrote. Skip it at mint time and `/{post-id}/comments` fails with a permissions
error even though the other three are present.

Note the dependency: Meta's reference lists `pages_manage_engagement` as
requiring `pages_read_user_content` and `pages_show_list`. You cannot cherry-pick
the write permission and skip the read one.

### And `business_management` — or `/me/accounts` stays empty

Walked in Graph API Explorer, 2026-09-15: with the four Page permissions on a
fresh **user** token, consent completed via **Edit settings**, and Pages ticked,
`GET /me/accounts` still returned `{"data": []}`. Adding **`business_management`**
to the same token made the Pages appear.

So `pages_show_list` is necessary and **not sufficient** when your Pages live
under a Business Portfolio / Business Manager. Without `business_management`,
Graph accepts the call and answers with an empty list — the same shape as a
missing Page grant, which is why the re-authorise loop alone does not fix it.

Add it in the Explorer permission picker when you **Get User Access Token**. It
is a user-token scope. Confirm it in **Tools → Access Token Debugger** (Step 4)
alongside the four Page permissions.

### And a fifth for ads — which is not on that Pages use case at all

`ads_read` is the permission that lets you resolve an ad to the post behind it,
and it is the one the [Comments on ads](#comments-on-ads-where-the-documentation-is-wrong)
section below depends on entirely. **You will not find it under Manage
everything on your Page**, however far you scroll. It belongs to the Marketing
API, a different product surface.

It needs a second use case on the same app. Walked on screen, 2026-09-08:

1. **App Dashboard → Add Use Case**, and filter by **Ads and monetization**.
2. Choose **Measure ad performance data with Marketing API** — the read-only
   one. *Create & manage ads with Marketing API* is its read/write sibling and
   carries `ads_management`, which can create and pause campaigns. Reading
   comments never needs that, and it is a harder App Review case to argue.
3. **Save.** The dialog warns that not every use case can share an app; this
   pair can. Both then appear under *App customization and requirements*.
4. Open **Customize the Measure ad performance data with Marketing API use
   case**. Look at the permissions list. `ads_read` is already listed there, at
   **Ready for testing** — which means development mode works on your own
   assets without App Review, exactly as with the Pages permissions.
   `ads_management` sits beside it; leave it alone. You do not tick a box or
   press Add — selecting the use case in steps 1–3 already attached `ads_read`
   to the app.

**Step 4 is a confirmation, not an action.** The use case brings `ads_read`.
What remains is getting that scope onto a *token*. Adding a use case (or a
permission) to the app does not rewrite tokens you already have. A user token
minted before the Marketing use case existed still lacks `ads_read`, even when
the dashboard shows the permission as Ready for testing. That is the same trap
as at the top of this section.

To put those scopes on a token, remint a **user** access token. `ads_read` is a
user-token scope — the Marketing API does not use Page tokens for
`/me/adaccounts`, campaigns, or resolving an ad to its post. Do not pick a Page
under **Page Access Tokens** for this step.

Walked in the Explorer:

1. Open **Tools → Graph API Explorer**.
2. Select your app in the app dropdown.
3. Open the **User or Page** dropdown. Choose **Get User Access Token** — not
   **Get App Token**, not **Uninstall the app**, and not a Page name under
   **Page Access Tokens**. That menu item is what opens the permission picker
   for a user token; merely having "User Token" shown in the closed dropdown
   is not enough if the Access Token field is empty or stale.
4. In the permissions dialog, select **the full set in one pass** — not
   `ads_read` alone, and not the Page scopes without
   `pages_read_user_content`. Ticking only a newly remembered scope replaces
   the previous grant with a narrower one:

   | Permission | Why it is on this token | Required? |
   |---|---|---|
   | `pages_show_list` | `/me/accounts` — which Pages exist | Yes |
   | `pages_read_engagement` | Page content and metadata | Yes |
   | `pages_read_user_content` | Comments written by other people | **Yes — same dialog as the others** |
   | `pages_manage_engagement` | Reply / hide | Yes if you write; omit only for read-only |
   | `business_management` | `/me/accounts` under a Business Portfolio | Yes when the list would otherwise be `[]` |
   | `ads_read` | Ad accounts, campaigns, ad → post | Only if you need ads |

5. Complete the consent dialog (Pages and, if asked, ad accounts / businesses).
6. Confirm in **Access Token Debugger** (Step 4) that those scopes appear.
   Missing `business_management` with an empty `/me/accounts` is the same class
   of failure as missing `pages_show_list`.

**You can skip the ads half of this subsection.** Page comment reads do not need
`ads_read`. Everything else in this article still works without it. What fails
is only the ad path — resolving an ad to the unpublished post behind it, as
covered in the companion article on dark posts. Leaving `ads_read` off is also
a deliberate security choice: it is the **user** token that talks to your ad
account (`/me/adaccounts`, campaigns, creatives). A user token without
`ads_read` cannot read that account. The Page token is not involved in that
check — it never reads the ad account; it only reads comments once you already
have a post id. If the user token will only ever drive Page sweeps, remint with
the four Page permissions plus `business_management`, and omit `ads_read`.

## Step 2: Confirm the Page is actually reachable

`GET /me/accounts` returns the Pages this identity manages. It is also where a
whole class of confusion starts, because **the Pages the API returns and the
Pages you administer in the UI are not always the same set.**

### Error 2500 before you get a list at all

If you type `me/accounts` (or `/me/accounts`) in the Graph API Explorer and
get this:

```json
{
  "error": {
    "message": "An active access token must be used to query information about the current user.",
    "type": "OAuthException",
    "code": 2500,
    "fbtrace_id": "ARcajsCKxEzS8dLOkFSHRPh"
  }
}
```

Graph is refusing the call because `/me` means **the current user**, and the
request does not carry a usable **user** access token. This is not a missing
Page, a stale grant, or a missing `pages_show_list` — those fail differently
(or return a partial list). Fix the token first.

The usual causes in the Explorer, in the order to check them:

1. **No token yet.** The Access Token field at the top is empty. Open
   **User or Page → Get User Access Token**, complete the login/consent dialog,
   then submit the query again.
2. **A Page is selected under User or Page.** `/me/accounts` lists Pages
   managed by a *person*. A Page access token is not a user identity, so Meta
   treats the call as having no active token for the current user. Switch the
   dropdown back to your **user**, generate a user token if the field cleared,
   and retry. (You need the user token here anyway — that is how you obtain
   each Page's token from the response.)
3. **Expired or invalid token.** Short-lived Explorer tokens die in about an
   hour. Generate a fresh user token and retry.

### Empty `{"data": []}` from `/me/accounts`

A successful call that returns no Pages is not error 2500 and not the
comments empty-array trap from the top of this article. Graph accepted the
**user** token and answered: this app has been granted access to **zero** Pages
for that identity.

Check, in order:

1. **Confirm it is a user token and it carries `pages_show_list` and
   `business_management`.** In the Explorer, **User or Page** must be a user
   token from **Get User Access Token**, not a Page under **Page Access
   Tokens**. Then open **Tools → Access Token Debugger**, paste the token, and
   click **Debug**. If either scope is missing, remint and tick it. Verified
   2026-09-15: the four Page permissions alone still returned `{"data": []}`
   until `business_management` was added.
2. **Re-authorise so the Page grant is not empty.** Permissions on the token
   and Pages the app may touch are separate. You can hold the Page scopes (and
   even `business_management`) and still see `data: []` if consent never
   attached any Page (or the grant went stale). Fix:

   1. **User or Page → Uninstall the app.**
   2. **User or Page → Get User Access Token** again.
   3. Select the permissions you need (the four Page scopes,
      `business_management`, plus `ads_read` if you want ads).
   4. On the consent screen, **do not click Continue / Continue with your
      previous settings.** That button replays the *last* grant. If that grant
      had zero Pages — or omitted the Page you just care about — you get a
      valid token and `data: []` again, with nothing to tell you the dialog
      skipped the checklist. Click **Edit settings** (wording varies; anything
      that is not the fast Continue).
   5. Choose **Opt in to current Pages only** so Facebook shows the checklist,
      and **tick every Page you need** — including the one you are testing on.
      Confirm.
   6. Retry `me/accounts?fields=id,name,tasks`.

3. **Cross-check the Explorer's own Page list.** Open **User or Page** again.
   Under **Page Access Tokens**, do any Pages appear (for example the Page you
   administer)? If a Page is listed there but `/me/accounts` is still empty,
   select that Page to mint a Page token and keep going for comment reads —
   enumeration and "can I get a Page token" are not always the same question.
   If **no** Pages appear under **Page Access Tokens** either, the grant really
   has nothing; stay on Edit settings until the checklist shows Pages and you
   have ticked them.

4. **Only then treat it as ownership / portfolio.** If the grant is fresh
   (`Edit settings`, Pages ticked), `pages_show_list` and `business_management`
   are both present, **Page Access Tokens** is empty, and `/me/accounts` is
   still `data: []`, the problem is Business Portfolio / role / how the Page
   was created — not the Explorer click path. Common pattern: you administer
   the Page in the UI, but the app is not in the business portfolio that owns
   the Page, so enumeration returns nothing. Try `GET /{page-id}?fields=id,name`
   with the same user token if you know the Page id — access by id can succeed
   when the list is empty. See the cases below.

Once the call succeeds *with Pages in `data`*, interpret the list. Business
Portfolio structure, Page ownership, and how a Page was created all affect what
enumerates. I've hit two distinct versions of this:

- The Page the API Couldn't See —
  Graph returned five Pages; I administer seven, and one of the missing ones was
  open in the next tab. Enumeration and access are different questions.
- Business, Creator, and the Link That Isn't a Link —
  the account types look linked in the UI and are not linked in the way the API
  means.

Check what Graph actually sees before debugging anything downstream. You do
**not** need a terminal for this.

**In Graph API Explorer** (the path marketers should use — same tool you already
opened for the token):

1. Stay on a **user** token from **Get User Access Token** (not a Page under
   **Page Access Tokens**).
2. In the path field, enter:

   ```text
   me/accounts?fields=id,name,tasks
   ```

3. Leave the method on **GET** and click **Submit**.

You should see a `data` array of Pages. Each entry's `id` and `name` are what
matter; `tasks` should include `MODERATE` if you will reply or hide. If `data`
is `[]`, go back to the empty-list checklist above — including
`business_management` — before chasing ownership.

**Optional, for developers** — the same call from a shell:

```bash
curl -s -H "Authorization: Bearer $USER_TOKEN" \
  "https://graph.facebook.com/v25.0/me/accounts?fields=id,name,tasks"
```

If your Page is not in that list after `pages_show_list` and
`business_management` are both on the token, no amount of further permission
tuning will help. Fix the ownership or the role first.

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
2. Choose **Get User Access Token** again. Because the app is no longer
   installed, the full consent flow runs rather than silently reusing the old
   grant. Use **Edit settings**, not Continue.
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
| `/me/accounts` — which Pages exist | User | `pages_show_list` **and** `business_management` (without the latter, Business Portfolio Pages often yield `data: []`) |
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

### Get a long-lived user token (and then a Page token) without a terminal

The Explorer token lasts about an hour. For anything beyond a quick test you
want the longer chain. You can walk it in the browser:

1. Copy the short-lived **user** token from Graph API Explorer.
2. Open **Tools → Access Token Debugger**, paste it, click **Debug**.
3. At the bottom, click **Extend Access Token** (requires the app secret —
   Meta prompts for it; you get the secret from App Dashboard → **App
   settings → Basic**). The result is a long-lived user token (~60 days).
4. Back in Graph API Explorer, paste that long-lived token into the Access
   Token field **or** run **Get User Access Token** again only if you must —
   for the Page step you mainly need the long-lived user token in hand.
5. Open **User or Page**. Under **Page Access Tokens**, click your Page
   (for example the Page you administer). The Explorer switches to that
   Page's token. **That is the token that reads comments.**
6. Optional check: with the **user** token still selected, submit
   `me/accounts?fields=id,name,tasks` — you should see the Page and
   `MODERATE` in `tasks` if you will write.

**What you save for the MCP server is the long-lived user token** (step 3), not
the Page token from step 5. See [What next](#what-next).

**Do not** select a Page token and then try to read `/me/accounts` — that is
error 2500 or an empty list. Mint the Page token from the dropdown (or from
the `access_token` field in a `/me/accounts` response), then use *that* for
comments smoke tests only.

**Optional, for developers** — exchange short-lived for long-lived in a shell
(needs App ID and App Secret):

```bash
curl -s "https://graph.facebook.com/v25.0/oauth/access_token\
?grant_type=fb_exchange_token\
&client_id=$APP_ID\
&client_secret=$APP_SECRET\
&fb_exchange_token=$SHORT_LIVED_TOKEN"
```

Then list Pages and copy each Page's `access_token`:

```bash
curl -s -H "Authorization: Bearer $LONG_LIVED_USER_TOKEN" \
  "https://graph.facebook.com/v25.0/me/accounts?fields=id,name,access_token,tasks"
```

Per
[Meta's long-lived token guide](https://developers.facebook.com/docs/facebook-login/guides/access-tokens/get-long-lived),
Page access tokens derived from a long-lived user token do not carry an
expiration date — they "only expire or are invalidated under certain conditions."
That is a meaningful operational difference: you refresh the user token, not the
Page tokens.

If you paste tokens into tooling yourself: put them in an
`Authorization: Bearer` header, not an `access_token=` query parameter. Query
strings get recorded by proxies, CDNs and server logs; headers do not.

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

**Run through this stack on 2026-09-11, and it holds.** This paragraph used to
say the opposite — that Meta's non-expiring claim was one I was repeating rather
than testing — and the correction is kept rather than edited away, because this
article's whole point is the difference between those two things.

`debug_token` on a System User token returned **`expires_at: 0`**, and so did the
**Page token exchanged from it** — which is the half that actually decides it,
since a non-expiring user token buys nothing if the exchange hands back a 60-day
Page token. Comment reads then ran through the server on that identity.

Two things came out of it that the documentation does not tell you. It is a
**tighter** credential, not just a longer-lived one: the personal token on that
account reached seven Pages, the System User the one assigned to it. And the
setup is **longer**, not shorter — the app is a separate asset assignment from
the Page, and getting that wrong produces a refusal that names the remedy you
have already applied.

**Two setup details that cost time.** The app is a *separate* asset assignment
from the Page and only the System-User-side assignment counts; and the
permission picker is a scrolling multi-select where a missed tick is silent —
the first token issued during this verification came back without
`pages_manage_engagement`, read comments perfectly, and would have refused every
write. Check the scopes in **Access Token Debugger**, not only in the picker.

Full walkthrough, with the trap and a known-good output to compare against:
[The System User token and the 60-day expiry](system-user-token.md).

**Confirm you are on a Page token before reading comments.** In Graph API
Explorer, **User or Page** should show your Page name (not "User Token"). Using
the user token on `/{post-id}/comments` is what produces the empty array at the
top of this article.

## Step 4: Verify the permissions you actually got

Do not trust the App Dashboard screen. Ask the token what it carries.

**In the browser (preferred):**

1. Open **Tools → Access Token Debugger**.
2. Paste the token (user or Page — check each if unsure).
3. Click **Debug**.

The **Scopes** list on that screen is the truth. If `pages_read_user_content` is
missing, comment reads will not work no matter what the dashboard shows, because
the token was minted before the permission was added. Check
`business_management` the same way when `/me/accounts` is empty, and `ads_read`
when you need the ad path.

**Optional, for developers** — the same check via `debug_token` (needs App ID
and App Secret):

```bash
curl -s "https://graph.facebook.com/v25.0/debug_token\
?input_token=$TOKEN_TO_CHECK\
&access_token=$APP_ID|$APP_SECRET"
```

The `check-permissions.sh` script in [Reader downloads](#reader-downloads) does
this and diffs the result against the four Page permissions above. It does not
look for `ads_read` or `business_management`; the Debugger (or the `scopes`
array in the curl response) does.

## Step 5: Check the MODERATE task before you try to write

`/me/accounts` returns a `tasks` array per Page — the roles this identity holds
on it. Replying to or hiding a comment needs **`MODERATE`**.

This matters because the failure mode is bad. Without it you get a generic
permissions error that names neither the missing role nor the Page, and you go
hunting through app configuration for a problem that is actually a Page role
assigned in Business Settings.

**In Graph API Explorer**, with a **user** token:

1. Submit `me/accounts?fields=id,name,tasks`.
2. Find your Page in `data`.
3. Confirm `tasks` includes `MODERATE`.

If it does not, fix the role in Business Settings / Page roles — not the app
permissions list.

**Optional, for developers:**

```bash
curl -s -H "Authorization: Bearer $USER_TOKEN" \
  "https://graph.facebook.com/v25.0/me/accounts?fields=id,name,tasks"
```

## Reading the comments

With a **Page** token selected in Graph API Explorer (**User or Page** → your
Page name):

1. Put a real post id in the path. Prefer an id copied from a feed response
   (`{page-id}/feed` or `{page-id}/posts` while on the Page token) — usually
   shaped like `{page-id}_{post-id}`. The trailing number alone from a
   facebook.com URL is unreliable here.
2. Submit:

   ```text
   {post-id}/comments?fields=id,message,created_time,from,like_count,comment_count,is_hidden,can_comment,can_hide,permalink_url,parent&filter=toplevel&order=chronological
   ```

3. Leave method on **GET** → **Submit**.

You should see comments in `data`. Empty `data` with a user token still selected
means you are on the wrong token — switch to the Page. That failure is silent
(`{"data": []}`), not a permissions error.

### Error: missing permissions on `/{post-id}/comments`

A permissions refusal (often `(#200)` or `(#10)`, wording like *permission* /
*permissions*) is **not** the empty-array trap. Graph accepted a Page-shaped
call and rejected the scopes (or the Page grant behind them).

**Almost always this means `pages_read_user_content` was not on the user token
when the Page token was minted.** It belongs in the **same** **Get User Access
Token** permissions dialog as `pages_show_list`, `pages_read_engagement`, and
(if you write) `pages_manage_engagement` — see Step 1. It is not a later bolt-on.
`pages_read_engagement` is the lookalike that does **not** cover comments by
other people; if you ticked engagement and skipped user content, this is the
error you get.

Fix, in order:

1. **Confirm the Explorer is on the Page.** **User or Page** must show your
   Page name, not "User Token". A user token usually returns empty `data` for
   comments; a Page token missing `pages_read_user_content` returns a
   permissions error instead.
2. **Debug the Page token.** Open **Tools → Access Token Debugger**, paste the
   token from the Explorer (copy the Access Token field while the Page is
   selected), click **Debug**. Scopes must include **`pages_read_user_content`**
   alongside the other Page permissions you selected at mint time.
3. **Remint with the full set, then take a fresh Page token.** Page tokens are a
   snapshot of the user token at exchange time. You cannot fix a missing
   `pages_read_user_content` by selecting the Page again without regenerating
   the user token.
   1. **User or Page → Get User Access Token**.
   2. Tick **all** of the Step 1 Page permissions together — including
      **`pages_read_user_content`** — plus `business_management` if you needed
      it for `/me/accounts`, plus `ads_read` only if you need ads.
   3. Complete consent with **Edit settings** (not Continue) and tick the Page.
   4. **User or Page →** your Page under **Page Access Tokens** again.
   5. Retry the comments call.
4. **Check granular targets if the scope is present but the call still fails.**
   In the Debugger output, `pages_read_user_content` may list `target_ids`. If
   your Page's id is not among them, the permission exists on the token but not
   for that Page — uninstall the app, **Get User Access Token** again with the
   full set, and explicitly opt that Page in.
5. **Confirm the post id.** Submit `{page-id}/feed?fields=id,message` on the
   Page token, copy an `id` from `data`, and use that exact value in
   `{id}/comments?...`. Wrong or partial ids produce confusing refusals that
   look like permission problems.

**Optional, for developers:**

```bash
curl -s -H "Authorization: Bearer $PAGE_TOKEN" \
  "https://graph.facebook.com/v25.0/$POST_ID/comments\
?fields=id,message,created_time,from,like_count,comment_count,is_hidden,can_comment,can_hide,permalink_url,parent\
&filter=toplevel&order=chronological"
```

`filter=toplevel` returns top-level comments; `filter=stream` flattens replies
into the same list. Replies to a specific comment come from that comment's own
comments edge: `{comment-id}/comments`.

When that call returns comments, Meta setup is finished — jump to
[What next](#what-next) at the end of this page (install the server and paste
the long-lived **user** token).

### Hiding and unhiding is one call, not two

This surprised me. There is no hide endpoint and no unhide endpoint. There is one
`POST` with a boolean.

**In Graph API Explorer**, with the **Page** token selected:

1. Set the method dropdown to **POST**.
2. Path: `{comment-id}` (the comment's id from the list above).
3. Add a field `is_hidden` with value `true` (hide) or `false` (unhide).
4. **Submit**.

Graph answers `{"success": true}`. Worth noting: it can also answer with
`{"success": false}`, which is not a success. Check the body, not only that the
call "worked".

**Optional, for developers:**

```bash
# hide
curl -s -X POST -H "Authorization: Bearer $PAGE_TOKEN" \
  -d "is_hidden=true" "https://graph.facebook.com/v25.0/$COMMENT_ID"

# unhide — same endpoint, same field
curl -s -X POST -H "Authorization: Bearer $PAGE_TOKEN" \
  -d "is_hidden=false" "https://graph.facebook.com/v25.0/$COMMENT_ID"
```

## Comments on ads: a separate article

Ads usually run on **unpublished** posts, and — contrary to Meta's
documentation — no Page-level edge returns those. `/feed` does not. Nor does
`/posts`, `/published_posts`, `is_published=false`, or `include_hidden=true`.
So none of the setup above, done perfectly, reaches a single comment on your
advertising.

That finding, its reproduction, and the only route that does work — ad →
creative → `effective_object_story_id` → post — used to live in this article and
now has its own:
**[The Posts /feed Won't Return: Your Ads' Comments, and Anything Another App
Published](./dark-posts.md)**.

**That article now carries a second reason a post goes missing, and it is not
about ads at all: two Page tokens for the same Page, issued by the same app, are
shown different numbers of posts.** A token exchanged from a Business System
User returned three posts where one exchanged from a personally-granted user
token returned five — and read the missing two, and their comments, without
complaint. The missing posts are absent from every edge on this page with the
setup done perfectly, and cannot be read by id either. So a `/feed` sweep can be
short with nothing to say it is, and **the first thing to try is a token granted
by a person who administers the Page**. Confirmed 2026-09-15; it is partly
detectable, and [the article says how](./dark-posts.md#the-second-reason-a-post-is-missing-the-token-you-are-using).

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

In Graph API Explorer, set the version dropdown (for example `v25.0`) and leave
it there while you test. Do not build against an unversioned host.

If you are writing code: put the version in one place; do not scatter version
strings through the codebase.

As of September 2026, `v26.0` is current (released 2026-07-29) and `v25.0` is
available until 2028-07-28. I pin `v25.0` deliberately — a version that has been
in the field for months has known behaviour, and the newest one does not. Review
the pin on a schedule, not on a whim.

## Reader downloads (optional — developers)

Shell scripts that answer the same questions this article raises in the
Explorer. Skip this section if you are testing only in the browser.

| File | What it does |
|---|---|
| `get-page-token.sh` | Exchanges a user token via `/me/accounts`, prints each Page's id, name and `tasks`, and flags which ones lack `MODERATE`. Never prints a token. |
| `check-permissions.sh` | Calls `debug_token` and diffs the granted scopes against the four Page permissions above, naming what is missing. |
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

- [The Posts /feed Won't Return: Your Ads' Comments, and Whatever Your Token Can't See](./dark-posts.md)
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
  access token (**User or Page →** your Page). This single fact explains most
  "the API returns nothing" reports.
- A **permissions error** on `/{post-id}/comments` means
  `pages_read_user_content` was not on the user token when the Page token was
  minted (or the Page is missing from that scope's `target_ids`). It should have
  been ticked **with** the other Page permissions in **Get User Access Token** —
  not added later, and not confused with `pages_read_engagement`. Remint the
  full set, re-select the Page, retry. Confirm in **Access Token Debugger**.
- **Error 2500** (`An active access token must be used to query information
  about the current user`) on `/me/accounts` means the Explorer has no usable
  **user** token — empty field, Page selected in User or Page, or expired.
  Generate a user token and retry; this is not a missing-Page problem.
- **`/me/accounts` returning `{"data": []}`** means the user token worked but
  Graph is not enumerating any Pages for this app. First confirm
  `business_management` and `pages_show_list` are both in `scopes` (verified:
  Page scopes alone were not enough). Then uninstall, **Get User Access
  Token**, click **Edit settings** (never **Continue with previous settings**),
  tick the Pages you need. Do not confuse this with the comments empty-array
  (that one is using a user token where a Page token is required).
- Selecting a **use case is not granting a permission**, and a token minted
  before you added a permission does not carry it. Verify in **Access Token
  Debugger**. Page scopes (including `pages_read_user_content`) live under
  **Add Use Case → Pages → Manage everything on your Page**. For ads: a second
  use case under **Ads and monetization**, then **Get User Access Token** with
  the Page scopes, `business_management`, and `ads_read` — not `ads_read`
  alone. Skip `ads_read` if you only sweep Pages.
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
  app permission. Check `tasks` on `me/accounts` in the Explorer before you
  debug your app config.
- `from` and `can_hide` **are returned** — I checked against a live Page — but
  neither is documented, so neither is promised. Compute "needs a reply" both
  ways and report which basis you used, because the weaker answer looks exactly
  like the stronger one.
- **"All current and future Pages" is a snapshot, not a subscription.** If a
  Page is missing from `/me/accounts`, uninstall the app in the Graph API
  Explorer and re-authorise before assuming an ownership problem.

## What next

If the Explorer smoke test returned comments, Meta configuration is done. Do
this next:

1. **Keep the long-lived user token** from **Access Token Debugger → Extend
   Access Token** — not the Page token you used to test comments. The server
   field is `META_ACCESS_TOKEN`; it exchanges the user token for Page tokens
   itself.
2. **Install facebook-engagement-mcp.** Prefer the Node `.mcpb` bundle
   (double-click in Claude Desktop, no terminal):
   **[Install and configure](../node/README.md#install-and-configure)**. Other
   languages: [repository README](../README.md#pick-your-language).
3. **Paste that long-lived user token** into **Meta access token** on the
   install form (or your client's env / config). Leave writes off until you
   intend to reply or hide.
4. **Ask the model to triage comments** on your Page (unanswered threads, a
   specific post, or orientation with no target to list what the token can
   reach).

Still useful after that:

- Ads / unpublished posts: [dark posts](./dark-posts.md)
- No 60-day renewal for a team: [System User token](./system-user-token.md)
- Write refused as `publish_actions`: [that error](./publish-actions-error.md)

---

*Part of the GrasshopperPebbles build-in-public series. This article is the
configuration reference; the finding about ad comments has its own piece, linked
above.*
