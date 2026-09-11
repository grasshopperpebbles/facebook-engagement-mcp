---
title: "The System User token, and the 60-day expiry"
description: "A Business Manager System User is the usual answer to re-minting a token every 60 days. What is verified here, what is not, and how to settle it yourself."
date: 2026-09-11
---

# The System User token, and the 60-day expiry

> **Status: verified end to end, 2026-09-11.** A System User token was issued,
> checked with `debug_token`, and driven through this server against a live
> Page. `expires_at` came back **0 — no expiry**, on the System User token *and*
> on the Page token exchanged from it. Comment **reads** ran through the server;
> the **write** path published a reply and deleted it, on an unpublished post so
> nothing was ever public. The 60-day exchange is not needed on this path. Every
> claim below was watched rather than read.

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
the Graph API Explorer, and **it does not expire** — confirmed here rather than
taken from the documentation.

It is also a tighter credential than the one it replaces. A personal token
reaches every Page you administer; on the account this was tested against that
was seven Pages, six of them irrelevant to this tool. The System User reaches
only the Pages assigned to it — one.

## The trap that costs the most time

**The app must be assigned to the System User as an asset, and that is not the
same act as giving the System User a role on the app.** Doing it from the app's
side — *Accounts → Apps → your app → Assign people* — leaves **Generate token**
refusing with:

> No permissions available. Assign an app role to the system user or select
> another app to continue.

The assignment that clears it is from the System User's side: **Users → System
users → your user → Assign assets → Apps**. The *Installed apps* tab looks like
where an app is added and is not — it stays empty until a token exists, which is
circular and reads like the thing you cannot do.

**It is not instant.** The app appeared under *Assigned assets* minutes after
the app-side role was granted, and the business-assets count went from 2 to 3
without anything else being clicked. If the dialog still refuses, reload Business
settings before concluding anything.

## Setting up the Page

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

**2. Assign it the Page *and* the app.** On the System User, **Assign assets** →
**Pages** → your Page → **full control**, then again with **Apps** → your app →
**full control**. Both are needed and they are separate acts — see *The trap*
above, which is where the time goes.

This step is the one that decides everything. **A System User inside a
portfolio that owns the Page does not thereby reach the Page** — the asset
assignment is a separate act, and skipping it produces a failure that reads
like a permissions problem and is not one. See *When it goes wrong* below.

**3. Generate the token.** On the System User, **Generate new token** → select
your Meta app → **Set expiration: Never** → tick the permissions this server
needs:

| Permission | What it gets you |
|---|---|
| `pages_show_list` | Which Pages the identity manages — **without this nothing works**, because every Page token comes from `/me/accounts` |
| `pages_read_user_content` | Reading the comments |
| `pages_manage_engagement` | Replying and hiding. Skip it and reads work perfectly while every write is refused |
| `ads_read` | Comments on your ads — see [why](dark-posts.md) |

**Check the list after you generate, not while you tick it.** The picker is a
scrolling multi-select and a missed tick is silent — the first token issued
during this verification came back without `pages_manage_engagement` and looked
completely healthy. `debug_token` is where you find out; see *Checking it
yourself* below.

Meta may also hand you `pages_read_engagement` and `pages_manage_posts`
alongside these, bundled with the app's use case. Neither is needed here and
neither does any harm.

**4. Paste it into the install form's Meta access token field**, in place of the
60-day one. The server does not care which kind of token it holds: it sends
whatever it is given to `/me/accounts` and takes the per-Page tokens from the
reply. Nothing in the server needs reconfiguring for a System User.

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

## What a good result looks like

Observed on 2026-09-11, with the values that matter:

```
type          SYSTEM_USER
expires_at    NEVER (expires_at is 0)
data_access   NEVER (expires_at is 0)
scopes        pages_show_list, ads_read, pages_read_engagement,
              pages_read_user_content, pages_manage_posts,
              pages_manage_engagement, public_profile

Pages this token reaches (1):
  → <page-id>   <Page name>   tasks: MANAGE, CREATE_CONTENT, MODERATE, ...

MODERATE on <page-id>: yes

Page token for <page-id>:
  type          PAGE
  expires_at    NEVER (expires_at is 0)

pages_manage_engagement:
  user token: granted with no target_ids (reads as all Pages).
  page token: granted with no target_ids (reads as all Pages).
```

