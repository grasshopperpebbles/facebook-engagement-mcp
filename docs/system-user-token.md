---
title: "The System User token, and the 60-day expiry"
description: "A Business Manager System User is the usual answer to re-minting a token every 60 days. What is verified here, what is not, and how to settle it yourself."
date: 2026-09-11
---

# The System User token, and the 60-day expiry

> **Status: partly verified, and the important half is not.**
> The Business Manager navigation below was watched on screen on 2026-09-11.
> **Whether a System User token actually carries no expiry has not been run
> here**, and neither has whether this server's Page-token exchange survives
> one. Meta's documentation says System User tokens do not expire; this project
> has been wrong eleven times believing a sentence it had not watched, so that
> claim sits here as a claim. The last section is how to settle it in one
> command — and if you settle it before we do, the result is worth an issue.

## The problem this is meant to solve

[Get your Meta access token](get-your-token.md) ends with a long-lived token
that lasts about 60 days. Then it stops, and someone repeats steps 3 and 4.

For one person that is an annoyance. For a team it is the whole cost of running
this: ten people is ten token exchanges every two months, and they do not
expire on the same day. Nothing about the tool degrades gracefully when a token
dies — comment reads with a dead token return an error, and comment reads with
a *user* token return an empty array, which looks like a quiet Page rather than
a broken install.

A **System User** is a non-human identity that belongs to a Business Manager
rather than to a person. Its token is issued from Business settings instead of
the Graph API Explorer. This is the standard answer to the treadmill.

## What is verified

These were watched on screen rather than read in a doc.

**Your Page must be *owned* by the Business Manager, not shared into it.** Open
[business.facebook.com/settings](https://business.facebook.com/settings) →
**Accounts** → **Pages**. Select the Page and read the line under its name: it
should say **Owned by:** your business portfolio. If it offers **Request
access** rather than **Add**, the Page belongs to a different Business Manager
and someone on that side has to approve the move.

**The Page id is on that screen**, under the Page's name. You will need it.

**"People" is not "Pages", and the distinction bites.** The **People** list can
show an entry named after your brand — an Instagram-linked account with
portfolio access — which reads at a glance like the Page being present. It is
not the Page. Only the **Pages** list under **Accounts** answers that question.

## Setting it up

**1. Create the System User.** Business settings → **Users** → **System
users** → **Add**. Name it after the thing that uses it, not after a person.
Role: **Admin**.

**2. Assign it the Page.** On the System User, **Assign assets** → **Pages** →
your Page → **full control**, or at minimum the Manage-Page tasks that include
moderation.

This step is the one that decides everything. **A System User inside a
portfolio that owns the Page does not thereby reach the Page** — the asset
assignment is a separate act, and skipping it produces a failure that reads
like a permissions problem and is not one. See *When it goes wrong* below.

**3. Generate the token.** On the System User, **Generate new token** → select
your Meta app → tick the permissions this server needs:

| Permission | What it gets you |
|---|---|
| `pages_show_list` | Which Pages the identity manages |
| `pages_read_user_content` | Reading the comments |
| `pages_manage_engagement` | Replying and hiding (only if you want writes) |
| `ads_read` | Comments on your ads — see [why](dark-posts.md) |

**4. Paste it into the install form's Meta access token field**, in place of the
60-day one. The server does not care which kind of token it holds: it sends
whatever it is given to `/me/accounts` and takes the per-Page tokens from the
reply.

## When it goes wrong

Two failures look identical from the outside and have opposite fixes. This
server tells them apart.

**"Graph listed Page X and returned no access token for it."** The identity can
see the Page and cannot act as it — almost always step 2 missing or role-less.
**Re-authorising will not help**, and re-minting the token will not either. Go
back and assign the Page as an asset with a Page role.

**"No Page token for X, and Graph did not list the Page at all."** The Page is
not reachable by this identity at all — wrong business portfolio, or a grant
that predates the Page being added.

**A refusal naming a `MODERATE` role you believe you hold.** The role list
comes from Graph, and an *absent* role list is not an empty one. If this fires
on a System User that plainly has full control, it is worth an issue — that
distinction is newly handled and has not been seen in the wild.

## Settling the expiry question yourself

From a clone of the monorepo:

```bash
META_ACCESS_TOKEN=<your system user token> \
  node servers/page-engagement/scripts/write-probe.mjs inspect <your-page-id>
```

Read the `expires_at` line in section 1:

- **`NEVER (expires_at is 0)`** — the token carries no expiry. This is the
  answer this page is waiting for.
- **`(absent — Meta returned no such field)`** — Meta did not say. Not the same
  thing, and not evidence of anything.
- **A timestamp** — it expires then, and the treadmill is unchanged.

Then read section 2 — your Page should be listed, with `tasks` including
`MODERATE` — and section 4, which debugs the exchanged Page token in its own
right.

**A token that authenticates is not a token that works.** The check that
actually matters is reading comments through the server with it, because the
Page-token exchange has to survive the new identity too. Do that before
concluding anything.

## The question underneath this

If System User tokens do not expire, there is still a decision to make that
this page cannot make for you. A System User is a *business* identity, not a
personal one. The install is designed so each person enters their own token and
reaches whatever their own Facebook identity reaches, and nobody holds anyone
else's credentials.

One System User token shared across a team removes the expiry and removes that
boundary with it. One System User per person keeps the boundary and removes the
expiry, but not the per-person setup.

Neither is wrong. Decide it deliberately rather than discovering it.
