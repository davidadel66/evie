import { afterEach, expect, test } from "bun:test"
import { join } from "node:path"
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
// The moment is an event the client is sent, or a promise such as "the tool has started".
async function sayThenCancel(run: Run, text: string, moment: ((event: EvieEvent) => boolean) | Promise<unknown>) {
  const reached = typeof moment === "function" ? next(run, moment) : moment
  const ended = next(run, (event) => event.type === "turn_end")
  await run.evie.send({ type: "send_message", session: run.session, text })
  await reached
  await run.evie.send({ type: "cancel", session: run.session })
  await ended
}

// A tool that takes a long time, and stops when the turn is cancelled.
// `started` resolves once the tool is running.
function slowTool(onCancel: "returns" | "throws" = "returns") {
  let begin = () => {}
  const started = new Promise<void>((resolve) => (begin = resolve))
  const plugin: Plugin = {
    name: "slow-tool",
    setup(host) {
      host.tools.add({
        name: "wait",
        description: "Waits.",
        inputSchema: { type: "object" },
        run: (_input, context) =>
          new Promise((resolve, reject) => {
            begin()
            context.signal.addEventListener(
              "abort",
              () => (onCancel === "returns" ? resolve("stopped waiting") : reject(new Error("aborted"))),
              { once: true },
            )
          }),
      })
    },
  }
  return { plugin, started }
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
  const slow = slowTool()
  const { run } = await start(
    [{ text: ["Working."], calls: [{ name: "wait", input: {} }, { name: "wait", input: {} }] }],
    [slow.plugin],
  )

  await sayThenCancel(run, "go", slow.started)

  const results = stored(run.events).flatMap((event) => (event.type === "tool_result" ? [event.body] : []))
  expect(results).toEqual([
    { callId: "call_1_0", name: "wait", text: "stopped waiting" },
    { callId: "call_1_1", name: "wait", text: "Cancelled: the turn was stopped before this tool ran." },
  ])
  expect(stored(run.events).at(-1)).toMatchObject({ type: "turn_end", body: { reason: "cancelled" } })
})

test("after a cancel the conversation carries on, with a result for every tool call", async () => {
  const slow = slowTool()
  const { run, model } = await start(
    [{ text: ["Working."], calls: [{ name: "wait", input: {} }, { name: "wait", input: {} }] }, { text: ["Carrying on."] }],
    [slow.plugin],
  )

  await sayThenCancel(run, "go", slow.started)
  await say(run, "what happened?")

  const asked = model.requests[1]!.messages
  const callIds = asked.flatMap((message) => (message.role === "assistant" ? (message.toolCalls ?? []).map((call) => call.id) : []))
  const resultIds = asked.flatMap((message) => (message.role === "tool" ? [message.callId] : []))
  expect(resultIds).toEqual(callIds)
  expect(callIds).toHaveLength(2)
  expect(stored(run.events).at(-1)).toMatchObject({ type: "turn_end", body: { reason: "completed" } })
})

test("the model is shown a cancelled reply with a note that the owner cancelled it", async () => {
  const { run, model } = await start([{ hang: true, text: ["Half an ans"] }, { text: ["The rest."] }, { text: ["More."] }])

  await sayThenCancel(run, "explain", (event) => event.type === "assistant_delta")
  await say(run, "go on")

  expect(model.requests[1]?.messages).toEqual([
    { role: "user", text: "explain" },
    { role: "assistant", text: "Half an ans\n\n[This reply was cut short: the owner cancelled it before it was finished.]" },
    { role: "user", text: "go on" },
  ])
  // The note is only in the request. What is stored is what was written.
  expect(stored(run.events).find((event) => event.type === "assistant_message")?.body).toEqual({ text: "Half an ans", interrupted: true })
  // The same log gives the same request, which keeps the prompt cache working.
  await say(run, "and again")
  expect(model.requests[2]?.messages.slice(0, 3)).toEqual(model.requests[1]!.messages)
})

test("cancelling stops a running shell command", async () => {
  const { run } = await start([{ text: [], calls: [{ name: "shell", input: { command: "touch started; sleep 30" } }] }], [shell()])
  const started = Date.now()
  const commandRunning = (async () => {
    while (!(await Bun.file(join(run.projectFolder, "started")).exists())) await new Promise((resolve) => setTimeout(resolve, 5))
  })()

  await sayThenCancel(run, "go", commandRunning)

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

test("a turn that was steered can still be cancelled, and the conversation carries on", async () => {
  const { run, model } = await start([{ hang: true, text: ["Half an ans"] }, { hang: true }, { text: ["Third answer."] }])
  const send = (text: string) => run.evie.send({ type: "send_message", session: run.session, text })

  const firstText = next(run, (event) => event.type === "assistant_delta")
  const ended = next(run, (event) => event.type === "turn_end")
  await send("first")
  await firstText
  await send("second")
  while (model.requests.length < 2) await new Promise((resolve) => setTimeout(resolve, 1))
  await run.evie.send({ type: "cancel", session: run.session })
  await ended
  await say(run, "third")

  expect(stored(run.events).filter((event) => event.type === "turn_end").map((event) => event.body.reason)).toEqual(["cancelled", "completed"])
  expect(model.requests[2]?.messages).toEqual([
    { role: "user", text: "first" },
    // Cut short by the second message, not by the cancel that came later.
    { role: "assistant", text: "Half an ans\n\n[This reply was cut short: the owner sent a new message before it was finished.]" },
    { role: "user", text: "second" },
    { role: "user", text: "third" },
  ])
})
