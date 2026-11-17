import { expect, test } from "bun:test"
import { createOpenRouterProvider, type Chunk, type Request } from "../src/index.ts"
import { DONE, data, fakeOpenRouter, sse, type FakeResponse } from "./support/fake-openrouter.ts"

const KEY = "sk-or-test-key-123"
const request: Request = { model: "some/model", messages: [{ role: "user", text: "hi" }], tools: [] }

async function ask(responses: FakeResponse[], options: { key?: string | undefined; request?: Request } = {}) {
  const fake = fakeOpenRouter(responses)
  const provider = createOpenRouterProvider({
    env: { OPENROUTER_API_KEY: "key" in options ? options.key : KEY },
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
      usage: { inputTokens: 0, outputTokens: 0 },
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
      usage: { inputTokens: 0, outputTokens: 0 },
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
      usage: { inputTokens: 0, outputTokens: 0 },
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
      usage: { inputTokens: 0, outputTokens: 0 },
    },
  ])
})
