import { afterEach, expect, test } from "bun:test"
import { shell, type EvieEvent, type Plugin } from "../src/index.ts"
import { say, startEvie, stored, type Run } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

let stop: (() => Promise<void>) | undefined
afterEach(async () => {
  await stop?.()
  stop = undefined
})

async function start(script: ScriptedReply[], plugins: Plugin[] = []) {
  const model = scriptedProvider(script)
  const run = await startEvie([model.plugin, ...plugins])
  stop = run.stop
  return { run, model }
}

// Resolves when the client is next sent an event that matches.
function next(run: Run, matches: (event: EvieEvent) => boolean) {
  return new Promise<EvieEvent>((resolve) => {
    const stop = run.evie.subscribe((event) => {
      if (matches(event)) {
        stop()
        resolve(event)
      }
    })
  })
}

// Sends a message, cancels the turn once the given moment arrives, and waits for the turn to end.
async function sayThenCancel(run: Run, text: string, moment: (event: EvieEvent) => boolean) {
  const reached = next(run, moment)
  const ended = next(run, (event) => event.type === "turn_end")
  await run.evie.send({ type: "send_message", session: run.session, text })
  await reached
  await run.evie.send({ type: "cancel", session: run.session })
  await ended
}

// A tool that takes a long time, and stops when the turn is cancelled.
const slowTool: Plugin = {
  name: "slow-tool",
  setup(host) {
    host.tools.add({
      name: "wait",
      description: "Waits.",
      inputSchema: { type: "object" },
      run: (_input, context) =>
        new Promise((resolve) => context.signal.addEventListener("abort", () => resolve("stopped waiting"), { once: true })),
    })
  },
}

test("cancelling while the model is replying keeps the text so far, marked as interrupted", async () => {
  const { run } = await start([{ hang: true, text: ["Hel", "lo"] }])

  await sayThenCancel(run, "hi", (event) => event.type === "assistant_delta" && event.text === "lo")

  expect(stored(run.events).map((event) => event.type)).toEqual(["turn_start", "user_message", "assistant_message", "turn_end"])
  expect(stored(run.events)[2]).toMatchObject({ body: { text: "Hello", interrupted: true } })
  expect(stored(run.events).at(-1)).toMatchObject({ body: { reason: "cancelled" } })
})

test("cancelling before the model has said anything stores no reply", async () => {
  const { run } = await start([{ hang: true }])

  await sayThenCancel(run, "hi", (event) => event.type === "user_message")

  expect(stored(run.events).map((event) => event.type)).toEqual(["turn_start", "user_message", "turn_end"])
  expect(stored(run.events).at(-1)).toMatchObject({ body: { reason: "cancelled" } })
})

test("a provider that reports the cancelled request as a failure still ends the turn as cancelled", async () => {
  const { run } = await start([{ hang: true, text: ["Hel"], reports: true }])

  await sayThenCancel(run, "hi", (event) => event.type === "assistant_delta")

  expect(stored(run.events).at(-1)).toMatchObject({ type: "turn_end", body: { reason: "cancelled" } })
  expect(stored(run.events)[2]).toMatchObject({ type: "assistant_message", body: { text: "Hel", interrupted: true } })
})

test("cancelling while a tool runs stops it, and every call still gets a result", async () => {
  const { run } = await start(
    [{ text: ["Working."], calls: [{ name: "wait", input: {} }, { name: "wait", input: {} }] }],
    [slowTool],
  )

  await sayThenCancel(run, "go", (event) => event.type === "assistant_message")

  const results = stored(run.events).flatMap((event) => (event.type === "tool_result" ? [event.body] : []))
  expect(results).toEqual([
    { callId: "call_1_0", name: "wait", text: "stopped waiting" },
    { callId: "call_1_1", name: "wait", text: "Cancelled: the turn was stopped before this tool ran." },
  ])
  expect(stored(run.events).at(-1)).toMatchObject({ type: "turn_end", body: { reason: "cancelled" } })
})

test("after a cancel the conversation carries on, with a result for every tool call", async () => {
  const { run, model } = await start(
    [{ text: ["Working."], calls: [{ name: "wait", input: {} }, { name: "wait", input: {} }] }, { text: ["Carrying on."] }],
    [slowTool],
  )

  await sayThenCancel(run, "go", (event) => event.type === "assistant_message")
  await say(run, "what happened?")

  const asked = model.requests[1]!.messages
  const callIds = asked.flatMap((message) => (message.role === "assistant" ? (message.toolCalls ?? []).map((call) => call.id) : []))
  const resultIds = asked.flatMap((message) => (message.role === "tool" ? [message.callId] : []))
  expect(resultIds).toEqual(callIds)
  expect(callIds).toHaveLength(2)
  expect(stored(run.events).at(-1)).toMatchObject({ type: "turn_end", body: { reason: "completed" } })
})

test("the interrupted reply is part of what the model sees next", async () => {
  const { run, model } = await start([{ hang: true, text: ["Half an ans"] }, { text: ["The rest."] }])

  await sayThenCancel(run, "explain", (event) => event.type === "assistant_delta")
  await say(run, "go on")

  expect(model.requests[1]?.messages).toEqual([
    { role: "user", text: "explain" },
    { role: "assistant", text: "Half an ans" },
    { role: "user", text: "go on" },
  ])
})

test("cancelling stops a running shell command", async () => {
  const { run } = await start([{ text: [], calls: [{ name: "shell", input: { command: "sleep 30" } }] }], [shell()])
  const started = Date.now()

  await sayThenCancel(run, "go", (event) => event.type === "assistant_message")

  const results = stored(run.events).flatMap((event) => (event.type === "tool_result" ? [event.body.text] : []))
  expect(results).toEqual(["[Stopped: the turn was cancelled.]"])
  expect(Date.now() - started).toBeLessThan(10_000)
})

test("cancelling when no turn is running does nothing", async () => {
  const { run } = await start([{ text: ["ok"] }])

  await run.evie.send({ type: "cancel", session: run.session })
  await say(run, "hi")

  expect(stored(run.events).map((event) => event.type)).toEqual(["turn_start", "user_message", "assistant_message", "turn_end"])
  expect(stored(run.events).at(-1)).toMatchObject({ body: { reason: "completed" } })
})

test("cancelling a session that is not open is refused", async () => {
  const { run } = await start([])

  await expect(run.evie.send({ type: "cancel", session: "no-such-session" })).rejects.toThrow('The session "no-such-session" is not open')
})

test("a message waiting when a turn is cancelled is answered in a turn of its own", async () => {
  const { run, model } = await start([{ hang: true }, { text: ["Second answer."] }])
  const bothEnded = new Promise<void>((resolve) => {
    let ended = 0
    run.evie.subscribe((event) => {
      if (event.type === "turn_end" && ++ended === 2) resolve()
    })
  })

  await run.evie.send({ type: "send_message", session: run.session, text: "first" })
  await run.evie.send({ type: "send_message", session: run.session, text: "second" })
  await run.evie.send({ type: "cancel", session: run.session })
  await bothEnded

  expect(model.requests[1]?.messages).toEqual([
    { role: "user", text: "first" },
    { role: "user", text: "second" },
  ])
})
