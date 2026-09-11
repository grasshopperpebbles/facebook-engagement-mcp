---
title: "Meta Told Me I Needed a Permission That Died in 2018"
date: 2026-09-08
verified: "Reproduced and resolved live on 2026-09-08. The same reply was sent twice to the same comment, one call apart: with a Page token it published (status 200); with a user token it returned the publish_actions message verbatim."
---

<!-- Source of truth for this article is the broadkast content repo,
     under grasshopperpebbles articles/facebook-engagement-mcp-setup/article-error-messages.md.
     This copy exists so the README can link to something that renders
     code correctly and does not depend on a site being live. -->


# Meta Told Me I Needed a Permission That Died in 2018

Here is the error that cost me a day:

```json
{
  "error": {
    "message": "(#200) The permission(s) publish_actions are not available. It has been deprecated. If you want to provide a way for your app users to share content to Facebook, we encourage you to use our Sharing products instead.",
    "type": "OAuthException",
    "code": 200,
    "fbtrace_id": "A46kpBOg-wNnhJ_Ba223e0W"
  }
}
```

I was publishing a reply to a Facebook comment. `publish_actions` was removed
from the Graph API in 2018. It is not the permission a comment reply needs, it is
not a permission you can request, and **no amount of App Review will grant it** —
there is nothing left to grant.

So the obvious reading is that Facebook is returning nonsense. That reading is
wrong, and it is wrong in the specific way that wastes the most time: the message
is *literally true*. It is describing a real permission check on a real code path.
It just is not describing the thing you did wrong.

**The thing I did wrong was use a user token where a Page token was required.**
That is the whole answer. It took a day to get to, and the route there is worth
more than the destination.

## The short version

```text
Symptom:   (#200) The permission(s) publish_actions are not available.
           It has been deprecated.
Reading:   "I need a permission that no longer exists" -> App Review -> dead end
Reality:   publish_actions was the permission for publishing AS A USER.
           A write carrying a USER token reaches that code path.
           The permission is gone, so Graph names it.
Fix:       Use a Page token. Nothing about permissions changes.
```

If you are here from a search engine mid-outage, that is your answer. Exchange
your user token for a Page token — `GET /me/accounts` returns one per Page — and
send the write with that. The rest of this article is how to *know* that rather
than guess it, because I guessed wrong three times first.

## Why the message is shaped like this

Facebook has two ways to publish something, and they were never the same API.

Publishing **as a person** — the app posts to a user's own timeline on their
behalf — required `publish_actions`. That permission was deprecated in April 2018
and removed. The whole category went away; the replacement is the Share dialog,
which is exactly what the error's second sentence is telling you to use.

Publishing **as a Page** — a Page replying to a comment on its own post —
requires `pages_manage_engagement` and a **Page access token**.

Those are different code paths, and *the token you send decides which one you are
on*. Send a user token to a comment-publish endpoint and you are asking to
publish as a person. Graph checks the permission that governs publishing as a
person, finds it does not exist any more, and says so. Accurately.

The error is not about your app. It is Graph telling you which of the two doors
you walked through, in the least helpful phrasing available.

### The same mistake, a different message — added 2026-09-11

Running the identical control with a **Business Manager System User** token
rather than a personal user token returns something else entirely:

```text
(#3) Publishing comments through the API is only available for page access tokens
```

Same wrong identity, same endpoint, same fix — and a message that names the
actual fault in one line. `publish_actions` is not mentioned, because a System
User has no personal timeline and never had the old permission to lose.

So **the misleading error is specific to personal user tokens**, and if you are
on a System User you will never see this article's symptom. That is worth
knowing in both directions: it means a search for `publish_actions` will not
find your problem, and it means the confusing case is the *common* one, since
most people start with an Explorer token.

It also sharpens the diagnosis this article argues for. Two identities, two
messages, one cause — which is exactly why "check identity before permission"
beats reading the message. A `(#200)` and a `(#3)` here mean the same thing.

## The wrong conclusion, and how I reached it

My first conclusion was that this needed App Review. I want to be precise about
how that happened, because the mechanism is more interesting than the mistake.

I did not reason from Facebook's documentation. I reasoned from **my own error
message**. My server caught the refusal and wrapped it in a helpful explanation
that said, in effect, *this operation requires `pages_manage_engagement`, which
requires App Review for Pages outside your development-mode app*. That sentence
was written months earlier, when it was a reasonable thing to say about a generic
permission error. It was now being printed underneath a completely different
failure.

