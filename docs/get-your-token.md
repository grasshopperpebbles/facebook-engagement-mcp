---
title: "Get your Meta access token"
description: "The short path to the one thing the installer asks you for. Five steps, about fifteen minutes, no code."
date: 2026-09-09
---

# Get your Meta access token

The install form asks for one thing you cannot type from memory: a Meta access
token. This page is the short path to it — five steps, about fifteen minutes,
no code. If a step goes wrong, each one links to the full explanation.

**Before you start, you need to be an admin of the Facebook Page** you want to
work with. Not just a poster or an editor — an admin. Nothing below works
otherwise.

## 1. Create a Meta app

Go to [developers.facebook.com/apps](https://developers.facebook.com/apps),
create an app, and choose the **Business** type.

Your app name cannot contain "Facebook", "Meta", "Insta" or "FB", or any near
variant. Meta rejects it without explaining why.

## 2. Ask for the permissions

Selecting a use case is *not* the same as being granted a permission — you still
have to add them on the use case and tick them when you mint the token.

**Page permissions** — App Dashboard → **Add Use Case** → **Pages** →
**Manage everything on your Page** → **Customize**, then add:

| Permission | What it gets you |
|---|---|
| `pages_show_list` | Which Pages you manage |
| `pages_read_engagement` | Page content and metadata |
| `pages_read_user_content` | Reading the comments — **required; tick it in the same dialog as the others**. Not the same as `pages_read_engagement` |
| `pages_manage_engagement` | Replying and hiding (only if you want writes) |
| `business_management` | `/me/accounts` actually returns Pages when they live under a Business Portfolio — without it the list is often empty |

**Ads (optional)** — a *second* use case: **Ads and monetization** → **Measure
ad performance data with Marketing API**. That is where **`ads_read`** lives;
it will not appear under Manage everything on your Page.

| Permission | What it gets you |
|---|---|
| `ads_read` | Comments on your ads |

Select the Page permissions **together** when you **Get User Access Token**.
Leave out `pages_read_user_content` and comment reads fail with a permissions
error. Leave out `ads_read` and comments on ads stay invisible — see
[why](dark-posts.md). Leave out `business_management` and `/me/accounts` may
return `{"data": []}` even when the Page permissions are present — see
[empty `/me/accounts`](setup-tokens-permissions.md#empty-data--from-meaccounts).

Full walkthrough: [setup, tokens and permissions](setup-tokens-permissions.md#where-the-page-permissions-live--manage-everything-on-your-page).

## 3. Generate a token

Open the [Graph API Explorer](https://developers.facebook.com/tools/explorer/),
select your app, open **User or Page → Get User Access Token**, select the
permissions above, and complete consent with **Edit settings** (not Continue).
Approve the Pages when it asks.

This token expires in about an hour. That is expected — step 4 fixes it.

**Check that Pages came through (no terminal).** In the Explorer path field,
submit:

```text
me/accounts?fields=id,name,tasks
```

You want your Page in the `data` array. Empty `data` usually means missing
`business_management` or an empty Page grant — see
[empty `/me/accounts`](setup-tokens-permissions.md#empty-data--from-meaccounts).
Developers who prefer a shell can use the `curl` examples in that guide; marketers
should stay in the Explorer.

## 4. Make it last

The one-hour token has to become a long-lived one (~60 days), and you need a
**Page** token (not the user token) for comments.

**In the browser:**

1. Copy the user token from the Explorer.
2. Open **Tools → Access Token Debugger**, paste it, **Debug**, then
   **Extend Access Token** (you will need the app secret from App Dashboard →
   App settings → Basic).
3. Back in the Explorer, open **User or Page** and select your Page under
   **Page Access Tokens**. That Page token is what the installer needs for
   comment reads.

Full detail: [the token chain](setup-tokens-permissions.md#get-a-long-lived-user-token-and-then-a-page-token-without-a-terminal).

Skipping this step is the most common reason the tool works for an hour and
then stops.

**Sixty days later it stops again**, and this path has no answer to that — you
repeat steps 3 and 4. If your Page lives in a business portfolio there is a
token that never expires: [the System User
token](system-user-token.md). It is more setup once, and none ever again.

## 5. Paste it in

Put the long-lived token in the install form's **Meta access token** field. It
is stored encrypted by Claude Desktop and never leaves your machine.

You are done. Ask Claude about comments on your Page.

## If something does not work

| What you see | What it usually means |
|---|---|
| `/me/accounts` empty | Missing `business_management` (common under Business Portfolio), or empty Page grant — re-authorise with **Edit settings**. [Empty `/me/accounts`](setup-tokens-permissions.md#empty-data--from-meaccounts) |
| No comments, no error | The token is a user token that was never exchanged, or you are not an admin of the Page. [The empty array](setup-tokens-permissions.md) |
| Permissions error on `/{post-id}/comments` | `pages_read_user_content` was missing from the mint dialog (it must be selected with the other Page permissions — not confused with `pages_read_engagement`). Remint the full set, re-select the Page. [Reading the comments](setup-tokens-permissions.md#reading-the-comments) |
| Nothing from your ads | `ads_read` was not granted. [Ads' comments are invisible](dark-posts.md) |
| A refusal naming `publish_actions` | A permission removed in 2018 — the message is misleading. [What it really means](publish-actions-error.md) |
| It worked, then stopped | The 60 days are up. Repeat steps 3 and 4 — or stop repeating them: a [System User token](system-user-token.md) does not expire |

The full setup reference, with every step explained and every command shown, is
[here](setup-tokens-permissions.md).
