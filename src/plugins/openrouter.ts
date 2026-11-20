import type { Chunk, Message, Plugin, Provider, Request, Usage } from "../core/types.ts"

// The built-in provider. It reaches every model through OpenRouter with one
// plain HTTPS request per step, and reads the streamed reply itself.
//
// The wire format is OpenRouter's chat completions API, checked against its
// documentation on 2026-11-17:
// https://openrouter.ai/docs/api-reference/overview

const ENDPOINT = "https://openrouter.ai/api/v1/chat/completions"
const MODELS = "https://openrouter.ai/api/v1/models"
// How a provider says a request was more than the model can read. The
// wording differs from one model company to the next. It is looked for only
// in a request that was refused as bad or too large, because a rate limit or
// a network failure can use the same words.
const TOO_LONG = /context (length|window)|maximum context|too many tokens|prompt is too long|input token count|maximum number of tokens|reduce the length/i
const REFUSED = [400, 413]
// How long the list of models may take to arrive.
const LIST_TIMEOUT = 10_000
const RETRIES = 3
// How long a reply may be, in tokens, unless the settings say otherwise.
// Without a limit OpenRouter reserves the model's maximum, and refuses the
// request when the account's credits do not cover that.
const DEFAULT_MAX_TOKENS = 16_000
const LONGEST_WAIT = 60_000

export type OpenRouterOptions = {
  // Settings from the environment. The API key is `OPENROUTER_API_KEY`; it
  // is used only here and never written anywhere. `EVIE_MAX_TOKENS` sets how
  // long a reply may be.
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

// A failed request, as the chunk that ends it. `status` is the HTTP status
// or error code it was refused with, when there was one.
const failed = (error: string, status?: number): Chunk => ({
  type: "finish",
  reason: "error",
  error,
  ...(status !== undefined && REFUSED.includes(status) && TOO_LONG.test(error) && { tooLong: true as const }),
})

export function createOpenRouterProvider(options: OpenRouterOptions): Provider {
  const send = options.fetch ?? globalThis.fetch
  const sleep = options.sleep ?? wait
  // What each model can read, fetched from OpenRouter's list once and kept.
  let limits: Promise<Map<string, number> | undefined> | undefined

  return {
    name: "openrouter",

    async contextLimit(model) {
      limits ??= send(MODELS, { signal: AbortSignal.timeout(LIST_TIMEOUT) })
        .then((response) => response.json() as Promise<{ data?: { id?: string; context_length?: number }[] }>)
        .then((body) => new Map((body.data ?? []).flatMap((entry) => (entry.id && entry.context_length ? [[entry.id, entry.context_length] as const] : []))))
        // If the list cannot be fetched, the limit is simply not known.
        .catch(() => undefined)
      const list = await limits
      // A failed fetch is not kept, so the next call tries again.
      if (!list) limits = undefined
      return list?.get(model)
    },

    async *stream(request, signal) {
      const key = options.env.OPENROUTER_API_KEY
      if (!key) {
        yield failed("No API key. Set OPENROUTER_API_KEY to your OpenRouter key.")
        return
      }

      const init = {
        method: "POST",
        headers: { Authorization: `Bearer ${key}`, "Content-Type": "application/json" },
        body: JSON.stringify(toBody(request, request.maxTokens ?? maxTokens(options.env.EVIE_MAX_TOKENS))),
        signal,
      }

      // A request is retried only while nothing has been received: once a
      // reply has started streaming, a failure ends it.
      let response: Response | undefined
      let problem = ""
      let status: number | undefined
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
          status = received.status
          const retryable = received.status === 429 || received.status >= 500
          if (!retryable) break
          delay = retryAfter(received) ?? 1000 * 2 ** attempt
        } catch (cause) {
          if (signal.aborted) break
          problem = `OpenRouter could not be reached: ${cause instanceof Error ? cause.message : String(cause)}`
          status = undefined
          delay = 1000 * 2 ** attempt
        }
      }

      if (signal.aborted) yield failed("The request was cancelled.")
      else if (!response) yield failed(problem, status)
      else yield* readReply(response, signal)
    },
  }
}

// ---- The request ----------------------------------------------------------------

function maxTokens(setting: string | undefined): number {
  const asked = Number(setting)
  return Number.isInteger(asked) && asked > 0 ? asked : DEFAULT_MAX_TOKENS
}

