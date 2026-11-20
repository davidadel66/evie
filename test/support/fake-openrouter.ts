// A stand-in for OpenRouter's HTTP endpoint. It answers each call with the
// next scripted response and records what it was sent, so the provider can be
// tested with no network.
//
// The streamed replies used with it are written from OpenRouter's
// documentation (read 2026-11-17), not captured from a live call.

export type FakeResponse =
  | { status: number; body: string; headers?: Record<string, string>; pieces?: number; cutOff?: boolean }
  | { networkError: string }

// `canBeStopped` says whether the call was given a signal that can end it.
export type RecordedCall = { url: string; headers: Record<string, string>; body: any; canBeStopped: boolean }

// Joins server-sent event lines the way OpenRouter sends them.
export const sse = (...lines: string[]) => lines.map((line) => `${line}\n\n`).join("")
export const data = (chunk: unknown) => `data: ${JSON.stringify(chunk)}`
export const DONE = "data: [DONE]"

export function fakeOpenRouter(responses: FakeResponse[]) {
  const calls: RecordedCall[] = []
  const waits: number[] = []

  const fetch = (async (input: Parameters<typeof globalThis.fetch>[0], init?: RequestInit) => {
    const response = responses[calls.length]
    calls.push({
      url: String(input),
      headers: Object.fromEntries(new Headers(init?.headers).entries()),
      body: init?.body === undefined ? undefined : JSON.parse(String(init.body)),
      canBeStopped: init?.signal != null,
    })
    if (!response) throw new Error("the fake has no response for this call")
    if ("networkError" in response) throw new TypeError(response.networkError)

    // The body arrives in several network pieces that ignore line boundaries.
    const bytes = new TextEncoder().encode(response.body)
    const size = Math.max(1, Math.ceil(bytes.length / (response.pieces ?? 1)))
    let at = 0
    const body = new ReadableStream<Uint8Array>({
      // One piece is handed over each time the reader asks for more.
      pull(controller) {
        if (at < bytes.length) {
          controller.enqueue(bytes.slice(at, at + size))
          at += size
        } else if (response.cutOff) {
          // `cutOff` stands for the connection breaking part way through.
          controller.error(new TypeError("socket closed"))
        } else {
          controller.close()
        }
      },
    })
    return new Response(body, { status: response.status, headers: response.headers ?? {} })
  }) as typeof globalThis.fetch

  // Stands in for waiting between retries, and records how long was asked for.
  const sleep = async (milliseconds: number) => {
    waits.push(milliseconds)
  }

  return { fetch, sleep, calls, waits }
}
