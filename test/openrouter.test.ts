import { expect, test } from "bun:test"
import { createOpenRouterProvider, type Chunk, type Request } from "../src/index.ts"
import { DONE, data, fakeOpenRouter, sse, type FakeResponse } from "./support/fake-openrouter.ts"

const KEY = "sk-or-test-key-123"
const request: Request = { model: "some/model", messages: [{ role: "user", text: "hi" }], tools: [] }

async function ask(
  responses: FakeResponse[],
  options: { key?: string | undefined; request?: Request; maxTokens?: string } = {},
) {
  const fake = fakeOpenRouter(responses)
  const provider = createOpenRouterProvider({
    env: { OPENROUTER_API_KEY: "key" in options ? options.key : KEY, EVIE_MAX_TOKENS: options.maxTokens },
    fetch: fake.fetch,
    sleep: fake.sleep,
  })
  const chunks: Chunk[] = []
  for await (const chunk of provider.stream(options.request ?? request, new AbortController().signal)) chunks.push(chunk)
  return { chunks, ...fake }
}

const text = (content: string) => data({ choices: [{ index: 0, delta: { role: "assistant", content }, finish_reason: null }] })
const finish = (reason: string) => data({ choices: [{ index: 0, delta: { content: "" }, finish_reason: reason }] })
const usage = (reason: string) =>
  data({
    choices: [{ index: 0, delta: { role: "assistant", content: "" }, finish_reason: reason }],
    usage: { prompt_tokens: 12, completion_tokens: 5, total_tokens: 17 },
  })
const ok = (...lines: string[]): FakeResponse => ({ status: 200, body: sse(...lines) })
const failure = (status: number, message: string, headers?: Record<string, string>): FakeResponse => ({
  status,
  body: JSON.stringify({ error: { code: status, message } }),
  ...(headers ? { headers } : {}),
})

test("a streamed reply becomes text chunks and one finish chunk with the reason and the token usage", async () => {
  const { chunks } = await ask([ok(text("Hello"), text(" there"), finish("stop"), usage("stop"), DONE)])

  expect(chunks).toEqual([
    { type: "text", text: "Hello" },
    { type: "text", text: " there" },
    { type: "finish", reason: "stop", usage: { inputTokens: 12, outputTokens: 5 } },
  ])
})

test("the request goes to OpenRouter's chat completions endpoint with the key as a bearer token", async () => {
  const { calls } = await ask([ok(text("ok"), usage("stop"), DONE)])

  expect(calls[0]?.url).toBe("https://openrouter.ai/api/v1/chat/completions")
  expect(calls[0]?.headers.authorization).toBe(`Bearer ${KEY}`)
  expect(calls[0]?.body).toEqual({
    model: "some/model",
    max_tokens: 16000,
    messages: [{ role: "user", content: "hi" }],
    stream: true,
  })
})

test("tools are described to the model in OpenRouter's function format", async () => {
  const withTool: Request = {
    ...request,
    tools: [{ name: "read", description: "Reads a file.", inputSchema: { type: "object", properties: {} } }],
  }
  const { calls } = await ask([ok(text("ok"), usage("stop"), DONE)], { request: withTool })

  expect(calls[0]?.body.tools).toEqual([
    {
      type: "function",
      function: { name: "read", description: "Reads a file.", parameters: { type: "object", properties: {} } },
    },
  ])
})

test("the pieces of each tool call are joined by their index into complete calls", async () => {
  const piece = (index: number, call: object) =>
    data({ choices: [{ index: 0, delta: { tool_calls: [{ index, ...call }] }, finish_reason: null }] })
  const { chunks } = await ask([
    ok(
      text("Let me look."),
      piece(0, { id: "call_a", type: "function", function: { name: "read", arguments: "" } }),
      piece(0, { function: { arguments: '{"path":' } }),
      piece(1, { id: "call_b", type: "function", function: { name: "grep", arguments: '{"pattern":"TODO"}' } }),
      piece(0, { function: { arguments: '"a.ts"}' } }),
      finish("tool_calls"),
      usage("tool_calls"),
      DONE,
    ),
  ])

  expect(chunks).toEqual([
    { type: "text", text: "Let me look." },
    { type: "tool_call", id: "call_a", name: "read", arguments: '{"path":"a.ts"}' },
    { type: "tool_call", id: "call_b", name: "grep", arguments: '{"pattern":"TODO"}' },
    { type: "finish", reason: "tool_calls", usage: { inputTokens: 12, outputTokens: 5 } },
  ])
})

