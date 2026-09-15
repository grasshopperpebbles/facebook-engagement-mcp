#!/usr/bin/env node
/**
 * The conformance runner (T-44).
 *
 * Launches an implementation as a subprocess, speaks MCP to it over stdio,
 * serves it recorded Graph responses from an HTTP stub, and checks the answers
 * against cases that encode rules where **the obvious implementation is the
 * wrong one**. Every rule in `cases/` is a bug this project actually shipped.
 *
 *   node conformance/run.mjs                 # every implementation in implementations.json
 *   node conformance/run.mjs node            # one of them
 *   node conformance/run.mjs --case threads-are-three-levels-deep
 *
 * **Why the runner knows nothing about any language.** It reads a launch command
 * from `implementations.json` and talks to a pipe. Node is `node …/index.js`;
 * Python will be `docker run -i --rm …`. That indirection is what keeps one
 * definition of "equal" rather than one per porter — a runner written in the
 * porter's language, by the porter, is a judge appointed by the defendant.
 */
import { readFileSync, readdirSync, existsSync } from "node:fs"
import { join, resolve } from "node:path"
import { McpProcess } from "./mcp-client.mjs"
import { GraphStub } from "./stub.mjs"

const HERE = import.meta.dirname
const REPO = resolve(HERE, "..")
const CASES = join(HERE, "cases")

const args = process.argv.slice(2)
const caseFilter = args.includes("--case") ? args[args.indexOf("--case") + 1] : undefined
const wanted = args.filter((a) => !a.startsWith("--") && a !== caseFilter)

/** `fixtures/` is shared with every implementation, so cases reference it by name. */
function fixture(name) {
  return JSON.parse(readFileSync(join(REPO, "fixtures", `${name}.json`), "utf8"))
}

/** Resolve `{"$fixture": "feed"}` anywhere in a case's `graph` block. */
function hydrate(value) {
  if (Array.isArray(value)) return value.map(hydrate)
  if (value === null || typeof value !== "object") return value
  if (typeof value.$fixture === "string") return fixture(value.$fixture)
  return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, hydrate(v)]))
}

/** `a.b[0].c` over a parsed response. Returns undefined rather than throwing. */
function at(object, path) {
  return path
    .replace(/\[(\d+)\]/g, ".$1")
    .split(".")
    .filter(Boolean)
    .reduce((node, key) => (node === undefined || node === null ? undefined : node[key]), object)
}

/**
 * Every thread's status, keyed by comment id — the projection most cases assert
 * on, because it IS the product's answer.
 *
 * Deliberately a flat map rather than the nested group structure: grouping is a
 * presentation choice an implementation is allowed to differ on, and the triage
 * verdict is not.
 */
function threadStatuses(activity) {
  const statuses = {}
  for (const group of activity.groups ?? []) {
    for (const thread of group.threads ?? []) {
      statuses[thread.id ?? thread.comment?.id] = thread.status
    }
  }
  return statuses
}

function check(testCase, activity, stub) {
  const failures = []
  const expect = testCase.expect ?? {}
  const json = JSON.stringify(activity)

  if (expect.at) {
    for (const [path, want] of Object.entries(expect.at)) {
      const got = at(activity, path)
      if (JSON.stringify(got) !== JSON.stringify(want)) {
        failures.push(`${path}: expected ${JSON.stringify(want)}, got ${JSON.stringify(got)}`)
      }
    }
  }

  if (expect.threadStatuses) {
    const got = threadStatuses(activity)
    for (const [id, want] of Object.entries(expect.threadStatuses)) {
      if (got[id] !== want) {
        failures.push(`thread ${id}: expected status ${want}, got ${got[id] ?? "(absent)"}`)
      }
    }
  }

  // The reply array's own order is an observable property, not an internal
  // detail: it is what a person reads, so a conversation out of order is wrong
  // on its own terms. Asserting it matters because T-36's fix is belt and
  // braces — the assembler sorts AND the triage reads the clock — so a case that
  // only checks the verdict passes when either half alone is broken.
  for (const { path, field } of expect.ordered ?? []) {
    const items = at(activity, path)
    if (!Array.isArray(items)) {
      failures.push(`ordered: ${path} is not an array`)
      continue
    }
    const values = items.map((item) => item?.[field])
    const sorted = [...values].sort()
    if (JSON.stringify(values) !== JSON.stringify(sorted)) {
      failures.push(`${path} is not ordered by ${field}: ${JSON.stringify(values)}`)
    }
  }

  for (const pattern of expect.matches ?? []) {
    if (!new RegExp(pattern, "i").test(json)) failures.push(`response must match /${pattern}/i`)
  }
  for (const pattern of expect.notMatches ?? []) {
    if (new RegExp(pattern, "i").test(json)) failures.push(`response must NOT match /${pattern}/i`)
  }

  // Asserting what WAS requested is how "it fetched the third level" is
  // distinguished from "it happened to have the right answer in the fixture".
  for (const suffix of expect.requested ?? []) {
    if (!stub.requests.some((r) => r.path.endsWith(suffix))) {
      failures.push(`expected a request to *${suffix}; none was made`)
    }
  }

  // And asserting what was NOT requested is how a rung is held to being cheap.
  // Orientation exists to be the call made BEFORE the caller knows what they
  // need, so an implementation that sweeps a Page to answer it has the same
  // output and the wrong cost. The ad route needs it for a different reason: an
  // implementation that resolved the ad AND swept the Page would pass every
  // output assertion while having missed the entire point, which is that no
  // Page-level sweep reaches an unpublished post.
  for (const suffix of expect.notRequested ?? []) {
    const made = stub.requests.filter((r) => r.path.endsWith(suffix))
    if (made.length > 0) {
      failures.push(`expected NO request to *${suffix}; ${made.length} were made`)
    }
  }

  return failures
}

