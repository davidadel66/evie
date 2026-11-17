import { afterEach, expect, test } from "bun:test"
import { readdir } from "node:fs/promises"
import type { Plugin } from "../src/index.ts"
import { say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

let stop: (() => Promise<void>) | undefined
afterEach(async () => {
  await stop?.()
  stop = undefined
})

test("a message to an idle session is answered and the turn is recorded in order", async () => {
  const model = scriptedProvider([{ text: ["Hello", " there"] }])
  const run = await startEvie([model.plugin])
  stop = run.stop

  await say(run, "hi")

  expect(stored(run.events).map((event) => event.type)).toEqual([
    "turn_start",
    "user_message",
    "assistant_message",
    "turn_end",
  ])
  const [, user, assistant] = stored(run.events)
  expect(user).toMatchObject({ type: "user_message", body: { text: "hi" } })
  expect(assistant).toMatchObject({ type: "assistant_message", body: { text: "Hello there" } })
})

test("the reply streams to the client as deltas, and no delta is stored", async () => {
  const model = scriptedProvider([{ text: ["Hello", " there"] }])
  const run = await startEvie([model.plugin])
  stop = run.stop

  await say(run, "hi")

  const deltas = run.events.filter((event) => event.type === "assistant_delta")
  expect(deltas.map((delta) => delta.text)).toEqual(["Hello", " there"])
  // Stored events are numbered without a gap, so the deltas took no place in the log.
  expect(stored(run.events).map((event) => event.seq)).toEqual([1, 2, 3, 4])
  // The deltas arrive before the complete message they add up to.
  const types = run.events.map((event) => event.type)
  expect(types.lastIndexOf("assistant_delta")).toBeLessThan(types.indexOf("assistant_message"))
})

test("a second message starts a new turn whose request includes the earlier conversation", async () => {
  const model = scriptedProvider([{ text: ["Paris."] }, { text: ["About two million."] }])
  const run = await startEvie([model.plugin])
  stop = run.stop

  await say(run, "What is the capital of France?")
  await say(run, "How many people live there?")

  expect(model.requests[1]?.messages).toEqual([
    { role: "user", text: "What is the capital of France?" },
    { role: "assistant", text: "Paris." },
    { role: "user", text: "How many people live there?" },
  ])
  expect(stored(run.events).filter((event) => event.type === "turn_start")).toHaveLength(2)
})

test("a message sent while a turn is running is answered in a turn of its own", async () => {
  const model = scriptedProvider([{ text: ["one"] }, { text: ["two"] }])
  const run = await startEvie([model.plugin])
  stop = run.stop

  const bothEnded = new Promise<void>((resolve) => {
    let ended = 0
    run.evie.subscribe((event) => {
      if (event.type === "turn_end" && ++ended === 2) resolve()
    })
  })
  await run.evie.send({ type: "send_message", session: run.session, text: "first" })
  await run.evie.send({ type: "send_message", session: run.session, text: "second" })
  await bothEnded

  expect(model.requests[1]?.messages).toEqual([
    { role: "user", text: "first" },
    { role: "assistant", text: "one" },
    { role: "user", text: "second" },
  ])
})

test("the same conversation always builds the same request, byte for byte", async () => {
  const requests: string[] = []
  for (let attempt = 0; attempt < 2; attempt++) {
    const model = scriptedProvider([{ text: ["Paris."] }, { text: ["About two million."] }])
    const tools: Plugin = {
      name: "one-tool",
      setup(host) {
        host.tools.add({ name: "lookup", description: "Looks something up.", inputSchema: { type: "object" } })
      },
    }
    const run = await startEvie([model.plugin, tools])
    await say(run, "What is the capital of France?")
    await say(run, "How many people live there?")
    requests.push(JSON.stringify(model.requests))
    await run.stop()
  }
  expect(requests[0]).toBe(requests[1]!)
})

test("the model named in the settings is the one asked", async () => {
  const model = scriptedProvider([{ text: ["ok"] }])
  const run = await startEvie([model.plugin], { model: "some/model-name" })
  stop = run.stop

  await say(run, "hi")

  expect(model.requests[0]?.model).toBe("some/model-name")
})

test("a tool registered by a plugin is described to the model, in the order registered", async () => {
  const model = scriptedProvider([{ text: ["ok"] }])
  const tools: Plugin = {
    name: "two-tools",
    setup(host) {
      host.tools.add({ name: "zebra", description: "Second by name, first by order.", inputSchema: { type: "object" } })
      host.tools.add({ name: "apple", description: "First by name, second by order.", inputSchema: { type: "object" } })
    },
  }
  const run = await startEvie([model.plugin, tools])
  stop = run.stop

  await say(run, "hi")

  expect(model.requests[0]?.tools).toEqual([
    { name: "zebra", description: "Second by name, first by order.", inputSchema: { type: "object" } },
    { name: "apple", description: "First by name, second by order.", inputSchema: { type: "object" } },
  ])
})

test("Evie refuses to start when no plugin registers a provider", async () => {
  await expect(startEvie([])).rejects.toThrow("No provider is registered")
})

test("two plugins cannot register a tool under the same name", async () => {
  const model = scriptedProvider([])
  const clash: Plugin = {
    name: "clash",
    setup(host) {
      host.tools.add({ name: "read", description: "One.", inputSchema: {} })
      host.tools.add({ name: "read", description: "Two.", inputSchema: {} })
    },
  }
  await expect(startEvie([model.plugin, clash])).rejects.toThrow('A tool named "read" is already registered')
})

test("a turn that fails still ends in the log, and the session carries on", async () => {
  const model = scriptedProvider([{ fail: "the model is unreachable" }, { text: ["Back again."] }])
  const run = await startEvie([model.plugin])
  stop = run.stop

  await say(run, "hi")
  await say(run, "are you there?")

  const ends = stored(run.events).filter((event) => event.type === "turn_end")
  expect(ends.map((end) => end.body)).toEqual([
    { reason: "error", error: "the model is unreachable" },
    { reason: "completed" },
  ])
  expect(model.requests[1]?.messages).toEqual([
    { role: "user", text: "hi" },
    { role: "user", text: "are you there?" },
  ])
})

test("all state is kept in the data folder named by EVIE_HOME", async () => {
  const model = scriptedProvider([{ text: ["ok"] }])
  const run = await startEvie([model.plugin])
  stop = run.stop

  await say(run, "hi")

  expect(await readdir(run.home)).toContain("evie.db")
  expect(await readdir(run.projectFolder)).toEqual([])
})

test("a client that throws while handling an event does not break the turn", async () => {
  const model = scriptedProvider([{ text: ["ok"] }])
  const run = await startEvie([model.plugin])
  stop = run.stop
  run.evie.subscribe(() => {
    throw new Error("a broken client")
  })

  await say(run, "hi")

  expect(stored(run.events).at(-1)).toMatchObject({ type: "turn_end", body: { reason: "completed" } })
})

test("closing Evie cancels a model request that would never finish", async () => {
  const model = scriptedProvider([{ hang: true }])
  const run = await startEvie([model.plugin])
  stop = run.stop

  await run.evie.send({ type: "send_message", session: run.session, text: "hi" })
  await run.evie.close()

  expect(stored(run.events).at(-1)).toMatchObject({ type: "turn_end" })
})

test("a reply with no text is not sent back to the model as an empty message", async () => {
  const model = scriptedProvider([{ text: [] }, { text: ["Here now."] }])
  const run = await startEvie([model.plugin])
  stop = run.stop

  await say(run, "hi")
  await say(run, "hello?")

  expect(model.requests[1]?.messages).toEqual([
    { role: "user", text: "hi" },
    { role: "user", text: "hello?" },
  ])
})

test("a request the provider reports as failed ends the turn with its reason, and stores no reply", async () => {
  const model = scriptedProvider([{ text: ["Hel"], error: "OpenRouter could not finish the reply (502): gone" }])
  const run = await startEvie([model.plugin])
  stop = run.stop

  await say(run, "hi")

  expect(stored(run.events).map((event) => event.type)).toEqual(["turn_start", "user_message", "turn_end"])
  expect(stored(run.events).at(-1)).toMatchObject({
    body: { reason: "error", error: "OpenRouter could not finish the reply (502): gone" },
  })
})