Four things in that output, each of which has been a real failure at some point:

- **Both tokens say NEVER.** The exchanged Page token inherits the non-expiry.
  That is the half that was not obvious and the half that makes this worth
  doing — a non-expiring user token buys nothing if the exchange returns a
  60-day Page token.
- **One Page, not all of them.** A System User reaches only its assigned assets.
  The personal token it replaced reached seven Pages on the same account.
- **`MODERATE` present.** Without it, replies and hides are refused whatever the
  scopes say.
- **`no target_ids`.** Meta can grant a permission against a *list of Page ids*.
  A token can carry `pages_manage_engagement` while your Page is absent from
  that permission's targets — reads still succeed, because they go through the
  Page-token exchange, and only writes fail.

**Reads and writes both verified on this identity, 2026-09-11.** Reads ran
through the server end to end. The write path was then proved with
`write-probe.mjs dark-reply`, which creates an *unpublished* post, comments on
it as the Page, replies to that comment, and deletes everything — nothing public
at any point. The reply published, `status 200`.

**The control in that run turned up something worth knowing.** Sending the same
write with the System User token directly, rather than the Page token exchanged
from it, is refused — correctly — with:

```text
(#3) Publishing comments through the API is only available for page access tokens
```

A *personal* user token in the same position returns the notorious
`(#200) The permission(s) publish_actions are not available. It has been
deprecated.` instead — a dead scope, named misleadingly, which has its own
[article](publish-actions-error.md). **A System User never produces that
message**, because it has no personal timeline and never held the old
permission. Same fault, same fix, clearer error.

## Checking it yourself

From a clone of the monorepo:

```bash
META_ACCESS_TOKEN=<your system user token> \
  node servers/page-engagement/scripts/write-probe.mjs inspect <your-page-id>
```

Read the `expires_at` line in section 1:

- **`NEVER (expires_at is 0)`** — the token carries no expiry. This is what a
  System User token should say.
- **`(absent — Meta returned no such field)`** — Meta did not say. Not the same
  thing, and not evidence of anything.
- **A timestamp** — it expires then. If this is a System User token, check you
  chose **Never** rather than 60 days at the expiration step.

Then read section 2 — your Page should be listed, with `tasks` including
`MODERATE` — and section 4, which debugs the exchanged Page token in its own
right.

**A token that authenticates is not a token that works.** The check that
actually matters is reading comments through the server with it, because the
Page-token exchange has to survive the new identity too. Do that before
concluding anything — it is how the verification above was finished, and it is
where a subtly wrong setup shows up.

## Rolling this out to a team: one System User per person

**Make a System User for each person, not one for everybody.** It is more setup
and it is the right default, for three reasons that only show up later.

- **You can cut off one person.** Someone leaves, or a laptop goes missing, and
  you delete their System User. Everyone else keeps working. With a shared
  token you re-issue it and then chase every colleague to paste the new one —
  during which nobody's tool works.
- **You can see who did what.** Replies and hides are attributed to the
  identity that made them. One shared token makes every action look like the
  same actor, and there is no reconstructing it afterwards.
- **You can give different people different reach.** A System User sees only the
  Pages assigned to it. Someone who handles one brand does not need a token that
  reaches all of them — and with a shared token, everyone has everyone's access
  by construction.

The cost is real and worth stating: the per-person setup is **longer** than the
60-day path it replaces, because each person needs the Page *and* the app
assigned to their System User before a token will generate. You do that once per
person, and then never again — against the old path's every-60-days, per person,
forever.

**Name them after the person, not the tool.** `grasshopperAdmin` tells you
nothing six months on when you need to know whose token to revoke.

**Nothing in the server changes.** Each person pastes their own token into the
same install form field; the server sends whatever it is given to `/me/accounts`
and works from what comes back.

### If you do share one anyway

There is a case for it — one operator, or a small team where everyone has the
same access anyway and the admin overhead is the real cost. If you go that way,
know what you are trading: no attribution, no individual revocation, and a
rotation that breaks everyone at once. Write down who holds it.