test("keep-alive comments are ignored, and lines split across network pieces are put back together", async () => {
  const body = sse(": OPENROUTER PROCESSING", text("Hello"), ": OPENROUTER PROCESSING", text(" there"), usage("stop"), DONE)
  const { chunks } = await ask([{ status: 200, body, pieces: 7 }])

  expect(chunks.filter((chunk) => chunk.type === "text").map((chunk) => chunk.text)).toEqual(["Hello", " there"])
  expect(chunks.at(-1)).toMatchObject({ type: "finish", reason: "stop" })
})

test("a reply cut off by the length limit says so", async () => {
  const { chunks } = await ask([ok(text("Hello"), finish("length"), usage("length"), DONE)])

  expect(chunks.at(-1)).toMatchObject({ type: "finish", reason: "length" })
})

test("a request refused as rate limited is retried, waiting as long as OpenRouter asks", async () => {
  const { chunks, calls, waits } = await ask([
    failure(429, "Rate limited", { "retry-after": "7" }),
    ok(text("Hello"), usage("stop"), DONE),
  ])

  expect(calls).toHaveLength(2)
  expect(waits).toEqual([7000])
  expect(chunks[0]).toEqual({ type: "text", text: "Hello" })
})

test("a request that fails with a server error is retried, waiting longer each time", async () => {
  const { chunks, calls, waits } = await ask([
    failure(502, "Chosen model is down"),
    failure(503, "No available provider"),
    ok(text("Hello"), usage("stop"), DONE),
  ])

  expect(calls).toHaveLength(3)
  expect(waits).toEqual([1000, 2000])
  expect(chunks.at(-1)).toMatchObject({ type: "finish", reason: "stop" })
})

test("a request that cannot reach OpenRouter is retried", async () => {
  const { chunks, calls } = await ask([{ networkError: "fetch failed" }, ok(text("Hello"), usage("stop"), DONE)])

  expect(calls).toHaveLength(2)
  expect(chunks[0]).toEqual({ type: "text", text: "Hello" })
})

test("after three retries the failure is reported as the end of the reply", async () => {
  const down = failure(503, "No available provider")
  const { chunks, calls } = await ask([down, down, down, down])

  expect(calls).toHaveLength(4)
  expect(chunks).toEqual([
    {
      type: "finish",
      reason: "error",
      error: "OpenRouter could not answer (503): No available provider",
    },
  ])
})

test("a request OpenRouter refuses outright is not retried, and the key is not in the message", async () => {
  const { chunks, calls } = await ask([failure(401, "Invalid credentials")])

  expect(calls).toHaveLength(1)
  expect(chunks).toEqual([
    {
      type: "finish",
      reason: "error",
      error: "OpenRouter could not answer (401): Invalid credentials",
    },
  ])
  expect(JSON.stringify(chunks)).not.toContain(KEY)
})

test("a failure after text has streamed ends the reply and is not retried", async () => {
  const broken = data({
    error: { code: 502, message: "Provider disconnected unexpectedly" },
    choices: [{ index: 0, delta: { content: "" }, finish_reason: "error" }],
  })
  const { chunks, calls } = await ask([ok(text("Hello"), broken)])

  expect(calls).toHaveLength(1)
  expect(chunks).toEqual([
    { type: "text", text: "Hello" },
    {
      type: "finish",
      reason: "error",
      error: "OpenRouter could not finish the reply (502): Provider disconnected unexpectedly",
    },
  ])
})

test("a reply that stops without finishing is reported as a failure", async () => {
  const { chunks } = await ask([ok(text("Hello"))])

  expect(chunks.at(-1)).toMatchObject({ type: "finish", reason: "error", error: "The reply ended before it was finished." })
})

