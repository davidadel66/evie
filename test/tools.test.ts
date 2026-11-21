import { afterEach, expect, test } from "bun:test"
import { readFile, realpath } from "node:fs/promises"
import type { Plugin, Tool, ToolContext } from "../src/index.ts"
import { say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

let stop: (() => Promise<void>) | undefined
afterEach(async () => {
  await stop?.()
  stop = undefined
})

// A plugin with one made-up tool, which records what it was given.
function toolPlugin(tool: Partial<Tool> & { run: Tool["run"] }) {
  const calls: { input: Record<string, unknown>; context: ToolContext }[] = []
  const plugin: Plugin = {
    name: "test-tools",
    setup(host) {
      host.tools.add({
        name: "echo",
        description: "Says something back.",
        inputSchema: { type: "object", properties: { text: { type: "string" } } },
        ...tool,
        run: async (input, context) => {
          calls.push({ input, context })
          return tool.run(input, context)
        },
      })
    },
  }
  return { plugin, calls }
}

async function start(script: ScriptedReply[], tool: Parameters<typeof toolPlugin>[0]) {
  const model = scriptedProvider(script)
  const tools = toolPlugin(tool)
  const run = await startEvie([model.plugin, tools.plugin])
  stop = run.stop
  return { run, model, tools }
}

const results = (run: { events: Parameters<typeof stored>[0] }) =>
  stored(run.events).flatMap((event) => (event.type === "tool_result" ? [event.body.text] : []))

test("when the model asks for a tool it runs, and the model is asked again with the result", async () => {
  const { run, model } = await start(
    [{ text: ["Let me check."], calls: [{ name: "echo", input: { text: "hello" } }] }, { text: ["It said hello."] }],
    { run: async (input) => `You said: ${input.text}` },
  )

  await say(run, "try the tool")

  expect(stored(run.events).map((event) => event.type)).toEqual([
    "turn_start",
    "user_message",
    "assistant_message",
    "tool_result",
    "assistant_message",
    "turn_end",
  ])
  expect(model.requests[1]?.messages).toEqual([
    { role: "user", text: "try the tool" },
    {
      role: "assistant",
      text: "Let me check.",
      toolCalls: [{ id: "call_1_0", name: "echo", arguments: '{"text":"hello"}' }],
    },
    { role: "tool", callId: "call_1_0", text: "You said: hello" },
  ])
})

test("tool calls in one step run one at a time, in the order asked", async () => {
  const order: string[] = []
  const { run } = await start(
    [{ text: [], calls: [{ name: "echo", input: { text: "a" } }, { name: "echo", input: { text: "b" } }] }, { text: ["done"] }],
    {
      run: async (input) => {
        order.push(`start ${input.text}`)
        await new Promise((resolve) => setTimeout(resolve, 5))
        order.push(`end ${input.text}`)
        return "ok"
      },
    },
  )

  await say(run, "go")

  expect(order).toEqual(["start a", "end a", "start b", "end b"])
})

test("a tool is given the folder of the session's project", async () => {
  const { run, tools } = await start([{ text: [], calls: [{ name: "echo", input: {} }] }, { text: ["done"] }], {
    run: async () => "ok",
  })

  await say(run, "go")

  expect(tools.calls[0]?.context.folder).toBe(await realpath(run.projectFolder))
})

test("in the general session a tool is told there is no project folder", async () => {
  const { run, tools } = await start([{ text: [], calls: [{ name: "echo", input: {} }] }, { text: ["done"] }], {
    run: async () => "ok",
  })
  const general = await run.evie.send({ type: "open_session" })

  await say(run, "go", general.session)

  expect(tools.calls[0]?.context.folder).toBeNull()
})

test("asking for a tool that does not exist returns the names of the ones that do, and the turn goes on", async () => {
  const { run } = await start([{ text: [], calls: [{ name: "search", input: {} }] }, { text: ["Sorry."] }], {
    run: async () => "ok",
  })

  await say(run, "go")

  expect(results(run)).toEqual(['There is no tool named "search". The tools are: echo.'])
  expect(stored(run.events).at(-1)).toMatchObject({ type: "turn_end", body: { reason: "completed" } })
})

test("input that is not JSON is returned to the model to fix, and the tool does not run", async () => {
  const { run, tools } = await start([{ text: [], calls: [{ name: "echo", input: '{"text": "unfinished' }] }, { text: ["ok"] }], {
    run: async () => "ok",
  })

  await say(run, "go")

  expect(results(run)).toEqual(["The input for echo was not valid JSON. Send the call again with the input as one JSON object."])
  expect(tools.calls).toHaveLength(0)
})

test("a number or a yes-or-no sent as text is repaired before the tool runs", async () => {
  const { run, tools } = await start(
    [{ text: [], calls: [{ name: "echo", input: { count: "12", loud: "true", text: 7 } }] }, { text: ["ok"] }],
    {
      inputSchema: {
        type: "object",
        properties: { count: { type: "integer" }, loud: { type: "boolean" }, text: { type: "string" } },
      },
      run: async () => "ok",
    },
  )

  await say(run, "go")

  expect(tools.calls[0]?.input).toEqual({ count: 12, loud: true, text: "7" })
})

test("input that is missing something or has the wrong kind of value says what to fix", async () => {
  const { run, tools } = await start([{ text: [], calls: [{ name: "echo", input: { count: "many" } }] }, { text: ["ok"] }], {
    inputSchema: {
      type: "object",
      properties: { text: { type: "string" }, count: { type: "integer" } },
      required: ["text"],
    },
    run: async () => "ok",
  })

  await say(run, "go")

  expect(results(run)).toEqual([
    'The input for echo is not right: "text" is required; "count" must be a number. Send the call again with the input fixed.',
  ])
  expect(tools.calls).toHaveLength(0)
})

test("output over 2,000 lines is saved in full, and the model is shown the start and where the rest is", async () => {
  const long = Array.from({ length: 2500 }, (_, index) => `line ${index + 1}`).join("\n")
  const { run } = await start([{ text: [], calls: [{ name: "echo", input: {} }] }, { text: ["ok"] }], {
    run: async () => long,
  })

  await say(run, "go")

  const [result] = results(run)
  const note = /\[Output cut: showing the first 2000 of 2500 lines\. The full output is saved at (.+?)\. Read that file/.exec(result!)
  expect(result).toStartWith("line 1\nline 2\n")
  expect(result).toContain("line 2000\n")
  expect(result).not.toContain("line 2001")
  expect(note).not.toBeNull()
  expect(note![1]).toStartWith(`${run.home}/tool-output/`)
  expect(await readFile(note![1]!, "utf8")).toBe(long)
})

test("a tool that keeps the end of long output shows its last 2,000 lines, and the full output is saved", async () => {
  const long = Array.from({ length: 2500 }, (_, index) => `line ${index + 1}`).join("\n")
  const { run } = await start([{ text: [], calls: [{ name: "echo", input: {} }] }, { text: ["ok"] }], {
    longOutput: "end",
    run: async () => long,
  })

  await say(run, "go")

  const [result] = results(run)
  const note = /\n\n\[Output cut: showing the last 2000 of 2500 lines\. The full output is saved at (.+?)\. Read that file with offset and limit to see more\.\]$/.exec(result!)
  expect(result).toStartWith("line 501\nline 502\n")
  expect(result).toContain("line 2500\n\n[Output cut")
  expect(result).not.toContain("line 500\n")
  expect(note).not.toBeNull()
  expect(note![1]).toStartWith(`${run.home}/tool-output/`)
  expect(await readFile(note![1]!, "utf8")).toBe(long)
})

test("output over 50 KiB is cut at the end of a line", async () => {
  const wide = Array.from({ length: 100 }, (_, index) => `${index + 1} ${"x".repeat(1000)}`).join("\n")
  const { run } = await start([{ text: [], calls: [{ name: "echo", input: {} }] }, { text: ["ok"] }], {
    run: async () => wide,
  })

  await say(run, "go")

  const [result] = results(run)
  const shown = result!.split("\n\n[Output cut")[0]!
  expect(Buffer.byteLength(shown)).toBeLessThanOrEqual(50 * 1024)
  expect(shown.endsWith("x".repeat(1000))).toBe(true)
  expect(result).toContain("of 100 lines")
})

test("for a tool that keeps the end, output over 50 KiB is cut at the start of a line", async () => {
  // Line 100 is 1,004 bytes and each line before it 1,003 and a line break,
  // so the last 50 lines are 50,200 bytes and the last 51 are over the limit.
  const wide = Array.from({ length: 100 }, (_, index) => `${index + 1} ${"x".repeat(1000)}`).join("\n")
  const { run } = await start([{ text: [], calls: [{ name: "echo", input: {} }] }, { text: ["ok"] }], {
    longOutput: "end",
    run: async () => wide,
  })

  await say(run, "go")

  const [result] = results(run)
  const shown = result!.split("\n\n[Output cut")[0]!
  expect(shown.startsWith(`51 ${"x".repeat(1000)}\n52 `)).toBe(true)
  expect(shown.endsWith(`100 ${"x".repeat(1000)}`)).toBe(true)
  expect(result).toContain("[Output cut: showing the last 50 of 100 lines.")
})

test("for a tool that keeps the end, one line too long to show keeps its end, without splitting a character", async () => {
  // 80,003 bytes. The last 51,200 begin in the middle of an "é", which is
  // left out, so 51,199 are shown.
  const { run } = await start([{ text: [], calls: [{ name: "echo", input: {} }] }, { text: ["ok"] }], {
    longOutput: "end",
    run: async () => `${"é".repeat(40_000)}end`,
  })

  await say(run, "go")

  const [result] = results(run)
  expect(result).toStartWith("éé")
  expect(result).toContain("end\n\n[Output cut: showing the last 51199 of 80003 bytes.")
  expect(result).not.toContain("�")
})

test("a tool that breaks ends the turn, and every call still has a result so the conversation carries on", async () => {
  const { run, model } = await start(
    [
      { text: [], calls: [{ name: "echo", input: { text: "a" } }, { name: "echo", input: { text: "b" } }] },
      { text: ["Carrying on."] },
    ],
    {
      run: async () => {
        throw new Error("something inside went wrong")
      },
    },
  )

  await say(run, "go")
  await say(run, "what happened?")

  expect(stored(run.events).filter((event) => event.type === "turn_end")[0]).toMatchObject({
    body: { reason: "error", error: 'The tool "echo" broke: something inside went wrong' },
  })
  const asked = model.requests[1]!.messages
  const callIds = asked.flatMap((message) => (message.role === "assistant" ? (message.toolCalls ?? []).map((call) => call.id) : []))
  const resultIds = asked.flatMap((message) => (message.role === "tool" ? [message.callId] : []))
  expect(resultIds).toEqual(callIds)
  expect(callIds).toHaveLength(2)
})

test("one line too long to show is cut part way, and the note says so in bytes", async () => {
  const { run } = await start([{ text: [], calls: [{ name: "echo", input: {} }] }, { text: ["ok"] }], {
    run: async () => "é".repeat(40_000),
  })

  await say(run, "go")

  const [result] = results(run)
  expect(result).toContain("[Output cut: showing the first 51200 of 80000 bytes.")
  expect(result).not.toContain("�")
})