function toBody(request: Request, limit: number) {
  return {
    model: request.model,
    max_tokens: limit,
    messages: [
      ...(request.system === undefined ? [] : [{ role: "system", content: request.system }]),
      ...request.messages.map(toWireMessage),
    ],
    // Left out when there are none: an empty list is not documented.
    ...(request.tools.length > 0 && {
      tools: request.tools.map((tool) => ({
        type: "function",
        function: { name: tool.name, description: tool.description, parameters: tool.inputSchema },
      })),
    }),
    stream: true,
    // Every request repeats the conversation so far, so most of it can be
    // read from the provider's prompt cache at a fraction of the price.
    // Most providers do that by themselves. Anthropic's models do it only
    // when asked: this asks, and the cached part then grows with the
    // conversation. Checked against OpenRouter's documentation on
    // 2026-11-20: https://openrouter.ai/docs/features/prompt-caching
    ...(request.model.startsWith("anthropic/") && { cache_control: { type: "ephemeral" } }),
  }
}

function toWireMessage(message: Message) {
  switch (message.role) {
    case "user":
      return { role: "user", content: message.text }
    case "assistant":
      if (!message.toolCalls) return { role: "assistant", content: message.text }
      return {
        role: "assistant",
        content: message.text === "" ? null : message.text,
        tool_calls: message.toolCalls.map((call) => ({
          id: call.id,
          type: "function",
          function: { name: call.name, arguments: call.arguments },
        })),
      }
    case "tool":
      return { role: "tool", tool_call_id: message.callId, content: message.text }
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
  usage?: {
    prompt_tokens?: number
    completion_tokens?: number
    prompt_tokens_details?: { cached_tokens?: number; cache_write_tokens?: number } | null
  }
}

async function* readReply(response: Response, signal: AbortSignal): AsyncGenerator<Chunk> {
  // A tool call arrives in pieces that share an index. The first piece has
  // its id and name; later pieces add to its arguments.
  const calls = new Map<number, { id: string; name: string; arguments: string }>()
  let finishReason: string | undefined
  // The token counts, when the reply carries them. Not every one does.
  let usage: Usage | undefined

  const chunks = wireChunks(response)
  while (true) {
    let chunk: WireChunk
    try {
      const next = await chunks.next()
      if (next.done) break
      chunk = next.value
    } catch (cause) {
      // The connection broke part way through the reply.
      if (signal.aborted) yield failed("The request was cancelled.")
      else yield failed(`The connection to OpenRouter was lost: ${cause instanceof Error ? cause.message : String(cause)}`)
      return
    }
    if (chunk.error) {
      const code = chunk.error.code ?? "unknown"
      yield failed(`OpenRouter could not finish the reply (${code}): ${chunk.error.message ?? "no reason given"}`, Number(chunk.error.code))
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
      const cached = chunk.usage.prompt_tokens_details?.cached_tokens
      const written = chunk.usage.prompt_tokens_details?.cache_write_tokens
      usage = {
        inputTokens: chunk.usage.prompt_tokens ?? 0,
        outputTokens: chunk.usage.completion_tokens ?? 0,
        ...(typeof cached === "number" && { cachedTokens: cached }),
        ...(typeof written === "number" && { cacheWriteTokens: written }),
      }
    }
  }

  for (const index of [...calls.keys()].sort((a, b) => a - b)) {
    yield { type: "tool_call", ...calls.get(index)! }
  }

  switch (finishReason) {
    case "stop":
    case "tool_calls":
    case "length":
      yield { type: "finish", reason: finishReason, ...(usage && { usage }) }
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
      const chunk = parseLine(buffer.slice(0, end))
      buffer = buffer.slice(end + 1)
      if (chunk === "done") return
      if (chunk) yield chunk
    }
  }
  // The last line may arrive with no line ending after it.
  const last = parseLine(buffer)
  if (last && last !== "done") yield last
}

function parseLine(raw: string): WireChunk | "done" | undefined {
  const line = raw.trim()
  if (!line.startsWith("data:")) return undefined
  const payload = line.slice("data:".length).trim()
  if (payload === "[DONE]") return "done"
  try {
    return JSON.parse(payload) as WireChunk
  } catch {
    // A line that is not JSON carries nothing that can be used.
    return undefined
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
    if (signal.aborted) return resolve()
    const timer = setTimeout(done, milliseconds)
    signal.addEventListener("abort", done, { once: true })
    function done() {
      clearTimeout(timer)
      signal.removeEventListener("abort", done)
      resolve()
    }
  })
}