test("without an API key no request is made, and the failure says how to set one", async () => {
  const { chunks, calls } = await ask([], { key: undefined })

  expect(calls).toHaveLength(0)
  expect(chunks).toEqual([
    {
      type: "finish",
      reason: "error",
      error: "No API key. Set OPENROUTER_API_KEY to your OpenRouter key.",
    },
  ])
})

test("a connection that breaks part way through the reply is reported, not thrown", async () => {
  const { chunks } = await ask([{ status: 200, body: sse(text("Hello")), cutOff: true }])

  expect(chunks).toEqual([
    { type: "text", text: "Hello" },
    {
      type: "finish",
      reason: "error",
      error: "The connection to OpenRouter was lost: socket closed",
    },
  ])
})

test("the last line of a reply is read even with no line ending after it", async () => {
  const body = sse(text("Hello")) + usage("stop")
  const { chunks } = await ask([{ status: 200, body }])

  expect(chunks.at(-1)).toEqual({ type: "finish", reason: "stop", usage: { inputTokens: 12, outputTokens: 5 } })
})

test("earlier tool calls and their results are sent back in OpenRouter's format", async () => {
  const withTools: Request = {
    model: "some/model",
    tools: [],
    messages: [
      { role: "user", text: "read it" },
      { role: "assistant", text: "", toolCalls: [{ id: "call_a", name: "read", arguments: '{"path":"a.ts"}' }] },
      { role: "tool", callId: "call_a", text: "1: hello" },
      { role: "assistant", text: "It says hello." },
    ],
  }
  const { calls } = await ask([ok(text("ok"), usage("stop"), DONE)], { request: withTools })

  expect(calls[0]?.body.messages).toEqual([
    { role: "user", content: "read it" },
    {
      role: "assistant",
      content: null,
      tool_calls: [{ id: "call_a", type: "function", function: { name: "read", arguments: '{"path":"a.ts"}' } }],
    },
    { role: "tool", tool_call_id: "call_a", content: "1: hello" },
    { role: "assistant", content: "It says hello." },
  ])
})

test("the system prompt is sent as the first message", async () => {
  const { calls } = await ask([ok(text("ok"), usage("stop"), DONE)], { request: { ...request, system: "You are Evie." } })

  expect(calls[0]?.body.messages).toEqual([
    { role: "system", content: "You are Evie." },
    { role: "user", content: "hi" },
  ])
})

test("a reply is limited to 16,000 tokens unless the settings say otherwise", async () => {
  const usual = await ask([ok(text("ok"), usage("stop"), DONE)])
  const chosen = await ask([ok(text("ok"), usage("stop"), DONE)], { maxTokens: "4000" })
  const nonsense = await ask([ok(text("ok"), usage("stop"), DONE)], { maxTokens: "lots" })

  expect(usual.calls[0]?.body.max_tokens).toBe(16000)
  expect(chosen.calls[0]?.body.max_tokens).toBe(4000)
  expect(nonsense.calls[0]?.body.max_tokens).toBe(16000)
})

test("what a model can read is looked up in OpenRouter's list, once", async () => {
  const fake = fakeOpenRouter([
    { status: 200, body: JSON.stringify({ data: [{ id: "some/model", context_length: 200000 }, { id: "other/model", context_length: 8000 }] }) },
  ])
  const provider = createOpenRouterProvider({ env: { OPENROUTER_API_KEY: KEY }, fetch: fake.fetch, sleep: fake.sleep })

  expect(await provider.contextLimit?.("some/model")).toBe(200000)
  expect(await provider.contextLimit?.("other/model")).toBe(8000)
  expect(await provider.contextLimit?.("unknown/model")).toBeUndefined()
  expect(fake.calls).toHaveLength(1)
  expect(fake.calls[0]?.url).toBe("https://openrouter.ai/api/v1/models")
})

test("when the list cannot be fetched, a model's limit is simply not known", async () => {
  const fake = fakeOpenRouter([{ networkError: "fetch failed" }])
  const provider = createOpenRouterProvider({ env: { OPENROUTER_API_KEY: KEY }, fetch: fake.fetch, sleep: fake.sleep })

  expect(await provider.contextLimit?.("some/model")).toBeUndefined()
})