async function runCase(implementation, testCase) {
  const stub = new GraphStub(hydrate(testCase.graph ?? {}))
  const origin = await stub.listen(implementation.stubHost)
  const env = {
    ...implementation.env,
    META_ACCESS_TOKEN: "conformance-token",
    META_GRAPH_ORIGIN: origin,
    META_ALLOW_GRAPH_ORIGIN_OVERRIDE: "true",
    ...testCase.env,
  }

  // `{{ENV}}` expands to one `--env KEY=VALUE` per variable.
  //
  // **This exists because the container silently tested something else.** A
  // containerised entry inherits nothing from the parent process, so a fixed
  // list of `--env` flags in implementations.json forwarded exactly the three
  // variables somebody thought of and dropped every variable a CASE sets. The
  // symptom was the two Python entries disagreeing — native passing the write
  // cases, the container failing them, on identical code — which was luck: had
  // only the container been listed, the suite would have reported a real
  // implementation failing a rule it actually honours, or worse, passed a rule
  // it does not because the case's env never arrived.
  const cmd = implementation.cmd.flatMap((arg) =>
    arg === "{{ENV}}" ? Object.keys(env).flatMap((key) => ["--env", `${key}=${env[key]}`]) : [arg],
  )

  const mcp = new McpProcess(cmd, env, REPO)

  try {
    await mcp.initialize()

    // A refusal is an answer, and half of what this capability does is refuse
    // safely. An implementation may reject an invalid call at the protocol layer
    // (a required field in the tool schema) or inside the tool body with an
    // explanatory message — both are legitimate, and a case must be able to
    // assert the BEHAVIOUR (refused, nothing reached Graph, the right field
    // named) without pinning which layer said so. So a protocol error is
    // normalised into the same shape as a returned error rather than crashing
    // the case.
    let activity
    try {
      const result = await mcp.callTool(testCase.tool, testCase.arguments ?? {})
      const text = result?.content?.[0]?.text ?? ""
      try {
        activity = JSON.parse(text)
      } catch {
        activity = { error: text }
      }
    } catch (protocolError) {
      activity = { error: String(protocolError instanceof Error ? protocolError.message : protocolError) }
    }
    return check(testCase, activity, stub)
  } catch (error) {
    // An implementation that will not start, or dies mid-call, is a FAILING
    // CASE — not a crashed run. The first port found this the hard way: a
    // Python import error took the whole suite down with a Node stack trace,
    // so the one language whose result mattered reported nothing at all. A
    // harness that cannot survive the failure it exists to detect is no better
    // than a check that cannot fail.
    return [String(error instanceof Error ? error.message : error)]
  } finally {
    await mcp.close()
    await stub.close()
  }
}

const implementations = JSON.parse(readFileSync(join(HERE, "implementations.json"), "utf8"))
const cases = readdirSync(CASES)
  .filter((f) => f.endsWith(".json"))
  .map((f) => ({ file: f, ...JSON.parse(readFileSync(join(CASES, f), "utf8")) }))
  .filter((c) => caseFilter === undefined || c.name === caseFilter)

if (cases.length === 0) {
  console.error("No cases matched. A suite with nothing in it passes and means nothing.")
  process.exit(1)
}

let failed = 0
let ran = 0
for (const [name, implementation] of Object.entries(implementations)) {
  if (wanted.length > 0 && !wanted.includes(name)) continue

  // A missing sibling checkout is a skip, not a failure — but it is SAID, so a
  // green run that silently tested one implementation cannot be mistaken for a
  // green run that tested both.
  //
  // **Unless it was asked for by name.** CI names the implementations it means
  // to test, and an entry that vanishes for that run is a coverage hole, not a
  // convenience: the suite would go green having tested less than it was told
  // to, and nothing would say so where anyone looks. Naming it is the caller
  // asserting it must run.
  const probe = implementation.existsCheck && resolve(REPO, implementation.existsCheck)
  if (probe && !existsSync(probe)) {
    if (wanted.includes(name)) {
      console.error(
        `\n✗ ${name}: asked for by name but ${implementation.existsCheck} is not present. ` +
          "Refusing to skip an implementation that was explicitly requested.",
      )
      failed += 1
      continue
    }
    console.log(`\n— ${name}: SKIPPED, ${implementation.existsCheck} not present`)
    continue
  }

  console.log(`\n▸ ${name}  (${implementation.cmd.join(" ")})`)
  for (const testCase of cases) {
    const failures = await runCase(implementation, testCase)
    ran += 1
    if (failures.length === 0) {
      console.log(`  ✓ ${testCase.name}`)
    } else {
      failed += 1
      console.log(`  ✗ ${testCase.name}  — ${testCase.rule}`)
      for (const failure of failures) console.log(`      ${failure}`)
    }
  }
}

if (ran === 0) {
  console.error("\nNo implementation ran. Treat that as a failure, not a pass.")
  process.exit(1)
}
console.log(`\n${ran - failed}/${ran} passed`)
process.exit(failed === 0 ? 0 : 1)
