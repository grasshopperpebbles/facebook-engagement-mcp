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

  it("says the token was withheld when Graph listed the Page but gave no token", async () => {
    // The other half of "inaccessible". Graph naming the Page and withholding
    // its token is a different fault from Graph never naming it, and the old
    // message reported the second for both — sending a reader to check
    // administration and `pages_show_list` for a case that may be neither.
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(accounts([{ id: "pg1" }, { id: "pg2", access_token: "t2" }]))
    const provider = createEnvTokenProvider({ userToken: "user-token", fetchImpl })

    await provider.forPage("pg1").catch((error: unknown) => {
      const message = (error as Error).message
      expect(message).toContain("pg1")
      expect(message).toContain("no access token")
      expect(message).not.toContain("pages_show_list")
      expect(message).not.toContain("t2")
    })
    await expect(provider.forPage("pg1")).rejects.toBeInstanceOf(PageAccessError)
  })

  it("still blames reach when Graph never listed the Page at all", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(accounts([{ id: "pg1", access_token: "t1" }]))
    const provider = createEnvTokenProvider({ userToken: "user-token", fetchImpl })

    await provider.forPage("pg9").catch((error: unknown) => {
      expect((error as Error).message).toContain("pages_show_list")
    })
  })

  it("reports tasks as unknown when Graph did not return the field", async () => {
    // Not `[]`. An empty array is "this identity holds no role", which the
    // MODERATE guard refuses writes on; an absent field is "Graph did not say",
    // which it must not refuse on. Conflating them refused every write naming a
    // role the identity may well hold — the `publish_actions` shape, a message
    // literally true and entirely misleading.
    const fetchImpl = vi.fn().mockResolvedValue(accounts([{ id: "pg1", access_token: "t1" }]))
    const provider = createEnvTokenProvider({ userToken: "user-token", fetchImpl })

    expect(await provider.tasksFor("pg1")).toBeUndefined()
  })

  it("reports an empty tasks array as itself, not as unknown", async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(accounts([{ id: "pg1", access_token: "t1", tasks: [] }]))
    const provider = createEnvTokenProvider({ userToken: "user-token", fetchImpl })

    expect(await provider.tasksFor("pg1")).toEqual([])
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
    first?.push("MODERATE")

    expect(await provider.tasksFor("pg1")).toEqual(["ANALYZE"])
  })

  it("never exposes the whole credential map", () => {
    const provider = createEnvTokenProvider({ userToken: "user-token" })

    expect(Object.keys(provider).sort()).toEqual(["forPage", "forUser", "tasksFor"])
  })
})