test("a list that could not be fetched is fetched again the next time, and the fetch has a time limit", async () => {
  const fake = fakeOpenRouter([{ networkError: "fetch failed" }, { status: 200, body: JSON.stringify({ data: [{ id: "some/model", context_length: 200000 }] }) }])
  const provider = createOpenRouterProvider({ env: { OPENROUTER_API_KEY: KEY }, fetch: fake.fetch, sleep: fake.sleep })

  expect(await provider.contextLimit?.("some/model")).toBeUndefined()
  expect(await provider.contextLimit?.("some/model")).toBe(200000)
  expect(fake.calls.map((call) => call.canBeStopped)).toEqual([true, true])
})

test("the models OpenRouter offers are listed from the same list, fetched once with the limits, each by its id and name", async () => {
  const fake = fakeOpenRouter([
    { status: 200, body: JSON.stringify({ data: [{ id: "some/model", name: "Some: Model", context_length: 200000 }, { id: "other/model", context_length: 8000 }, { name: "No id" }] }) },
  ])
  const provider = createOpenRouterProvider({ env: { OPENROUTER_API_KEY: KEY }, fetch: fake.fetch, sleep: fake.sleep })

  expect(await provider.models?.()).toEqual([
    { id: "some/model", name: "Some: Model" },
    { id: "other/model", name: "other/model" },
  ])
  expect(await provider.contextLimit?.("other/model")).toBe(8000)
  expect(fake.calls).toHaveLength(1)
})

test("when the list of models cannot be fetched, listing them says why, and the next listing fetches it again", async () => {
  const fake = fakeOpenRouter([{ networkError: "fetch failed" }, { status: 200, body: JSON.stringify({ data: [{ id: "some/model", name: "Some: Model" }] }) }])
  const provider = createOpenRouterProvider({ env: { OPENROUTER_API_KEY: KEY }, fetch: fake.fetch, sleep: fake.sleep })

  expect(provider.models?.()).rejects.toThrow("OpenRouter's list of models could not be fetched. Check the network, and try again.")
  await Bun.sleep(1)
  expect(await provider.models?.()).toEqual([{ id: "some/model", name: "Some: Model" }])
})

test("an error reply, or one without a list, is no list: it is not kept, and the next asking fetches it again", async () => {
  const fake = fakeOpenRouter([
    { status: 429, body: JSON.stringify({ error: { code: 429, message: "Rate limited" } }) },
    { status: 200, body: JSON.stringify({ error: { message: "Something went wrong" } }) },
    { status: 200, body: JSON.stringify({ data: [{ id: "some/model", name: "Some: Model", context_length: 200000 }] }) },
  ])
  const provider = createOpenRouterProvider({ env: { OPENROUTER_API_KEY: KEY }, fetch: fake.fetch, sleep: fake.sleep })

  expect(provider.models?.()).rejects.toThrow("OpenRouter's list of models could not be fetched.")
  await Bun.sleep(1)
  expect(await provider.contextLimit?.("some/model")).toBeUndefined()
  expect(await provider.contextLimit?.("some/model")).toBe(200000)
  expect(fake.calls).toHaveLength(3)
})

test("the list is kept for an hour, then fetched again, so that new models appear", async () => {
  const fake = fakeOpenRouter([
    { status: 200, body: JSON.stringify({ data: [{ id: "some/model", name: "Some: Model" }] }) },
    { status: 200, body: JSON.stringify({ data: [{ id: "some/model", name: "Some: Model" }, { id: "new/model", name: "New: Model" }] }) },
  ])
  let now = 1_000_000
  const provider = createOpenRouterProvider({ env: { OPENROUTER_API_KEY: KEY }, fetch: fake.fetch, sleep: fake.sleep, now: () => now })

  expect((await provider.models?.())?.map((model) => model.id)).toEqual(["some/model"])
  now += 60 * 60_000 - 1
  expect((await provider.models?.())?.map((model) => model.id)).toEqual(["some/model"])
  now += 1
  expect((await provider.models?.())?.map((model) => model.id)).toEqual(["some/model", "new/model"])
  expect(fake.calls).toHaveLength(2)
})