So I read my own guess, found it plausible — of course I did, I wrote it — and
started planning an App Review submission for a permission I already had.

**The tell I ignored:** we were storing the error's `message` and throwing away
everything else. No `error_subcode`. No `error_user_title` or `error_user_msg`.
No `fbtrace_id`. Those are the fields that distinguish one `#200` from another,
and without them the only evidence I held could not tell a scope problem from a
role problem from an identity problem. Evidence that thin can only be re-read,
and re-reading it just reproduces the first guess with more confidence.

If you take one habit from this article, take this one:

> **Log the whole error object, not the message.** Graph's `message` is written
> for a human skimming a dashboard. The subcode is written for you.

## The four checks that isolate it in one pass

The fix for guessing is not guessing harder. It is instrumenting every boundary
between your token and your write, in order, so the failing one is visible
instead of inferred.

There are only four things between a token and a published reply. Check them all
at once — they cost four requests and about a minute.

### 1. What is this token, actually?

```bash
curl -s -G "https://graph.facebook.com/v25.0/debug_token" \
  --data-urlencode "input_token=$TOKEN_UNDER_TEST" \
  -H "Authorization: Bearer $MY_USER_TOKEN"
```

The field that matters is `type`. It says `USER` or `PAGE`. Everything in this
article hangs on that one word, and it is the field nobody checks, because you
believe you know which token you are holding.

I did not. My own written notes from earlier that day said the failure had been
reproduced "in the Graph API Explorer with a Page token". It cannot have been a
Page token — a Page token publishes the reply. The single detail nobody verified
was the one that mattered.

Also check `expires_at`. A token from the Graph API Explorer lasts about an hour.

### 2. Is the permission granted *for this Page*?

This is the check I expected to be the answer, and it is worth running even
though it was not.

`debug_token` returns `granular_scopes`, and this is the part people miss:

```json
"granular_scopes": [
  { "scope": "pages_manage_engagement", "target_ids": ["1098414686687306"] },
  { "scope": "pages_read_engagement" }
]
```

Meta grants Page permissions **against a list of Page ids**. A token can carry
`pages_manage_engagement` while the Page you are writing to is absent from that
permission's `target_ids`. Reads keep working, because reads go through a Page
token you exchanged earlier. Only the write fails — which produces exactly the
"but I *have* that permission" confusion you are in.

An entry with **no `target_ids` key at all** means all Pages. That is what mine
showed, which killed the hypothesis immediately:

```text
pages_manage_engagement      → ALL (no target_ids key)
```

A related trap, and one I had already been bitten by: **a Page grant goes stale**.
"Opt in to all current and future Pages" describes the option you clicked, not
the grant you received. A Page created *after* you authorised is not in it. I
once had a token returning 5 Pages when the account had 7. The cure is to
uninstall the app under **Apps and Websites** in the Graph API Explorer and
re-authorise.

### 3. Do you hold the right role on the Page?

```bash
curl -s -G "https://graph.facebook.com/v25.0/me/accounts" \
  --data-urlencode "fields=id,name,tasks" \
  -H "Authorization: Bearer $MY_USER_TOKEN"
```

You need `MODERATE` in `tasks` to reply to or hide a comment. Its absence
produces its own misleading permission error, covered in the companion setup
article. Mine was present.

### 4. Ask Graph whether the write is allowed, before attempting it

This one is underused and it is the cheapest of the four.

```bash
curl -s -G "https://graph.facebook.com/v25.0/$COMMENT_ID" \
  --data-urlencode "fields=id,from,can_comment,can_hide,is_hidden" \
  -H "Authorization: Bearer $PAGE_TOKEN"
```

```json
{
  "id": "122251672790055926_4484328161782909",
  "from": { "name": "Les Green", "id": "28452515681102242" },
  "can_comment": true,
  "can_hide": true,
  "is_hidden": false
}
```

`can_comment` is Graph's own answer to *may this token reply here*. Getting it as
a field beats inferring it from a refusal. If it is `false`, stop — you have your
answer without a failed write in your logs and without a public reply you did not
intend.

Mine said `true`, on the exact comment that had been refusing me all day. That is
the moment the permission theory should have died, and would have, had I asked the
question in that order.

## The experiment that settled it

