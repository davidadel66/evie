import { afterEach, expect, test } from "bun:test"
import { chmod, mkdir, mkdtemp, readdir, readFile, realpath, rm, utimes, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { deleteOldOutput, keepOutputTidy } from "../src/core/tools.ts"
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

// 100 lines, the first and last of 1,024 bytes and the rest of 1,023, so the
// first 50 lines with the 49 breaks between them are exactly 50 KiB, and so
// are the last 50: either way the cut falls on a line boundary.
const boundaryLine = (n: number) => `${n} `.padEnd(n === 1 || n === 100 ? 1024 : 1023, "x")
const onBoundary = Array.from({ length: 100 }, (_, index) => boundaryLine(index + 1)).join("\n")
// Each line shown as its number when it is whole, or as itself when not.
const lineNumbers = (shown: string) =>
  shown.split("\n").map((line) => {
    const n = Number.parseInt(line)
    return line === boundaryLine(n) ? n : line
  })
const numbers = (from: number, to: number) => Array.from({ length: to - from + 1 }, (_, index) => from + index)

test("output over 50 KiB whose cut falls at the end of a line keeps that line", async () => {
  const { run } = await start([{ text: [], calls: [{ name: "echo", input: {} }] }, { text: ["ok"] }], {
    run: async () => onBoundary,
  })

  await say(run, "go")

  const [result] = results(run)
  const shown = result!.split("\n\n[Output cut")[0]!
  expect(lineNumbers(shown)).toEqual(numbers(1, 50))
  expect(result).toContain("[Output cut: showing the first 50 of 100 lines.")
})

test("for a tool that keeps the end, output over 50 KiB whose cut falls at the start of a line keeps that line", async () => {
  const { run } = await start([{ text: [], calls: [{ name: "echo", input: {} }] }, { text: ["ok"] }], {
    longOutput: "end",
    run: async () => onBoundary,
  })

  await say(run, "go")

  const [result] = results(run)
  const shown = result!.split("\n\n[Output cut")[0]!
  expect(lineNumbers(shown)).toEqual(numbers(51, 100))
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

// ---- Saved output is kept for a week ----------------------------------------------

const DAY = 24 * 60 * 60 * 1000

// Marks a file or folder as last changed the given number of days ago.
async function age(path: string, days: number) {
  const when = new Date(Date.now() - days * DAY)
  await utimes(path, when, when)
}

// A folder of saved output holding the given files, each last changed the
// given number of days ago. Each folder inside last changed when its newest
// file was added, as it would have.
async function savedOutput(files: Record<string, number>) {
  const folder = await mkdtemp(join(tmpdir(), "evie-tool-output-"))
  const after = stop
  stop = async () => {
    await after?.()
    await rm(folder, { recursive: true, force: true })
  }
  const newest = new Map<string, number>()
  for (const [name, days] of Object.entries(files)) {
    const path = join(folder, name)
    const inside = join(path, "..")
    await mkdir(inside, { recursive: true })
    await writeFile(path, "output")
    await age(path, days)
    newest.set(inside, Math.min(newest.get(inside) ?? days, days))
  }
  for (const [inside, days] of newest) if (inside !== folder) await age(inside, days)
  return folder
}

const left = async (folder: string) => (await readdir(folder, { recursive: true })).sort()

test("saved output older than seven days is deleted, and so is a folder left empty, while newer output is kept", async () => {
  const folder = await savedOutput({
    "old-session/call_1.txt": 8,
    "old-session/call_2.txt": 30,
    "mixed-session/call_1.txt": 8,
    "mixed-session/call_2.txt": 6,
    "mixed-session/job-1.log": 0,
  })

  await deleteOldOutput(folder)

  expect(await left(folder)).toEqual(["mixed-session", "mixed-session/call_2.txt", "mixed-session/job-1.log"])
})

test("a background job's files are kept however old, and so is their folder, while other old output beside them is deleted", async () => {
  // A session's jobs are numbered from their files, so a deleted one would
  // let a later job take a number the conversation already uses.
  const folder = await savedOutput({ "a-session/job-1.log": 30, "a-session/job-1.json": 30, "a-session/job-1.exit": 30, "a-session/call_1.txt": 30 })

  await deleteOldOutput(folder)

  expect(await left(folder)).toEqual(["a-session", "a-session/job-1.exit", "a-session/job-1.json", "a-session/job-1.log"])
})

test("a folder that changed in the last week is kept even when empty, as output may be about to be saved in it", async () => {
  const folder = await savedOutput({ "old-session/call_1.txt": 8 })
  // Made a moment ago, for a file not yet written.
  await mkdir(join(folder, "new-session"))
  // Left empty a while ago.
  await mkdir(join(folder, "empty-session"))
  await age(join(folder, "empty-session"), 8)

  await deleteOldOutput(folder)

  expect(await left(folder)).toEqual(["new-session"])
})

test("output kept tidy is tidied again each time the interval comes round, until that stops", async () => {
  const folder = await savedOutput({ "call_1.txt": 8 })
  const oldFile = async (name: string) => {
    await writeFile(join(folder, name), "output")
    await age(join(folder, name), 8)
  }

  const stopTidying = keepOutputTidy(folder, { every: 20 })
  try {
    // Output that has grown old while the server runs, twice over, each
    // added once the tidying before has deleted what was there.
    for (const name of ["call_2.txt", "call_3.txt"]) {
      while ((await left(folder)).length > 0) await Bun.sleep(1)
      await oldFile(name)
    }
    while ((await left(folder)).length > 0) await Bun.sleep(1)
  } finally {
    stopTidying()
  }
  // Any tidying under way when it stopped has ended by now.
  await Bun.sleep(50)
  await oldFile("call_4.txt")
  await Bun.sleep(100)

  expect(await left(folder)).toEqual(["call_4.txt"])
})

// Root can delete from a folder it may not write to, so there the file could
// not be kept from being deleted.
test.skipIf(process.getuid?.() === 0)("a missing folder, or a file that cannot be deleted, does not stop the rest being tidied", async () => {
  const folder = await savedOutput({ "locked/call_1.txt": 8, "open/call_1.txt": 8 })
  // A file is deleted from its folder, so this one's cannot be.
  await chmod(join(folder, "locked"), 0o500)

  try {
    await deleteOldOutput(join(folder, "missing"))
    await deleteOldOutput(folder)
  } finally {
    await chmod(join(folder, "locked"), 0o700)
  }

  expect(await left(folder)).toEqual(["locked", "locked/call_1.txt"])
})