test("a reply that carries no token counts reports none", async () => {
  const { chunks } = await ask([ok(text("Hello"), finish("stop"), DONE)])

  expect(chunks.at(-1)).toEqual({ type: "finish", reason: "stop" })
})

test("a request refused for being more than the model can read is marked as too long", async () => {
  const tooLong = await ask([failure(400, "This endpoint's maximum context length is 200000 tokens. However, you requested about 250000 tokens.")])
  const other = await ask([failure(400, "Invalid model specified")])

  expect(tooLong.chunks[0]).toMatchObject({ type: "finish", reason: "error", tooLong: true })
  expect(other.chunks[0]).not.toHaveProperty("tooLong")
})

test("only a refused request is marked as too long, not a rate limit or a failure that reads alike", async () => {
  const busy = failure(429, "Rate limit reached: too many tokens per minute")
  const rateLimited = await ask([busy, busy, busy, busy])
  const unreachable = await ask([
    { networkError: "body exceeds the maximum context length" },
    { networkError: "body exceeds the maximum context length" },
    { networkError: "body exceeds the maximum context length" },
    { networkError: "body exceeds the maximum context length" },
  ])
  const tooLarge = await ask([failure(413, "prompt is too long: 250000 tokens > 200000 maximum")])
  const midReply = await ask([
    ok(text("Hello"), data({ error: { code: 400, message: "This model's maximum context length is 8000 tokens" }, choices: [] })),
  ])

  expect(rateLimited.chunks[0]).not.toHaveProperty("tooLong")
  expect(unreachable.chunks[0]).not.toHaveProperty("tooLong")
  expect(tooLarge.chunks[0]).toMatchObject({ tooLong: true })
  expect(midReply.chunks.at(-1)).toMatchObject({ reason: "error", tooLong: true })
})

// ---- The prompt cache ----------------------------------------------------------------------------

test("an Anthropic model is asked to cache the prompt, and other models are left to do it by themselves", async () => {
  const claude = await ask([ok(text("ok"), usage("stop"), DONE)], { request: { ...request, model: "anthropic/claude-sonnet-4.6" } })
  const other = await ask([ok(text("ok"), usage("stop"), DONE)], { request: { ...request, model: "openai/gpt-5.2" } })

  expect(claude.calls[0]?.body.cache_control).toEqual({ type: "ephemeral" })
  expect(other.calls[0]?.body).not.toHaveProperty("cache_control")
})

test("how much of a request was read from the cache, and written to it, is reported with its size", async () => {
  const withCache = data({
    choices: [{ index: 0, delta: { role: "assistant", content: "" }, finish_reason: "stop" }],
    usage: { prompt_tokens: 9000, completion_tokens: 40, prompt_tokens_details: { cached_tokens: 8200, cache_write_tokens: 600 } },
  })
  const { chunks } = await ask([ok(text("ok"), withCache, DONE)])
  const plain = await ask([ok(text("ok"), usage("stop"), DONE)])

  expect(chunks.at(-1)).toEqual({ type: "finish", reason: "stop", usage: { inputTokens: 9000, outputTokens: 40, cachedTokens: 8200, cacheWriteTokens: 600 } })
  // A provider that says nothing about its cache is not reported as having read nothing from it.
  expect(plain.chunks.at(-1)).toEqual({ type: "finish", reason: "stop", usage: { inputTokens: 12, outputTokens: 5 } })
})

test("text to stop at is passed on, and left out when there is none", async () => {
  const withStop = await ask([ok(text("ok"), usage("stop"), DONE)], { request: { ...request, stop: ["[Note from Evie:"] } })
  const without = await ask([ok(text("ok"), usage("stop"), DONE)])

  expect(withStop.calls[0]?.body.stop).toEqual(["[Note from Evie:"])
  expect(without.calls[0]?.body).not.toHaveProperty("stop")
})
