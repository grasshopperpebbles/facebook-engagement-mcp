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

In the app, add these. Selecting a use case is *not* the same as being granted
a permission — you have to grant them explicitly.

| Permission | What it gets you |
|---|---|
| `pages_show_list` | Which Pages you manage |
| `pages_read_user_content` | Reading the comments |
| `pages_manage_engagement` | Replying and hiding (only if you want writes) |
| `ads_read` | Comments on your ads |

Leave out `ads_read` and comments on ads stay invisible — see
[why](dark-posts.md).

## 3. Generate a token

Open the [Graph API Explorer](https://developers.facebook.com/tools/explorer/),
select your app, select the permissions above, and click **Generate Access
Token**. Approve the Page when it asks.

This token expires in about an hour. That is expected — step 4 fixes it.

## 4. Make it last

The one-hour token has to be exchanged for a long-lived one, which lasts about
60 days. The exchange is a single request and the full guide has it ready to
paste: [the token chain](setup-tokens-permissions.md#step-3-the-token-chain).

Skipping this step is the most common reason the tool works for an hour and
then stops.

## 5. Paste it in

Put the long-lived token in the install form's **Meta access token** field. It
is stored encrypted by Claude Desktop and never leaves your machine.

You are done. Ask Claude about comments on your Page.

## If something does not work

| What you see | What it usually means |
|---|---|
| No comments, no error | The token is a user token that was never exchanged, or you are not an admin of the Page. [The empty array](setup-tokens-permissions.md) |
| Nothing from your ads | `ads_read` was not granted. [Ads' comments are invisible](dark-posts.md) |
| A refusal naming `publish_actions` | A permission removed in 2018 — the message is misleading. [What it really means](publish-actions-error.md) |
| It worked, then stopped | The 60 days are up. Repeat steps 3 and 4 |

The full setup reference, with every step explained and every command shown, is
[here](setup-tokens-permissions.md).
