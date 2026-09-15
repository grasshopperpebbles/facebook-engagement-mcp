import { describe, expect, it, vi } from "vitest"
import { createLogger, timed } from "../src/logging/logger.js"

describe("createLogger", () => {
  it("writes one JSON object per line", () => {
    const sink = vi.fn()
    createLogger({ sink }).log({ operation: "read", ok: true, pageId: "pg1" })

    expect(JSON.parse(sink.mock.calls[0]![0] as string)).toMatchObject({
      operation: "read",
      ok: true,
      pageId: "pg1",
    })
  })

  it("omits absent fields rather than emitting nulls", () => {
    const sink = vi.fn()
    createLogger({ sink }).log({ operation: "read", ok: true })

    expect(sink.mock.calls[0]![0] as string).not.toContain("pageId")
  })

  it("drops any field that is not on the allowed list", () => {
    // An allow-list, not a deny-list: a future caller passing a comment body
    // under a new key must not be able to leak it by accident.
    const sink = vi.fn()
    createLogger({ sink }).log({
      operation: "read",
      ok: true,
      message: "Do you ship to Ireland?",
    } as never)

    expect(sink.mock.calls[0]![0] as string).not.toContain("Ireland")
  })

  it("defaults to stderr, never stdout", () => {
    // stdout is the MCP stdio channel. A log line written there corrupts the
    // protocol stream and the client disconnects.
    const stderr = vi.spyOn(process.stderr, "write").mockReturnValue(true)
    const stdout = vi.spyOn(process.stdout, "write").mockReturnValue(true)

    createLogger().log({ operation: "read", ok: true })

    expect(stderr).toHaveBeenCalled()
    expect(stdout).not.toHaveBeenCalled()
    stderr.mockRestore()
    stdout.mockRestore()
  })
})

describe("timed", () => {
  it("records a duration and success", async () => {
    const sink = vi.fn()
    const logger = createLogger({ sink })

    const result = await timed(logger, { operation: "read", pageId: "pg1" }, async () => 42)

    expect(result).toBe(42)
    const entry = JSON.parse(sink.mock.calls[0]![0] as string) as Record<string, unknown>
    expect(entry.ok).toBe(true)
    expect(typeof entry.durationMs).toBe("number")
  })

  it("logs a failure and rethrows", async () => {
    const sink = vi.fn()
    const logger = createLogger({ sink })

    await expect(
      timed(logger, { operation: "reply" }, async () => {
        throw new Error("boom")
      }),
    ).rejects.toThrow("boom")

    expect(JSON.parse(sink.mock.calls[0]![0] as string).ok).toBe(false)
  })

  it("never logs the thrown error's message, which may quote content", async () => {
    const sink = vi.fn()
    const logger = createLogger({ sink })

    await timed(logger, { operation: "reply" }, async () => {
      throw new Error("failed on comment: Do you ship to Ireland?")
    }).catch(() => undefined)

    expect(sink.mock.calls[0]![0] as string).not.toContain("Ireland")
  })
})
