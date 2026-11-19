import { afterEach, expect, test } from "bun:test"
import type { Plugin } from "../src/index.ts"
import { startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

let stop: (() => Promise<void>) | undefined
afterEach(async () => {
  await stop?.()
  stop = undefined
})

// A tool that waits until it is told to finish, so a test can send a message
// while a turn is in the middle of its work.
function pausingTool() {
  let begin = () => {}
  let finish = () => {}
  const started = new Promise<void>((resolve) => (begin = resolve))
  const released = new Promise<void>((resolve) => (finish = resolve))
  const plugin: Plugin = {
    name: "pausing-tool",
    setup(host) {
      host.tools.add({
        name: "work",
        description: "Does some work.",
        inputSchema: { type: "object" },
        run: async () => {
          begin()
          await released
          return "work done"
        },
      })
    },
  }
  return { plugin, started, finish }
}

async function start(script: ScriptedReply[]) {
  const model = scriptedProvider(script)
  const tool = pausingTool()
  const run = await startEvie([model.plugin, tool.plugin])
  stop = run.stop
  const ended = new Promise<void>((resolve) => {
    run.evie.subscribe((event) => {
      if (event.type === "turn_end") resolve()
    })
  })
  const send = (text: string) => run.evie.send({ type: "send_message", session: run.session, text })
  return { run, model, tool, ended, send }
}

test("a message sent while a turn is working is seen by the model at its next step, in the same turn", async () => {
  const { run, model, tool, ended, send } = await start([{ text: ["Starting."], calls: [{ name: "work", input: {} }] }, { text: ["Changed course."] }])

  await send("do the long thing")
  await tool.started
  await send("actually, stop after this step")
  tool.finish()
  await ended

  expect(stored(run.events).map((event) => event.type)).toEqual([
    "turn_start",
    "user_message",
    "assistant_message",
    "tool_result",
    "user_message",
    "assistant_message",
    "turn_end",
  ])
  expect(model.requests[1]?.messages).toEqual([
    { role: "user", text: "do the long thing" },
    { role: "assistant", text: "Starting.", toolCalls: [{ id: "call_1_0", name: "work", arguments: "{}" }] },
    { role: "tool", callId: "call_1_0", text: "work done" },
    { role: "user", text: "actually, stop after this step" },
  ])
})

test("several messages sent during a turn all arrive, in the order they were sent", async () => {
  const { model, tool, ended, send } = await start([{ text: [], calls: [{ name: "work", input: {} }] }, { text: ["ok"] }])

  await send("go")
  await tool.started
  await send("first correction")
  await send("second correction")
  tool.finish()
  await ended

  expect(model.requests[1]?.messages.slice(-2)).toEqual([
    { role: "user", text: "first correction" },
    { role: "user", text: "second correction" },
  ])
})

// ---- Steering a reply in progress -------------------------------------------------------

async function startReplying(script: ScriptedReply[], plugins: Plugin[] = []) {
  const model = scriptedProvider(script)
  const run = await startEvie([model.plugin, ...plugins])
  stop = run.stop
  const send = (text: string) => run.evie.send({ type: "send_message", session: run.session, text })
  const until = (matches: (type: string) => boolean) =>
    new Promise<void>((resolve) => {
      const off = run.evie.subscribe((event) => {
        if (matches(event.type)) {
          off()
          resolve()
        }
      })
    })
  return { run, model, send, until }
}

test("a message sent while the model is writing stops that reply, keeps what it wrote, and is answered in the same turn", async () => {
  const { run, model, send, until } = await startReplying([{ hang: true, text: ["Here is how", " the API"] }, { text: ["Only the events, then."] }])

  const writing = until((type) => type === "assistant_delta")
  const ended = until((type) => type === "turn_end")
  await send("tell me how the API works")
  await writing
  await send("focus only on the events")
  await ended

  expect(stored(run.events).map((event) => event.type)).toEqual([
    "turn_start",
    "user_message",
    "assistant_message",
    "user_message",
    "assistant_message",
    "turn_end",
  ])
  expect(stored(run.events)[2]).toMatchObject({ body: { interrupted: true } })
  expect(stored(run.events).at(-1)).toMatchObject({ body: { reason: "completed" } })
  const sent = model.requests[1]!.messages
  expect(sent[0]).toEqual({ role: "user", text: "tell me how the API works" })
  expect(sent[1]).toMatchObject({ role: "assistant" })
  expect("Here is how the API").toStartWith((sent[1] as { text: string }).text)
  expect(sent[2]).toEqual({ role: "user", text: "focus only on the events" })
})

test("a reply stopped before it said anything leaves no empty reply behind", async () => {
  const { run, model, send, until } = await startReplying([{ hang: true }, { text: ["ok"] }])

  const ended = until((type) => type === "turn_end")
  await send("first")
  while (model.requests.length < 1) await new Promise((resolve) => setTimeout(resolve, 1))
  await send("second")
  await ended

  expect(stored(run.events).filter((event) => event.type === "assistant_message")).toHaveLength(1)
  expect(model.requests[1]?.messages).toEqual([
    { role: "user", text: "first" },
    { role: "user", text: "second" },
  ])
})

test("a provider that reports the stopped reply as a failure does not end the turn", async () => {
  const { run, send, until } = await startReplying([{ hang: true, text: ["Half"], reports: true }, { text: ["ok"] }])

  const writing = until((type) => type === "assistant_delta")
  const ended = until((type) => type === "turn_end")
  await send("first")
  await writing
  await send("second")
  await ended

  expect(stored(run.events).at(-1)).toMatchObject({ type: "turn_end", body: { reason: "completed" } })
})

test("a message that arrives while a request is being put together is included in it", async () => {
  let reached = () => {}
  let release = () => {}
  const building = new Promise<void>((resolve) => (reached = resolve))
  const gate = new Promise<void>((resolve) => (release = resolve))
  const slowHook: Plugin = {
    name: "slow-hook",
    setup(host) {
      host.on("before_request", async () => {
        reached()
        await gate
      })
    },
  }
  const { model, send, until } = await startReplying([{ text: ["ok"] }], [slowHook])

  const ended = until((type) => type === "turn_end")
  await send("first")
  await building
  await send("second")
  release()
  await ended

  expect(model.requests).toHaveLength(1)
  expect(model.requests[0]?.messages).toEqual([
    { role: "user", text: "first" },
    { role: "user", text: "second" },
  ])
})
