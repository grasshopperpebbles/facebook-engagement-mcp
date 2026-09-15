# Conformance suite

One runner. Every implementation must pass it.

```bash
node conformance/run.mjs                    # every implementation
node conformance/run.mjs node               # just one
node conformance/run.mjs --case the-page-must-have-the-last-word
```

The Node implementation must be built first (`cd node && npm run build`) — the
suite drives the **built** artefact, never the source, for the reason in
*Against the thing people get* below.

## What this is for

Nothing copies code between languages. `node/scripts/extract.mjs` vendors the
TypeScript Graph client out of a private monorepo, which works only because both
ends are TypeScript; nothing can vendor into Python. So what holds several
implementations equivalent is not shared code — it is this suite.

Each case encodes a rule where **the obvious implementation is the wrong one**.
Every rule here is a bug this project actually shipped, found against live Graph,
usually after passing review. A port written from the README would reintroduce
them, and nothing else in the repository would notice.

## How it works

The runner launches an implementation as a subprocess, speaks MCP JSON-RPC over
its stdin and stdout, and serves it recorded Graph responses from an HTTP stub on
loopback. The implementation is pointed at the stub with `META_GRAPH_ORIGIN`,
which also moves the `paging.next` origin pin — one value, so a stubbed run
cannot follow `next` off to a third host.

**The runner knows nothing about any language.** `implementations.json` gives a
launch command; Node's is `node node/dist/index.js`, Python's will be
`docker run -i --rm …`. The runner talks to a pipe.

That indirection is the point. The alternative — a runner per language, written
in the porter's language by the porter — is a judge appointed by the defendant,
and it lets each implementation decide for itself what "equal" means.

## Against the thing people get

The suite drives the built artefact and, for languages that ship as a package, it
will drive the **installed package** rather than the source tree.

This project has already paid for that lesson once. The `bin` entrypoint guard
compared `import.meta.url` against `process.argv[1]` unresolved, which is false
under npm's bin symlink — so the server exited 0 having started nothing. Twelve
reviews read that line and approved it. The defect did not exist in the source;
it existed in the packaging. A container that does `COPY . /app` and runs from
source reproduces exactly the review that missed it.

## Every case has been watched failing

A case that has never failed is not yet a case — it might assert nothing. Each
one below was checked by breaking the implementation in the specific way its rule
describes and confirming the case objects.

| Case | Mutation | Caught |
|---|---|---|
| `threads-run-three-levels-deep` | stop following the third level | ✅ |
| `the-page-must-have-the-last-word` | `.some(isPage)` instead of the last word | ✅ |
| `the-page-must-have-the-last-word` | stop following the third level | ✅ |
| `replies-are-not-in-time-order` | assembler stops sorting | ✅ *(after sharpening — see below)* |
| `replies-are-not-in-time-order` | assembler stops sorting **and** triage reads `at(-1)` | ✅ |
| `no-author-anywhere-needs-reply` | `replies.length > 0 ? answered : needs_reply` | ✅ |
| `no-author-anywhere-needs-reply` | restore the overturned cause in the note | ✅ |

### The one that did not catch its own mutation at first

`replies-are-not-in-time-order` was written for T-36 and **passed** when
`lastWord` was mutated to read `replies.at(-1)`.

T-36 was fixed in two places — the assembler sorts what it builds, *and* the
triage answers from timestamps — so breaking either alone leaves the other
covering. Only breaking both failed the case, which happens to be exactly the
shape a port written from the README would have, so the case was never useless.
But a case that cannot disagree with a single-point regression is weaker than it
looks, and it looked fine.

The fix was to assert an additional property the assembler alone is responsible
for: the rendered replies come back in chronological order. That is not an
internal detail — the array is what a person reads, so a conversation out of
order is wrong on its own terms. With that assertion the case catches either half
alone.

**This is the suite's own rule turning on the suite.** *A check earns its place
by being able to disagree*, and the only way to find out whether it can is to
make it.

## Adding a case

1. Write the rule as one sentence, and the `why` as the incident it came from.
   A case whose reason is "seems sensible" belongs in a unit test, not here.
2. Break the implementation in the way the rule describes. **Watch the case
   fail**, and record the mutation in the table above.
3. Restore, and confirm it passes again.

Assertions available: `at` (exact value at a path), `threadStatuses` (the
product's actual answer, keyed by comment id), `ordered`, `matches` /
`notMatches` (regex over the whole response), and `requested` (a Graph path that
must have been fetched — which is how "it followed the third level" is told apart
from "the fixture happened to contain the right answer").

`notMatches` exists because of T-45: a claim can be wrong in a response that is
otherwise correct, and the wording is what a user reads.