Four checks all clean, and the write still refusing. At that point you are not
looking for a broken permission any more — you are looking for **the difference
between the call that works and the call that does not**.

I had two cases and had never held them side by side:

- Every refusal had been against a comment on a **dark post** (an unpublished
  post, which is what most ads run on).
- The reply that succeeded had been to a comment on an **ordinary published
  post**.

So the obvious suspect was the post type. I built the test: create a dark post,
comment on it as the Page, reply to that comment, delete everything. It
published. `status 200`. The dark post was never the variable.

That left the token. And the token is trivially testable, because you can send
**the same reply twice, to the same comment, one call apart, changing nothing but
the credential**:

```text
POST /{comment-id}/comments   with the PAGE token   ->  200  {"id": "..."}
POST /{comment-id}/comments   with the USER token   ->  403  publish_actions
```

That second response was the message from the top of this article, verbatim, down
to the sentence about Sharing products. One variable. One call apart. Nothing
left to interpret.

**This is the shape of experiment worth reaching for earlier than feels
necessary.** Not "what is wrong with my permissions", which invites reading. Just
"what is different between the working call and the broken one" — then change one
thing at a time until the difference moves.

## The bug it was hiding

I would like to report that this was a bad manual test and my code was fine. It
was not.

My server took the Page id as an **optional** parameter on both write tools. When
it was supplied, the server exchanged it for a Page token and wrote as the Page.
When it was absent, it fell through to the client built from the startup token —
**the user token**. And the parameter's own description read:

> "Page that owns the comment. Supplying it gives clearer permission errors."

That is exactly backwards. It does not improve the error; it is the difference
between the call working and the call being impossible. Anything calling my tool
with only a comment id — which the schema explicitly permitted — got the
`publish_actions` refusal, forever, with no way to discover why.

So the misleading vendor message was sitting on top of a real defect of mine, and
each one hid the other. The fix is boring: the Page id is required now, checked
before the dry run so a dry run cannot claim a call would succeed when it could
not, and the error handler recognises the `publish_actions` text and says *you
sent a user token* rather than *ask Facebook for permission*.

There is a general point here, and it is the one I keep relearning:

> **A misleading error from someone else's system is a good place for a bug of
> your own to hide.** You spend your attention arguing with the vendor, which is
> attention not spent on the twenty lines where you chose the credential.

## What I would do differently

Three things, in the order they would have saved the most time.

**Keep the whole error body.** `message` alone is not evidence. Store `code`,
`error_subcode`, `type`, `error_user_title`, `error_user_msg` and `fbtrace_id`,
and put them somewhere you will read them. `#200` is a generic permission code
that Facebook returns for many distinct failures; the subcode and the user-facing
fields are what separate them, and discarding those leaves you with a number that
means "something about permissions" and nothing more.

**Do not let your own explanation outrank the vendor's text.** My server replaced
Facebook's message with a friendlier one and, in doing so, destroyed the only
clue. It now shows both, always, and says which is which. A wrapper that
paraphrases an error you do not fully understand is a wrapper that launders a
guess into a fact.

**Check the identity before the permission.** "Which token is this?" is one
request and rules out an entire class of failure. "Which permission is missing?"
is a research project. I had them in the wrong order, and the wrong order is the
default order for everyone, because permissions are what the error talks about.

And one meta-lesson, which is the third time this project has produced it: **no
amount of reading falsifies a claim about someone else's system.** A written note
saying "reproduced with a Page token" survived review because it was written
down, and it was wrong. Reading it again — however carefully, however many times
— could never have found that. Running one command did.

## Companion articles

This is the third piece in a set, and it assumes the first:

- [Reading Facebook Page Comments with the Graph API: App Setup, Tokens, and
  Permissions](./setup-tokens-permissions.md) — app creation, the three-token chain, `MODERATE`, and why a
  comment read with a user token returns an empty array instead of an error. If
  you are setting this up from scratch, start there.
- [Facebook's `/feed` Does Not Return Unpublished Posts — So Your Ads' Comments
  Are Invisible](./dark-posts.md) — the finding that ad comments are unreachable by any
  Page-level sweep, and the ad-resolution route that does reach them.

The read-side twin of this article's bug lives in the first one, and it is worth
knowing about because it fails *silently* rather than loudly: **a comment read
with a user token returns an empty array, not an error.** Same root cause — wrong
identity — with none of the noise. At least `publish_actions` shouted.
