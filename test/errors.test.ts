import { describe, expect, it } from "vitest"
import { explainGraphError } from "../src/outcomes/errors.js"
import { MetaApiError } from "../src/vendor/meta-client/index.js"

describe("expired token", () => {
  const expired = () =>
    new MetaApiError({
      message: "Error validating access token: Session has expired on Tuesday, 08-Sep-26.",
      status: 401,
      code: 190,
      subcode: 463,
    })

  // Observed 2026-09-08: the old wording, "re-authenticate and retry", sent a
  // model hunting for an OAuth reconnect flow this server does not have. There
  // is no OAuth — a person pastes a token into the client's configuration — so
  // the message has to name that, or the operator is told to do something
  // impossible.
  it("says the token expired, in those words", () => {
    expect(explainGraphError(expired(), { operation: "read" })).toMatch(/expired/i)
  })

  it("names where the token is set rather than telling the operator to re-authenticate", () => {
    const message = explainGraphError(expired(), { operation: "read" })

    expect(message).toMatch(/META_ACCESS_TOKEN/)
    expect(message).not.toMatch(/re-authenticate/i)
  })

  it("says how long a replacement lasts, because that is the usual mistake", () => {
    expect(explainGraphError(expired(), { operation: "read" })).toMatch(/hour|60 day/i)
  })
})

describe("permission refusals keep Meta's own words", () => {
  // Observed 2026-09-08: replying to a comment was refused with "(#200) The
  // permission(s) publish_actions are not available. It has been deprecated."
  // publish_actions died in 2018, so the message is misleading — but it is also
  // the only clue, and this server was discarding it in favour of its own
  // explanation. A caller then debugged the wrong thing.
  //
  // CORRECTION, same day, established live: this assertion originally required
  // the explanation to name `pages_manage_engagement`, because that was assumed
  // to be the permission really at fault. It is not. `publish_actions` was the
  // permission for publishing AS A USER; a write carrying a user token instead
  // of a Page token reaches a code path whose permission no longer exists, and
  // Graph names it. The refusal is about identity, and reproducing it took one
  // call with the user token (T-26). Sending the reader to App Review for
  // `pages_manage_engagement` was the wrong instruction, so the assertion that
  // demanded it is wrong too.
  it("names the identity problem rather than a permission that cannot be granted", () => {
    const refused = new MetaApiError({
      message:
        "(#200) The permission(s) publish_actions are not available. It has been deprecated.",
      status: 403,
      code: 200,
    })

    const explained = explainGraphError(refused, { operation: "reply" })

    expect(explained).toMatch(/user token/i)
    expect(explained).toMatch(/pageId/)
    // Meta's own words survive, which was the point of the original test.
    expect(explained).toMatch(/publish_actions/)
    // And it must not tell anyone to seek review for a permission removed in 2018.
    expect(explained).not.toMatch(/requires App Review/i)
  })

  it("still explains an ordinary permission refusal in terms of the scope it needs", () => {
    const refused = new MetaApiError({
      message: "(#200) Permissions error",
      status: 403,
      code: 200,
    })

    expect(explainGraphError(refused, { operation: "reply" })).toMatch(/pages_manage_engagement/)
  })
})
