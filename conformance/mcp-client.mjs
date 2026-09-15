/**
 * The smallest MCP client that can call a tool, speaking JSON-RPC over a child
 * process's stdin and stdout.
 *
 * **Hand-written rather than the SDK on purpose.** The suite's job is to check
 * that each implementation speaks the protocol a host actually speaks. Driving
 * it with the same TypeScript SDK the TypeScript implementation is built on
 * would make half of that check circular — the SDK would paper over a framing
 * or handshake difference in exactly the implementation least likely to have
 * one, and fail to in the ports. About eighty lines buys independence.
 */
import { spawn } from "node:child_process"

const PROTOCOL_VERSION = "2025-06-18"

export class McpProcess {
  /**
   * @param {string[]} cmd argv; `cmd[0]` is the program
   * @param {Record<string,string>} env
   * @param {string} cwd
   */
  constructor(cmd, env, cwd) {
    this.child = spawn(cmd[0], cmd.slice(1), {
      cwd,
      env: { ...process.env, ...env },
      stdio: ["pipe", "pipe", "pipe"],
    })
    this.nextId = 1
    this.pending = new Map()
    this.buffer = ""
    /** Kept so a failure can show what the server said rather than "it hung". */
    this.stderr = ""

    this.child.stdout.setEncoding("utf8")
    this.child.stdout.on("data", (chunk) => this.#onData(chunk))
    this.child.stderr.setEncoding("utf8")
    this.child.stderr.on("data", (chunk) => {
      this.stderr += chunk
    })
    this.child.on("exit", (code) => {
      for (const { reject } of this.pending.values()) {
        reject(new Error(`server exited (${code}) before answering.\nstderr:\n${this.stderr}`))
      }
      this.pending.clear()
    })
  }

  // Newline-delimited JSON, which is what the stdio transport uses. A message
  // is never split across lines, so anything that does not parse is the server
  // printing to the wrong stream — a real and easy mistake, and one worth
  // reporting precisely rather than hanging on.
  #onData(chunk) {
    this.buffer += chunk
    let index = this.buffer.indexOf("\n")
    while (index !== -1) {
      const line = this.buffer.slice(0, index).trim()
      this.buffer = this.buffer.slice(index + 1)
      if (line) this.#onMessage(line)
      index = this.buffer.indexOf("\n")
    }
  }

  #onMessage(line) {
    let message
    try {
      message = JSON.parse(line)
    } catch {
      throw new Error(`Non-JSON on stdout, which must carry only protocol traffic: ${line}`)
    }
    const waiting = this.pending.get(message.id)
    if (!waiting) return
    this.pending.delete(message.id)
    if (message.error) waiting.reject(new Error(JSON.stringify(message.error)))
    else waiting.resolve(message.result)
  }

  #send(method, params) {
    const id = this.nextId++
    this.child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", id, method, params })}\n`)
    return new Promise((resolve, reject) => this.pending.set(id, { resolve, reject }))
  }

  #notify(method, params) {
    this.child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", method, params })}\n`)
  }

  async initialize() {
    const result = await this.#send("initialize", {
      protocolVersion: PROTOCOL_VERSION,
      capabilities: {},
      clientInfo: { name: "conformance", version: "0" },
    })
    this.#notify("notifications/initialized", {})
    return result
  }

  listTools() {
    return this.#send("tools/list", {})
  }

  callTool(name, args) {
    return this.#send("tools/call", { name, arguments: args })
  }

  async close() {
    this.child.stdin.end()
    this.child.kill()
  }
}
