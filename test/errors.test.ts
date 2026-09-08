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
