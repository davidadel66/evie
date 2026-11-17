import type { Chunk, Plugin, Provider, Request, Usage } from "../core/types.ts"

// The built-in provider. It reaches every model through OpenRouter with one
// plain HTTPS request per step, and reads the streamed reply itself.
//
// The wire format is OpenRouter's chat completions API, checked against its
// documentation on 2026-11-17:
// https://openrouter.ai/docs/api-reference/overview

const ENDPOINT = "https://openrouter.ai/api/v1/chat/completions"
const RETRIES = 3
const LONGEST_WAIT = 60_000

export type OpenRouterOptions = {
  // Settings from the environment. The API key is `OPENROUTER_API_KEY`; it
  // is used only here and never written anywhere.
  env: Record<string, string | undefined>
  // Stand-ins for the network and the clock, for tests.
  fetch?: typeof globalThis.fetch
  sleep?: (milliseconds: number, signal: AbortSignal) => Promise<void>
}

export function openrouter(options: OpenRouterOptions): Plugin {
  return {
    name: "openrouter",
    setup(host) {
      host.providers.add(createOpenRouterProvider(options))
    },
  }
}

const NO_USAGE: Usage = { inputTokens: 0, outputTokens: 0 }
const failed = (error: string): Chunk => ({ type: "finish", reason: "error", error, usage: NO_USAGE })

export function createOpenRouterProvider(options: OpenRouterOptions): Provider {
  const send = options.fetch ?? globalThis.fetch
  const sleep = options.sleep ?? wait

  return {
    name: "openrouter",

    async *stream(request, signal) {
      const key = options.env.OPENROUTER_API_KEY
      if (!key) {
        yield failed("No API key. Set OPENROUTER_API_KEY to your OpenRouter key.")
        return
      }

      const init = {
        method: "POST",
        headers: { Authorization: `Bearer ${key}`, "Content-Type": "application/json" },
        body: JSON.stringify(toBody(request)),
        signal,
      }

      // A request is retried only while nothing has been received: once a
      // reply has started streaming, a failure ends it.
      let response: Response | undefined
      let problem = ""
      let delay = 0
      for (let attempt = 0; attempt <= RETRIES; attempt++) {
        if (attempt > 0) await sleep(delay, signal)
        if (signal.aborted) break
        try {
          const received = await send(ENDPOINT, init)
          if (received.ok) {
            response = received
            break
          }
          problem = `OpenRouter could not answer (${received.status}): ${await errorMessage(received)}`
          const retryable = received.status === 429 || received.status >= 500
          if (!retryable) break
          delay = retryAfter(received) ?? 1000 * 2 ** attempt
        } catch (cause) {
          if (signal.aborted) break
          problem = `OpenRouter could not be reached: ${cause instanceof Error ? cause.message : String(cause)}`
          delay = 1000 * 2 ** attempt
        }
      }

      if (signal.aborted) yield failed("The request was cancelled.")
      else if (!response) yield failed(problem)
      else yield* readReply(response)
    },
  }
}

// ---- The request ----------------------------------------------------------------

function toBody(request: Request) {
  return {
    model: request.model,
    messages: request.messages.map((message) => ({ role: message.role, content: message.text })),
    // Left out when there are none: an empty list is not documented.
    ...(request.tools.length > 0 && {
      tools: request.tools.map((tool) => ({
        type: "function",
        function: { name: tool.name, description: tool.description, parameters: tool.inputSchema },
      })),
    }),
    stream: true,
  }
}

// ---- The reply --------------------------------------------------------------------

// The parts of a streamed chunk that are read. Everything else is ignored.
type WireChunk = {
  error?: { code?: number; message?: string }
  choices?: {
    delta?: {
      content?: string | null
      tool_calls?: { index: number; id?: string; function?: { name?: string; arguments?: string } }[]
    }
    finish_reason?: string | null
  }[]
  usage?: { prompt_tokens?: number; completion_tokens?: number }
}

async function* readReply(response: Response): AsyncGenerator<Chunk> {
  // A tool call arrives in pieces that share an index. The first piece has
  // its id and name; later pieces add to its arguments.
  const calls = new Map<number, { id: string; name: string; arguments: string }>()
  let finishReason: string | undefined
  let usage = NO_USAGE

  for await (const chunk of wireChunks(response)) {
    if (chunk.error) {
      const code = chunk.error.code ?? "unknown"
      yield failed(`OpenRouter could not finish the reply (${code}): ${chunk.error.message ?? "no reason given"}`)
      return
    }
    const choice = chunk.choices?.[0]
    if (choice?.delta?.content) yield { type: "text", text: choice.delta.content }
    for (const piece of choice?.delta?.tool_calls ?? []) {
      const call = calls.get(piece.index) ?? { id: "", name: "", arguments: "" }
      if (piece.id) call.id = piece.id
      if (piece.function?.name) call.name = piece.function.name
      call.arguments += piece.function?.arguments ?? ""
      calls.set(piece.index, call)
    }
    // The reason arrives twice: with the last content, and again on the
    // final chunk that carries the usage.
    if (choice?.finish_reason) finishReason = choice.finish_reason
    if (chunk.usage) {
      usage = { inputTokens: chunk.usage.prompt_tokens ?? 0, outputTokens: chunk.usage.completion_tokens ?? 0 }
    }
  }

  for (const index of [...calls.keys()].sort((a, b) => a - b)) {
    yield { type: "tool_call", ...calls.get(index)! }
  }

  switch (finishReason) {
    case "stop":
    case "tool_calls":
    case "length":
      yield { type: "finish", reason: finishReason, usage }
      return
    case undefined:
      yield failed("The reply ended before it was finished.")
      return
    case "content_filter":
      yield failed("The reply was blocked by a content filter.")
      return
    default:
      yield failed(`The reply ended for a reason that is not understood: ${finishReason}.`)
  }
}

// Reads the reply as server-sent events: one `data:` line per chunk, ended by
// `data: [DONE]`. Lines that begin with a colon are keep-alive comments.
async function* wireChunks(response: Response): AsyncGenerator<WireChunk> {
  if (!response.body) return
  const decoder = new TextDecoder()
  let buffer = ""
  for await (const bytes of response.body) {
    buffer += decoder.decode(bytes, { stream: true })
    let end: number
    while ((end = buffer.indexOf("\n")) !== -1) {
      const line = buffer.slice(0, end).trim()
      buffer = buffer.slice(end + 1)
      if (!line.startsWith("data:")) continue
      const payload = line.slice("data:".length).trim()
      if (payload === "[DONE]") return
      try {
        yield JSON.parse(payload) as WireChunk
      } catch {
        // A line that is not JSON carries nothing that can be used.
      }
    }
  }
}

// ---- Failures and waiting -----------------------------------------------------------

async function errorMessage(response: Response): Promise<string> {
  const body = await response.text().catch(() => "")
  try {
    const message = (JSON.parse(body) as { error?: { message?: string } }).error?.message
    if (message) return message
  } catch {}
  return body.slice(0, 200) || "no reason given"
}

// How long OpenRouter asks to wait before trying again, when it says.
function retryAfter(response: Response): number | undefined {
  const seconds = Number(response.headers.get("retry-after"))
  if (!Number.isFinite(seconds) || seconds <= 0) return undefined
  return Math.min(seconds * 1000, LONGEST_WAIT)
}

function wait(milliseconds: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(done, milliseconds)
    signal.addEventListener("abort", done, { once: true })
    function done() {
      clearTimeout(timer)
      signal.removeEventListener("abort", done)
      resolve()
    }
  })
}
