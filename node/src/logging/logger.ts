/**
 * Structured logging.
 *
 * Two rules, both load-bearing:
 *
 * 1. Output goes to **stderr**. stdout is the MCP stdio channel; a log line
 *    written there corrupts the protocol stream.
 * 2. Fields are an **allow-list**. Comments carry personal and user-generated
 *    content, so a deny-list would leak the first field somebody forgot.
 */

export interface LogFields {
  operation: string
  pageId?: string
  postId?: string
  commentId?: string
  /** Meta's fbtrace_id. Quote this when escalating to Meta support. */
  metaRequestId?: string
  durationMs?: number
  ok: boolean
  /** Short, fixed diagnostic text. Never provider content. */
  note?: string
}

const ALLOWED = [
  "operation",
  "pageId",
  "postId",
  "commentId",
  "metaRequestId",
  "durationMs",
  "ok",
  "note",
] as const

export interface Logger {
  log(fields: LogFields): void
}

export function createLogger(
  options: { sink?: (line: string) => void; debug?: boolean } = {},
): Logger {
  const sink = options.sink ?? ((line: string) => void process.stderr.write(`${line}\n`))

  return {
    log(fields) {
      const entry: Record<string, unknown> = {}
      for (const key of ALLOWED) {
        const value = fields[key]
        if (value !== undefined) entry[key] = value
      }
      sink(JSON.stringify(entry))
    },
  }
}

/** Run an operation, logging its duration and outcome either way. */
export async function timed<T>(
  logger: Logger,
  base: Omit<LogFields, "ok" | "durationMs">,
  run: () => Promise<T>,
): Promise<T> {
  const started = Date.now()
  try {
    const result = await run()
    logger.log({ ...base, ok: true, durationMs: Date.now() - started })
    return result
  } catch (cause) {
    // The error message is deliberately not logged: Graph errors can quote the
    // content that caused them.
    logger.log({ ...base, ok: false, durationMs: Date.now() - started })
    throw cause
  }
}
