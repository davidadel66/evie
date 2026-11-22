import { afterEach, expect, test } from "bun:test"
import { naming, type Chunk, type Plugin, type Provider, type Request } from "../src/index.ts"
import { nextEvent, say, startEvie, stored, type Run } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

// Naming sessions: after the owner's first message, a small model writes the
// session a name in the background, and the owner can rename it at any time.
// The model here is a stand-in for both: the naming model's requests are
// answered by `namer`, every other by the script.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

const NAMING_MODEL = "anthropic/claude-haiku-5.5"

type Namer = (request: Request, signal: AbortSignal) => AsyncIterable<Chunk>

// A namer that answers with a name, once `until` has settled.
const answers = (name: string, until: Promise<unknown> = Promise.resolve()): Namer =>
  async function* () {
    await until
    yield { type: "text", text: name }
    yield { type: "finish", reason: "stop" }
  }

async function start(script: ScriptedReply[], namer: Namer, plugins: Plugin[] = [naming()]) {
  const main = scriptedProvider(script)
  const asked: Request[] = []
  const provider: Provider = {
    name: "stand-in",
    stream(request, signal) {
      if (request.model !== NAMING_MODEL) return main.provider.stream(request, signal)
      asked.push(structuredClone(request))
      return namer(request, signal)
    },
  }
  const run = await startEvie([{ name: "stand-in", setup: (host) => host.providers.add(provider) }, ...plugins], { questions: true })
  cleanups.push(run.stop)
  return { run, main, asked }
}

// The session's name, as the list of sessions shows it.
async function nameOf(run: Run, session = run.session) {
  return (await run.evie.send({ type: "list_sessions" })).find((each) => each.session === session)?.name
}

// A tool that starts a subagent and gives back the id of its session.
const delegate: Plugin = {
  name: "delegate",
  setup(host) {
    host.tools.add({ name: "delegate", description: "Starts a subagent.", inputSchema: { type: "object" }, run: (_input, context) => context.children.start({ text: "look around" }) })
  },
}

// A promise to wait on, and the function that settles it.
function gate() {
  let open = () => {}
  const opened = new Promise<void>((resolve) => (open = resolve))
  return { opened, open }
}

test("after the owner's first message a small model names the session in the background, without holding up the reply", async () => {
  const naming = gate()
  const { run, asked } = await start([{ text: ["Noted."] }], answers("Spring vegetable beds", naming.opened))

  await say(run, "Plan which vegetables go in which beds this spring")

  // The turn has ended while the name is still being written: until it
  // comes, the session is called by the opening words of its first message.
  expect(await nameOf(run)).toBe("Plan which vegetables go in which beds this spring")
  const named = nextEvent(run, "named")
  naming.open()
  expect((await named).body).toEqual({ name: "Spring vegetable beds", by: "evie" })
  expect(await nameOf(run)).toBe("Spring vegetable beds")
  // A summary told every client of the new name.
  expect(run.events.findLast((event) => event.type === "summary")).toMatchObject({ session: run.session, name: "Spring vegetable beds" })

  // The small model was given the first message, and rules for a short
  // name, with nothing to call.
  expect(asked).toHaveLength(1)
  expect(asked[0]!.messages).toEqual([{ role: "user", text: "Plan which vegetables go in which beds this spring" }])
  expect(asked[0]!.system).toContain("3 to 8 words")
  expect(asked[0]!.tools).toEqual([])
})

test("a naming request that fails leaves the session the opening words of its first message, tells the owner nothing, and is not tried again", async () => {
  const failing: Namer = async function* () {
    yield { type: "text", text: "Half a na" }
    throw new Error("the line dropped")
  }
  const { run, asked } = await start([{ text: ["Noted."] }, { text: ["Noted again."] }], failing)

  await say(run, "Plan which vegetables go in which beds this spring")
  await Bun.sleep(20)
  await say(run, "And the herbs too")
  await Bun.sleep(20)

  expect(asked).toHaveLength(1)
  expect(await nameOf(run)).toBe("Plan which vegetables go in which beds this spring")
  expect(stored(run.events).map((event) => event.type)).not.toContain("named")
  expect(stored(run.events).map((event) => event.type)).not.toContain("note")
})

test("a naming request that takes too long is given up, and a name that comes after it is not used", async () => {
  // A provider that pays no heed to being stopped, and answers late.
  const slow: Namer = async function* () {
    await Bun.sleep(150)
    yield { type: "text", text: "Spring vegetable beds" }
    yield { type: "finish", reason: "stop" }
  }
  const { run } = await start([{ text: ["Noted."] }], slow, [naming({ within: 30 })])

  await say(run, "Plan which vegetables go in which beds this spring")
  await Bun.sleep(250)

  expect(await nameOf(run)).toBe("Plan which vegetables go in which beds this spring")
  expect(stored(run.events).map((event) => event.type)).not.toContain("named")
})

test("the owner can rename a session at any time, and a name the small model writes afterwards does not replace his", async () => {
  const naming = gate()
  const { run } = await start([{ text: ["Noted."] }], answers("Spring vegetable beds", naming.opened))
  await say(run, "Plan which vegetables go in which beds this spring")

  await run.evie.send({ type: "rename_session", session: run.session, name: "  Garden\n beds  " })
  naming.open()
  await Bun.sleep(20)

  expect(await nameOf(run)).toBe("Garden beds")
  expect(stored(run.events).flatMap((event) => (event.type === "named" ? [event.body] : []))).toEqual([{ name: "Garden beds", by: "owner" }])

  // And again later, over a name the model gave.
  await run.evie.send({ type: "rename_session", session: run.session, name: "Beds for spring" })
  expect(await nameOf(run)).toBe("Beds for spring")
})

test("a name is not activity in the session: it keeps its place in the list, and one the owner settled stays settled", async () => {
  const { run } = await start([{ text: ["Noted."] }], answers("Spring vegetable beds"))
  const named = nextEvent(run, "named")
  await say(run, "Plan which vegetables go in which beds this spring")
  await named
  const before = (await run.evie.send({ type: "list_sessions" }))[0]!
  await run.evie.send({ type: "settle_session", session: run.session, settled: true })
  await Bun.sleep(5)

  await run.evie.send({ type: "rename_session", session: run.session, name: "Garden beds" })

  expect(await run.evie.send({ type: "list_sessions" })).toEqual([{ ...before, name: "Garden beds", state: "settled" }])
})

test("a blank name, a session that is not there, and a subagent's session are refused", async () => {
  const { run, asked } = await start([{ text: [], calls: [{ name: "delegate", input: {} }] }, { text: ["Looked."] }, { text: ["Started it."] }], answers("Spring vegetable beds"), [naming(), delegate])
  await say(run, "Plan which vegetables go in which beds this spring")
  const child = stored(run.events).find((event) => event.type === "tool_result")!.body as { text: string }
  await Bun.sleep(20)

  await expect(run.evie.send({ type: "rename_session", session: run.session, name: " \n " })).rejects.toThrow("cannot be blank")
  await expect(run.evie.send({ type: "rename_session", session: "no-such-session", name: "Beds" })).rejects.toThrow('There is no session with the id "no-such-session"')
  await expect(run.evie.send({ type: "rename_session", session: run.session, name: 7 as never })).rejects.toThrow("must be text")
  await expect(run.evie.send({ type: "rename_session", session: child.text, name: "Beds" })).rejects.toThrow("is a subagent's")
  // Nor is a subagent's session named by the small model: only the owner's session was.
  expect(asked).toHaveLength(1)
})
