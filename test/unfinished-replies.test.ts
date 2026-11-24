import { afterEach, expect, test } from "bun:test"
import type { Plugin } from "../src/index.ts"
import { say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

// A reply that did not finish is never taken for the final answer: one cut
// off at the length limit, or one whose tool call arrived as text. The model
// is told and asked again in the same turn, and three in a row end the turn
// as a failure that says why.

let stop: (() => Promise<void>) | undefined
afterEach(async () => {
  await stop?.()
  stop = undefined
})

// One made-up tool that records each time it runs.
function echoTool() {
  const ran: Record<string, unknown>[] = []
  const plugin: Plugin = {
    name: "test-tools",
    setup(host) {
      host.tools.add({
        name: "echo",
        description: "Says something back.",
        inputSchema: { type: "object", properties: { text: { type: "string" } } },
        run: async (input) => {
          ran.push(input)
          return `You said: ${input.text}`
        },
      })
    },
  }
  return { plugin, ran }
}

async function start(script: ScriptedReply[]) {
  const model = scriptedProvider(script)
  const tool = echoTool()
  const run = await startEvie([model.plugin, tool.plugin])
  stop = run.stop
  return { run, model, tool }
}

const types = (run: { events: Parameters<typeof stored>[0] }) => stored(run.events).map((event) => event.type)
const turnEnd = (run: { events: Parameters<typeof stored>[0] }) => stored(run.events).findLast((event) => event.type === "turn_end")

test("a reply cut off at the length limit is not the final answer: the model is told and asked again in the same turn", async () => {
  const { run, model } = await start([{ text: ["Here is the whole file in one go: "], cutOff: true }, { text: ["Done, in smaller pieces."] }])

  await say(run, "write the file")

  expect(types(run)).toEqual(["turn_start", "user_message", "assistant_message", "note", "assistant_message", "turn_end"])
  expect(turnEnd(run)?.body).toEqual({ reason: "completed" })
  const asked = model.requests[1]?.messages ?? []
  expect(asked.slice(0, 2)).toEqual([
    { role: "user", text: "write the file" },
    { role: "assistant", text: "Here is the whole file in one go: " },
  ])
  expect(asked[2]?.role).toBe("user")
  expect(asked[2]?.text).toContain("cut off")
  expect(asked[2]?.text).toContain("smaller pieces")
})

test("the tool calls of a cut-off reply are not run, since their input may be cut short: each gets a result asking for it again", async () => {
  const { run, model, tool } = await start([
    { text: ["Writing it."], calls: [{ name: "echo", input: { text: "the whole fi" } }], cutOff: true },
    { text: ["Done."] },
  ])

  await say(run, "write the file")

  expect(tool.ran).toEqual([])
  expect(types(run)).toEqual(["turn_start", "user_message", "assistant_message", "tool_result", "note", "assistant_message", "turn_end"])
  const asked = model.requests[1]?.messages ?? []
  expect(asked[2]).toMatchObject({ role: "tool", callId: "call_1_0" })
  expect(asked[2]?.text).toContain("Not run")
  expect(asked[2]?.text).toContain("send it again")
  expect(asked[3]?.text).toContain("cut off")
})

test("three cut-off replies in a row end the turn as a failure that says the limit was reached", async () => {
  const cut: ScriptedReply = { text: ["Still going "], cutOff: true }
  const { run, model } = await start([cut, cut, cut])

  await say(run, "write the file")

  expect(model.requests).toHaveLength(3)
  const end = turnEnd(run)?.body
  expect(end).toMatchObject({ reason: "error" })
  expect(end?.error).toContain("length limit")
  expect(end?.error).toContain("maxTokens")
})

test("only replies in a row count: a finished reply between cut-off ones starts the count again", async () => {
  const cut: ScriptedReply = { text: ["Still going "], cutOff: true }
  const { run, tool } = await start([cut, cut, { text: [], calls: [{ name: "echo", input: { text: "part one" } }] }, cut, cut, { text: ["Done."] }])

  await say(run, "write the file")

  expect(tool.ran).toEqual([{ text: "part one" }])
  expect(turnEnd(run)?.body).toEqual({ reason: "completed" })
})

test("a reply whose tool call arrived as text is not the final answer: the model is told it did not run and asked again", async () => {
  const garbled = "<tool_call>echo<arg_key>text</arg_key><arg_value>hi</arg_value></tool_call>"
  const { run, model, tool } = await start([
    { text: [garbled], callAsText: true },
    { text: [], calls: [{ name: "echo", input: { text: "hi" } }] },
    { text: ["Done."] },
  ])

  await say(run, "say hi")

  expect(tool.ran).toEqual([{ text: "hi" }])
  expect(types(run)).toEqual(["turn_start", "user_message", "assistant_message", "note", "assistant_message", "tool_result", "assistant_message", "turn_end"])
  expect(turnEnd(run)?.body).toEqual({ reason: "completed" })
  const told = model.requests[1]?.messages.at(-1)
  expect(told?.role).toBe("user")
  expect(told?.text).toContain("as text")
  expect(told?.text).toContain("did not run")
})

test("three replies in a row with a tool call as text end the turn as a failure that says so", async () => {
  const garbled: ScriptedReply = { text: ["<tool_call>echo</tool_call>"], callAsText: true }
  const { run, tool } = await start([garbled, garbled, garbled])

  await say(run, "say hi")

  expect(tool.ran).toEqual([])
  const end = turnEnd(run)?.body
  expect(end).toMatchObject({ reason: "error" })
  expect(end?.error).toContain("as text")
})
