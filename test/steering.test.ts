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
