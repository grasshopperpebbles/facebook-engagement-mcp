import { describe, expect, it, vi } from "vitest"
import { createEnvTokenProvider, PageAccessError } from "../src/auth/token-provider.js"

const accounts = (data: unknown[]) => new Response(JSON.stringify({ data }), { status: 200 })

describe("EnvTokenProvider", () => {
  it("returns the user token unchanged", async () => {
    const provider = createEnvTokenProvider({ userToken: "user-token" })

    expect(await provider.forUser()).toBe("user-token")
  })

  it("exchanges the user token for a Page token", async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(accounts([{ id: "pg1", access_token: "page-token", tasks: ["MODERATE"] }]))
    const provider = createEnvTokenProvider({ userToken: "user-token", fetchImpl })

    expect(await provider.forPage("pg1")).toBe("page-token")
  })

  it("caches the exchange — a second Page does not refetch", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(
      accounts([
        { id: "pg1", access_token: "t1", tasks: [] },
        { id: "pg2", access_token: "t2", tasks: [] },
      ]),
    )
    const provider = createEnvTokenProvider({ userToken: "user-token", fetchImpl })

    await provider.forPage("pg1")
    await provider.forPage("pg2")

    expect(fetchImpl).toHaveBeenCalledTimes(1)
  })

  it("makes one request when concurrent callers race", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(accounts([{ id: "pg1", access_token: "t1" }]))
    const provider = createEnvTokenProvider({ userToken: "user-token", fetchImpl })

    await Promise.all([provider.forPage("pg1"), provider.forPage("pg1")])

    expect(fetchImpl).toHaveBeenCalledTimes(1)
  })

  it("reports an inaccessible Page without leaking any token", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(accounts([{ id: "pg1", access_token: "t1" }]))
    const provider = createEnvTokenProvider({ userToken: "user-token", fetchImpl })

    await expect(provider.forPage("pg9")).rejects.toBeInstanceOf(PageAccessError)
    await provider.forPage("pg9").catch((error: unknown) => {
      expect((error as Error).message).not.toContain("t1")
      expect((error as Error).message).not.toContain("user-token")
      expect((error as Error).message).toContain("pg9")
    })
  })

  it("exposes the tasks a Page token carries", async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(
        accounts([{ id: "pg1", access_token: "t1", tasks: ["MODERATE", "ANALYZE"] }]),
      )
    const provider = createEnvTokenProvider({ userToken: "user-token", fetchImpl })

    expect(await provider.tasksFor("pg1")).toEqual(["MODERATE", "ANALYZE"])
  })

  it("returns a copy of tasks, so a caller cannot grant itself a role", async () => {
    // The cached credential is shared across every call. Handing out the live
    // array would let a caller push "MODERATE" and have the next permission
    // check believe it.
    const fetchImpl = vi
      .fn()
      .mockImplementation(() => accounts([{ id: "pg1", access_token: "t1", tasks: ["ANALYZE"] }]))
    const provider = createEnvTokenProvider({ userToken: "user-token", fetchImpl })

    const first = await provider.tasksFor("pg1")
    first.push("MODERATE")

    expect(await provider.tasksFor("pg1")).toEqual(["ANALYZE"])
  })

  it("never exposes the whole credential map", () => {
    const provider = createEnvTokenProvider({ userToken: "user-token" })

    expect(Object.keys(provider).sort()).toEqual(["forPage", "forUser", "tasksFor"])
  })
})
