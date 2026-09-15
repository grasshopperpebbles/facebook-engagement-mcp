/**
 * A Graph API stub, served over real HTTP on loopback.
 *
 * **Why HTTP rather than a mocked fetch.** A mock reaches inside one
 * implementation's code; an HTTP server is reachable by every language without
 * knowing anything about any of them, which is the entire argument for driving
 * conformance over a process boundary. It also means the implementation under
 * test exercises its own transport — URL building, headers, retries, pagination,
 * error mapping — rather than having that layer stubbed out. Every one of those
 * has carried a real defect here.
 *
 * Routes are matched by PATH SUFFIX, deliberately. The API version segment is
 * part of the request path (`/v25.0/pg1/feed`) and each implementation pins its
 * own version; a case that hard-coded `v25.0` would fail a correct
 * implementation that pinned something else, which is a conformance suite
 * testing the wrong thing.
 */
import { createServer } from "node:http"

/** Recorded requests, so a case can assert what was NOT asked for. */
export class GraphStub {
  /** @param {Record<string, unknown>} routes suffix -> JSON body, or {status, body} */
  constructor(routes) {
    this.routes = routes
    /** @type {{method: string, path: string, search: string}[]} */
    this.requests = []
    this.server = createServer((req, res) => this.#handle(req, res))
  }

  /**
   * @param {string} host how the implementation under test will address this
   *   stub. Loopback for a local process; `host.docker.internal` (or a compose
   *   service name) for a container, where `127.0.0.1` is the CONTAINER's own
   *   loopback and not the host's.
   *
   * Binding follows from that: a container cannot reach a socket bound only to
   * the host's loopback, so a non-loopback host implies binding on all
   * interfaces. This is the concrete reason the Graph origin override is not
   * loopback-restricted — a rule requiring loopback would pass every local run
   * and fail every containerised one.
   */
  async listen(host = "127.0.0.1") {
    const bind = host === "127.0.0.1" ? "127.0.0.1" : "0.0.0.0"
    await new Promise((resolve) => this.server.listen(0, bind, resolve))
    const { port } = /** @type {{port: number}} */ (this.server.address())
    return `http://${host}:${port}`
  }

  async close() {
    await new Promise((resolve) => this.server.close(resolve))
  }

  /** The longest matching suffix wins, so `/a/b/comments` beats `/comments`. */
  #match(pathname) {
    let best
    for (const suffix of Object.keys(this.routes)) {
      if (!pathname.endsWith(suffix)) continue
      if (best === undefined || suffix.length > best.length) best = suffix
    }
    return best
  }

  #handle(req, res) {
    const url = new URL(req.url ?? "/", "http://stub")
    this.requests.push({
      method: req.method ?? "GET",
      path: url.pathname,
      search: url.search,
    })

    const suffix = this.#match(url.pathname)
    if (suffix === undefined) {
      // Graph's own shape for "no such node", which is what an implementation
      // should be handling — not a bare 404 with an HTML body.
      res.writeHead(400, { "content-type": "application/json" })
      res.end(
        JSON.stringify({
          error: { message: `Unknown path ${url.pathname}`, type: "GraphMethodException", code: 100 },
        }),
      )
      return
    }

    const route = this.routes[suffix]
    const { status = 200, body = route } =
      route !== null && typeof route === "object" && "status" in route ? route : {}

    res.writeHead(status, { "content-type": "application/json" })
    res.end(JSON.stringify(status === 200 ? (body ?? route) : body))
  }
}
