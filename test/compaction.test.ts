import { afterEach, expect, test } from "bun:test"
import { mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { runTerminal, type EvieEvent, type Message, type Plugin } from "../src/index.ts"
import { say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// A message of about `tokens` tokens: sizes are estimated at four characters a token.
const long = (label: string, tokens: number) => `${label} ${"x".repeat(tokens * 4 - label.length - 1)}`
const used = (inputTokens: number) => ({ inputTokens, outputTokens: 10 })
const SUMMARY = "## Goal\nBuild the thing.\n\n## Next step\nKeep going."

// A tool that takes a path, as the file tools do.
const pathTool: Plugin = {
  name: "path-tool",
  setup(host) {
    host.tools.add({
      name: "look",
      description: "Looks at a file.",
      inputSchema: { type: "object", properties: { path: { type: "string" } } },
      run: async (input) => `looked at ${input.path}`,
    })
  },
}

// Starts Evie so that a conversation is compacted at 1,000 tokens, which
// keeps about the last 150 word for word.
async function start(
  script: ScriptedReply[],
  options: { limit?: number; lookUpLimit?: () => Promise<number | undefined>; ceiling?: number; home?: string; projectFolder?: string } = {},
) {
  const model = scriptedProvider(script, {
    ...(options.limit !== undefined && { limit: options.limit }),
    ...(options.lookUpLimit && { lookUpLimit: options.lookUpLimit }),
  })
  const run = await startEvie([model.plugin, pathTool], {
    compaction: { ceiling: options.ceiling ?? 1000 },
    ...(options.home && { home: options.home }),
    ...(options.projectFolder && { projectFolder: options.projectFolder }),
  })
  cleanups.push(run.stop)
  const compactions = () => stored(run.events).filter((event) => event.type === "compaction")
  return { run, model, compactions }
}

// Waits until the model has been sent this many requests.
async function requestsReach(model: { requests: unknown[] }, count: number) {
  while (model.requests.length < count) await Bun.sleep(1)
}
const turnEnds = (run: { events: EvieEvent[] }) => stored(run.events).filter((event) => event.type === "turn_end")

const isSummary = (message: Message | undefined) => message?.role === "user" && message.text.startsWith("[The earlier part of this conversation was replaced")

test("a conversation that has grown to the limit is compacted before the next request", async () => {
  const { run, model, compactions } = await start([
    { text: ["A1"], usage: used(400) },
    { text: ["A2"], usage: used(990) },
    { text: [SUMMARY] },
    { text: ["A3"] },
  ])

  await say(run, long("first", 100))
  await say(run, long("second", 100))
  await say(run, long("third", 100))

  // The summary request: the part being summarised, as the model saw it, then the instruction.
  const summaryRequest = model.requests[2]!.messages
  expect(summaryRequest.slice(0, 2)).toEqual([
    { role: "user", text: long("first", 100) },
    { role: "assistant", text: "A1" },
  ])
  expect(summaryRequest).toHaveLength(3)
  expect(summaryRequest[2]?.text).toStartWith("Stop working on the task. Write a summary of the conversation above")

  // The request that follows: the summary, then the recent part word for word.
  const after = model.requests[3]!.messages
  expect(isSummary(after[0])).toBe(true)
  expect(after[0]?.text).toContain(SUMMARY)
  expect(after.slice(1)).toEqual([
    { role: "user", text: long("second", 100) },
    { role: "assistant", text: "A2" },
    { role: "user", text: long("third", 100) },
  ])
  expect(compactions().map((event) => event.body)).toEqual([{ summary: SUMMARY, keepFrom: 6, files: [] }])
})

test("a conversation below the limit is left alone", async () => {
  const { run, model, compactions } = await start([{ text: ["A1"], usage: used(400) }, { text: ["A2"] }])

  await say(run, long("first", 100))
  await say(run, long("second", 100))

  expect(compactions()).toEqual([])
  expect(model.requests).toHaveLength(2)
})

test("nothing is deleted: the summary and the kept part are what a reopened session sends", async () => {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  const projectFolder = await realpath(await mkdtemp(join(tmpdir(), "evie-project-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }), () => rm(projectFolder, { recursive: true, force: true }))
  const first = await start([{ text: ["A1"], usage: used(400) }, { text: ["A2"], usage: used(990) }, { text: [SUMMARY] }, { text: ["A3"] }], { home, projectFolder })
  await say(first.run, long("first", 100))
  await say(first.run, long("second", 100))
  await say(first.run, long("third", 100))
  await first.run.evie.close()

  const second = await start([{ text: ["A4"] }], { home, projectFolder })
  const reopened = await second.run.evie.send({ type: "open_session", folder: projectFolder, continue: true })
  await say(second.run, "fourth", reopened.session)

  const sent = second.model.requests[0]!.messages
  expect(isSummary(sent[0])).toBe(true)
  expect(sent.slice(1).map((message) => message.text)).toEqual([long("second", 100), "A2", long("third", 100), "A3", "fourth"])
})

test("the cut never falls between a tool call and its result", async () => {
  const { run, model, compactions } = await start([
    { text: ["Looking."], calls: [{ name: "look", input: { path: "src/big.ts" } }] },
    { text: ["A1"], usage: used(990) },
    { text: [SUMMARY] },
    { text: ["A2"] },
  ])
  // The tool's result is the large thing here, so the 150 tokens to keep are used up inside it.
  const big: Plugin = {
    name: "big-result",
    setup(host) {
      host.on("after_tool", () => long("result", 200))
    },
  }
  const withBig = await startEvie([model.plugin, pathTool, big], { compaction: { ceiling: 1000 } })
  cleanups.push(withBig.stop)
  void run
  void compactions

  await say(withBig, "start")
  await say(withBig, "carry on")

  const after = model.requests[3]!.messages
  expect(isSummary(after[0])).toBe(true)
  // The reply that asked for the tool is kept with the result, not left behind in the summary.
  expect(after.slice(1).map((message) => message.role)).toEqual(["assistant", "tool", "assistant", "user"])
  expect(after[1]).toMatchObject({ role: "assistant", toolCalls: [{ name: "look" }] })
})

test("the files the summarised part worked with are listed after the summary", async () => {
  const { run, model, compactions } = await start([
    { text: [], calls: [{ name: "look", input: { path: "src/a.ts" } }, { name: "look", input: { path: "src/b.ts" } }] },
    { text: ["A1"], usage: used(400) },
    { text: ["A2"], usage: used(990) },
    { text: [SUMMARY] },
    { text: ["A3"] },
  ])

  await say(run, long("first", 100))
  await say(run, long("second", 100))
  await say(run, long("third", 100))

  expect(compactions()[0]?.body.files).toEqual(["src/a.ts", "src/b.ts"])
  expect(model.requests.at(-1)!.messages[0]?.text).toEndWith("Files worked with before this point:\n- src/a.ts\n- src/b.ts")
})

test("a second compaction updates the first summary and keeps its files", async () => {
  const { run, model, compactions } = await start([
    { text: [], calls: [{ name: "look", input: { path: "src/a.ts" } }] },
    { text: ["A1"], usage: used(400) },
    { text: [], calls: [{ name: "look", input: { path: "src/c.ts" } }] },
    { text: ["A2"], usage: used(990) },
    { text: [SUMMARY] },
    { text: ["A3"], usage: used(990) },
    { text: ["## Goal\nBuild the thing, updated."] },
    { text: ["A4"] },
  ])

  await say(run, long("first", 100))
  await say(run, long("second", 100))
  await say(run, long("third", 100))
  await say(run, long("fourth", 100))

  expect(compactions()).toHaveLength(2)
  // The second summary request starts with the first summary, so the model can update it.
  expect(isSummary(model.requests[6]!.messages[0])).toBe(true)
  expect(compactions()[1]?.body.summary).toBe("## Goal\nBuild the thing, updated.")
  expect(compactions()[1]?.body.files).toEqual(["src/a.ts", "src/c.ts"])
  // Afterwards there is one summary in the request, not two.
  expect(model.requests[7]!.messages.filter(isSummary)).toHaveLength(1)
})

for (const [what, bad] of [
  ["is empty", { text: [] }],
  ["was cut off", { text: ["## Goal\nBuild the"], cutOff: true }],
  ["tries to carry on working", { text: ["Let me look."], calls: [{ name: "look", input: { path: "x" } }] }],
] as [string, ScriptedReply][]) {
  test(`a summary that ${what} is thrown away, and the turn carries on with the whole conversation`, async () => {
    const { run, model, compactions } = await start([{ text: ["A1"], usage: used(400) }, { text: ["A2"], usage: used(990) }, bad, { text: ["A3"] }])

    await say(run, long("first", 100))
    await say(run, long("second", 100))
    await say(run, long("third", 100))

    expect(compactions()).toEqual([])
    expect(model.requests[3]!.messages.map((message) => message.role)).toEqual(["user", "assistant", "user", "assistant", "user"])
    expect(stored(run.events).at(-1)).toMatchObject({ type: "turn_end", body: { reason: "completed" } })
  })
}

test("a request refused as too long is compacted and sent once more", async () => {
  const { run, model, compactions } = await start([
    { text: ["A1"] },
    { text: ["A2"] },
    { text: [], error: "This endpoint's maximum context length is 1000 tokens.", tooLong: true },
    { text: [SUMMARY] },
    { text: ["A3"] },
  ])

  await say(run, long("first", 100))
  await say(run, long("second", 100))
  await say(run, long("third", 100))

  expect(compactions()).toHaveLength(1)
  expect(isSummary(model.requests[4]!.messages[0])).toBe(true)
  expect(stored(run.events).at(-1)).toMatchObject({ type: "turn_end", body: { reason: "completed" } })
})

test("/compact compacts on request, and says when there is too little to compact", async () => {
  const { run, model, compactions } = await start([{ text: ["A1"] }, { text: ["A2"] }, { text: [SUMMARY] }, { text: ["A3"] }])
  const compact = () => run.evie.send({ type: "run_command", session: run.session, name: "compact", args: "" })

  await say(run, "hi")
  expect((await compact()).text).toBe("There is not enough conversation to compact yet.")

  await say(run, long("second", 200))
  expect((await compact()).text).toBe("Compacted: the earlier part of the conversation is now a summary.")
  await say(run, "third")

  expect(compactions()).toHaveLength(1)
  expect(isSummary(model.requests.at(-1)!.messages[0])).toBe(true)
})

test("the size of each request and reply is recorded with the reply", async () => {
  const { run } = await start([{ text: ["A1"], usage: { inputTokens: 321, outputTokens: 12 } }])

  await say(run, "hi")

  expect(stored(run.events).find((event) => event.type === "assistant_message")).toMatchObject({
    body: { usage: { inputTokens: 321, outputTokens: 12 } },
  })
})

test("a model's own limit is used when it is lower than the ceiling: compaction comes at 80% of it", async () => {
  // The model reads 1,000 tokens, so it is compacted at 800, well below the ceiling.
  const under = await start([{ text: ["A1"], usage: used(500) }, { text: ["A2"] }], { limit: 1000, ceiling: 200_000 })
  await say(under.run, long("first", 100))
  await say(under.run, long("second", 100))
  expect(under.compactions()).toEqual([])

  const over = await start([{ text: ["A1"], usage: used(400) }, { text: ["A2"], usage: used(790) }, { text: [SUMMARY] }, { text: ["A3"] }], { limit: 1000, ceiling: 200_000 })
  await say(over.run, long("first", 100))
  await say(over.run, long("second", 100))
  await say(over.run, long("third", 100))
  expect(over.compactions()).toHaveLength(1)
})

test("the terminal says when the conversation has been compacted", async () => {
  const { run } = await start([{ text: ["A1"], usage: used(400) }, { text: ["A2"], usage: used(990) }, { text: [SUMMARY] }, { text: ["A3"] }])
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield long("first", 100)
      await new Promise((resolve) => setTimeout(resolve, 20))
      yield long("second", 100)
      await new Promise((resolve) => setTimeout(resolve, 20))
      yield long("third", 100)
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("[The conversation was getting long, so its earlier part is now a summary.]")
  expect(printed).not.toContain("## Goal")
})

test("/compact can be cancelled, and nothing is changed", async () => {
  const { run, model, compactions } = await start([{ text: ["A1"] }, { text: ["A2"] }, { hang: true }, { text: ["A3"] }])
  await say(run, long("first", 200))
  await say(run, long("second", 200))

  const compacting = run.evie.send({ type: "run_command", session: run.session, name: "compact", args: "" })
  await requestsReach(model, 3)
  await run.evie.send({ type: "cancel", session: run.session })

  expect((await compacting).text).toBe("Compaction was cancelled, so nothing was changed.")
  expect(compactions()).toEqual([])
  // The session carries on as before.
  await say(run, "third")
  expect(turnEnds(run).at(-1)).toMatchObject({ body: { reason: "completed" } })
})

test("closing Evie during /compact stops the summary and waits for it", async () => {
  const { run, model, compactions } = await start([{ text: ["A1"] }, { text: ["A2"] }, { hang: true }])
  await say(run, long("first", 200))
  await say(run, long("second", 200))

  let answer: string | undefined
  const compacting = run.evie.send({ type: "run_command", session: run.session, name: "compact", args: "" }).then((result) => (answer = result.text))
  await requestsReach(model, 3)
  await run.evie.close()

  // By the time Evie has closed, the compaction is over: nothing is left to write to the closed database.
  expect(answer).toBe("Compaction was cancelled, so nothing was changed.")
  expect(compactions()).toEqual([])
  await compacting
})

test("a turn cancelled while it compacts after a too-long request ends as cancelled", async () => {
  const { run, model, compactions } = await start([
    { text: ["A1"] },
    { text: ["A2"] },
    { text: [], error: "This endpoint's maximum context length is 1000 tokens.", tooLong: true },
    { hang: true },
  ])
  await say(run, long("first", 100))
  await say(run, long("second", 100))

  const ended = new Promise<void>((resolve) => run.evie.subscribe((event) => event.type === "turn_end" && resolve()))
  await run.evie.send({ type: "send_message", session: run.session, text: long("third", 100) })
  await requestsReach(model, 4)
  await run.evie.send({ type: "cancel", session: run.session })
  await ended

  expect(turnEnds(run).at(-1)).toEqual(expect.objectContaining({ body: { reason: "cancelled" } }))
  expect(compactions()).toEqual([])
})

test("when the provider reports no sizes, the conversation is measured by estimate and still compacted", async () => {
  const { run, compactions } = await start([{ text: ["A1"] }, { text: ["A2"] }, { text: [SUMMARY] }, { text: ["A3"] }])

  await say(run, long("first", 400))
  await say(run, long("second", 400))
  await say(run, long("third", 400))

  expect(stored(run.events).find((event) => event.type === "assistant_message")?.body).not.toHaveProperty("usage")
  expect(compactions()).toHaveLength(1)
})

test("a very small limit does not break the turn: the newest message is always kept", async () => {
  const { run, model, compactions } = await start([{ text: ["A1"] }, { text: [SUMMARY] }, { text: ["A2"] }], { ceiling: 2 })

  await say(run, "hi")
  await say(run, "hello again")

  expect(compactions()).toHaveLength(1)
  expect(model.requests[2]!.messages.at(-1)).toEqual({ role: "user", text: "hello again" })
  expect(turnEnds(run).at(-1)).toMatchObject({ body: { reason: "completed" } })
})

test("a turn can be cancelled while the model's limit is being looked up", async () => {
  const { run } = await start([{ text: ["A1"] }], { lookUpLimit: () => new Promise(() => {}) })

  const ended = new Promise<void>((resolve) => run.evie.subscribe((event) => event.type === "turn_end" && resolve()))
  await run.evie.send({ type: "send_message", session: run.session, text: "hi" })
  await Bun.sleep(5)
  await run.evie.send({ type: "cancel", session: run.session })
  await ended

  expect(turnEnds(run).at(-1)).toMatchObject({ body: { reason: "cancelled" } })
})

test("a limit that could not be looked up is looked up again later", async () => {
  let lookups = 0
  const lookUpLimit = async () => {
    if (++lookups === 1) throw new Error("the list could not be fetched")
    return 1000
  }
  const { run, compactions } = await start(
    [{ text: ["A1"], usage: used(400) }, { text: ["A2"], usage: used(790) }, { text: [SUMMARY] }, { text: ["A3"] }],
    { lookUpLimit, ceiling: 200_000 },
  )

  await say(run, long("first", 100))
  await say(run, long("second", 100))
  await say(run, long("third", 100))

  expect(compactions()).toHaveLength(1)
})

test("a client can read how full the conversation is, as the loop measures it, and where it is compacted, with the model in use", async () => {
  // The model reads 1,000 tokens, so it is compacted at 800, below the ceiling.
  const { run } = await start([{ text: ["A1"], usage: { inputTokens: 300, outputTokens: 20 } }], { limit: 1000, ceiling: 200_000 })
  const read = () => run.evie.send({ type: "read_context", session: run.session })

  expect(await read()).toEqual({ model: "test/model", size: 0, compactsAt: 800 })
  await say(run, "hi")
  expect(await read()).toEqual({ model: "test/model", size: 320, compactsAt: 800 })
  // Switched to another model, the conversation is the same size, and the model is the new one.
  await run.evie.send({ type: "set_model", session: run.session, model: "other/model" })
  expect(await read()).toEqual({ model: "other/model", size: 320, compactsAt: 800 })
})

test("reading how full the conversation is needs the session open, and with a model whose limit is not known, the ceiling decides", async () => {
  const { run } = await start([], { ceiling: 5000 })

  expect(await run.evie.send({ type: "read_context", session: run.session })).toEqual({ model: "test/model", size: 0, compactsAt: 5000 })
  expect(run.evie.send({ type: "read_context", session: "no-such-session" })).rejects.toThrow('The session "no-such-session" is not open.')
})

test("reading how full the conversation is does not wait on a limit lookup that never answers: past two seconds, the ceiling decides for this answer", async () => {
  const { run } = await start([], { ceiling: 5000, lookUpLimit: () => new Promise(() => {}) })

  const asked = Date.now()
  expect(await run.evie.send({ type: "read_context", session: run.session })).toEqual({ model: "test/model", size: 0, compactsAt: 5000 })
  expect(Date.now() - asked).toBeLessThan(3000)
})

test("with no session, a client reads what a new session would start with: the model, in a project that sets its own once a session has opened there, an empty conversation, and where it is compacted", async () => {
  const model = scriptedProvider([], { limit: 1000 })
  const run = await startEvie([model.plugin], { compaction: { ceiling: 200_000 }, projectSettings: async () => ({ plugins: [], model: "project/model" }) })
  cleanups.push(run.stop)

  expect(await run.evie.send({ type: "read_context" })).toEqual({ model: "test/model", size: 0, compactsAt: 800 })
  expect(await run.evie.send({ type: "read_context", project: run.project!.id })).toEqual({ model: "project/model", size: 0, compactsAt: 800 })
})
