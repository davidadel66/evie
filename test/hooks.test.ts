import { afterEach, expect, test } from "bun:test"
import type { HookContext, Hooks, Host, Plugin } from "../src/index.ts"
import { say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

let stop: (() => Promise<void>) | undefined
afterEach(async () => {
  await stop?.()
  stop = undefined
})

// A tool that says its input back, and counts how often it ran.
function echoTool() {
  let runs = 0
  const plugin: Plugin = {
    name: "echo-tool",
    setup(host) {
      host.tools.add({
        name: "echo",
        description: "Says something back.",
        inputSchema: { type: "object", properties: { text: { type: "string" } } },
        run: async (input) => {
          runs += 1
          return `You said: ${input.text}`
        },
      })
    },
  }
  return { plugin, runs: () => runs }
}

const plugin = (name: string, setup: (host: Host) => void): Plugin => ({ name, setup })

async function start(script: ScriptedReply[], plugins: Plugin[]) {
  const model = scriptedProvider(script)
  const run = await startEvie([model.plugin, ...plugins])
  stop = run.stop
  const results = () => stored(run.events).flatMap((event) => (event.type === "tool_result" ? [event.body.text] : []))
  return { run, model, results }
}

const callEcho: ScriptedReply[] = [{ text: [], calls: [{ name: "echo", input: { text: "hi" } }] }, { text: ["done"] }]

// ---- before_request -----------------------------------------------------------------

test("a before_request hook can set the system prompt and change the tools the model is offered", async () => {
  const { run, model } = await start(
    [{ text: ["ok"] }],
    [
      echoTool().plugin,
      plugin("shaper", (host) =>
        host.on("before_request", (request) => ({ ...request, system: "You are Evie.", tools: [] })),
      ),
    ],
  )

  await say(run, "hi")

  expect(model.requests[0]?.system).toBe("You are Evie.")
  expect(model.requests[0]?.tools).toEqual([])
})

test("hooks run in the order their plugins are listed, each receiving what the one before returned", async () => {
  const append = (name: string) =>
    plugin(name, (host) => host.on("before_request", (request) => ({ ...request, system: `${request.system ?? ""}${name};` })))
  const { run, model } = await start([{ text: ["ok"] }], [append("first"), append("second"), append("third")])

  await say(run, "hi")

  expect(model.requests[0]?.system).toBe("first;second;third;")
})

test("a hook that returns nothing leaves the request as it was", async () => {
  const { run, model } = await start([{ text: ["ok"] }], [echoTool().plugin, plugin("watcher", (host) => host.on("before_request", () => {}))])

  await say(run, "hi")

  expect(model.requests[0]?.tools.map((tool) => tool.name)).toEqual(["echo"])
})

test("a tool that a hook left out of the request cannot be called", async () => {
  const echo = echoTool()
  const { run, results } = await start(callEcho, [
    echo.plugin,
    plugin("hider", (host) => host.on("before_request", (request) => ({ ...request, tools: [] }))),
  ])

  await say(run, "go")

  expect(results()).toEqual(['There is no tool named "echo". The tools are: none.'])
  expect(echo.runs()).toBe(0)
})

// ---- before_tool --------------------------------------------------------------------

test("a before_tool hook can deny a call: the tool does not run, and the reason is its result", async () => {
  const echo = echoTool()
  const { run, results } = await start(callEcho, [
    echo.plugin,
    plugin("guard", (host) => host.on("before_tool", (call) => (call.name === "echo" ? { deny: "Echoing is not allowed here." } : undefined))),
  ])

  await say(run, "go")

  expect(results()).toEqual(["Echoing is not allowed here."])
  expect(echo.runs()).toBe(0)
  expect(stored(run.events).at(-1)).toMatchObject({ type: "turn_end", body: { reason: "completed" } })
})

test("a before_tool hook sees the checked input, and a call no hook denies runs", async () => {
  const seen: unknown[] = []
  const echo = echoTool()
  const { run, results } = await start(callEcho, [
    echo.plugin,
    plugin("watcher", (host) => host.on("before_tool", (call) => void seen.push({ name: call.name, input: call.input }))),
  ])

  await say(run, "go")

  expect(seen).toEqual([{ name: "echo", input: { text: "hi" } }])
  expect(results()).toEqual(["You said: hi"])
})

test("once one hook denies a call, later hooks are not asked", async () => {
  const asked: string[] = []
  const { run } = await start(callEcho, [
    echoTool().plugin,
    plugin("first", (host) => host.on("before_tool", () => (asked.push("first"), { deny: "no" }))),
    plugin("second", (host) => host.on("before_tool", () => void asked.push("second"))),
  ])

  await say(run, "go")

  expect(asked).toEqual(["first"])
})

// ---- after_tool ---------------------------------------------------------------------

test("after_tool hooks can change the result the model sees, in order", async () => {
  const { run, model, results } = await start(callEcho, [
    echoTool().plugin,
    plugin("loud", (host) => host.on("after_tool", (_call, text) => text.toUpperCase())),
    plugin("signed", (host) => host.on("after_tool", (call, text) => `${text} [${call.name}]`)),
  ])

  await say(run, "go")

  expect(results()).toEqual(["YOU SAID: HI [echo]"])
  expect(model.requests[1]?.messages.at(-1)).toEqual({ role: "tool", callId: "call_1_0", text: "YOU SAID: HI [echo]" })
})

// ---- what every hook is told ----------------------------------------------------------

test("every hook is told the session, its project, the session's events and the turn's cancel signal", async () => {
  const told: Record<string, HookContext> = {}
  const { run } = await start(callEcho, [
    echoTool().plugin,
    plugin("watcher", (host) => {
      host.on("before_request", (_request, context) => void (told.before_request ??= context))
      host.on("before_tool", (_call, context) => void (told.before_tool = context))
      host.on("after_tool", (_call, _text, context) => void (told.after_tool = context))
    }),
  ])

  await say(run, "go")

  for (const name of ["before_request", "before_tool", "after_tool"]) {
    expect(told[name]?.session).toBe(run.session)
    expect(told[name]?.project).toEqual(run.project)
    expect(told[name]?.signal).toBeInstanceOf(AbortSignal)
  }
  expect(told.before_request?.events.map((event) => event.type)).toEqual(["turn_start", "user_message"])
  expect(told.after_tool?.events.map((event) => event.type)).toEqual(["turn_start", "user_message", "assistant_message"])
})

test("in the general session a hook is told there is no project", async () => {
  let project: unknown = "not told"
  const { run } = await start([{ text: ["ok"] }], [plugin("watcher", (host) => host.on("before_request", (_request, context) => void (project = context.project)))])
  const general = await run.evie.send({ type: "open_session" })

  await say(run, "hi", general.session)

  expect(project).toBeNull()
})

// ---- a plugin that breaks ---------------------------------------------------------------

test("a hook that throws before a request ends the turn with an error that names the plugin", async () => {
  const { run, model } = await start(
    [{ text: ["ok"] }],
    [plugin("clumsy", (host) => host.on("before_request", () => { throw new Error("boom") }))],
  )

  await say(run, "hi")

  expect(stored(run.events).at(-1)).toMatchObject({
    type: "turn_end",
    body: { reason: "error", error: 'The plugin "clumsy" broke in its before_request hook: boom' },
  })
  expect(model.requests).toHaveLength(0)
})

test("a hook that throws around a tool ends the turn, and the call still has a result", async () => {
  const { run, model } = await start(
    [...callEcho.slice(0, 1), { text: ["Carrying on."] }],
    [echoTool().plugin, plugin("clumsy", (host) => host.on("after_tool", () => { throw new Error("boom") }))],
  )

  await say(run, "go")
  await say(run, "what happened?")

  expect(stored(run.events).filter((event) => event.type === "turn_end")[0]).toMatchObject({
    body: { reason: "error", error: 'The plugin "clumsy" broke in its after_tool hook: boom' },
  })
  const asked = model.requests[1]!.messages
  expect(asked.filter((message) => message.role === "tool")).toHaveLength(1)
})

test("a hook that returns something that is not a denial does not stop a later hook from denying", async () => {
  const echo = echoTool()
  const sloppy = (() => true) as unknown as Hooks["before_tool"]
  const { run, results } = await start(callEcho, [
    echo.plugin,
    plugin("sloppy", (host) => host.on("before_tool", sloppy)),
    plugin("guard", (host) => host.on("before_tool", () => ({ deny: "Not allowed." }))),
  ])

  await say(run, "go")

  expect(results()).toEqual(["Not allowed."])
  expect(echo.runs()).toBe(0)
})
