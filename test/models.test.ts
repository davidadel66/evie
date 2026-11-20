import { afterEach, expect, test } from "bun:test"
import { realpath } from "node:fs/promises"
import { files, models, runTerminal, shell, type Plugin } from "../src/index.ts"
import { say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

let stop: (() => Promise<void>) | undefined
afterEach(async () => {
  await stop?.()
  stop = undefined
})

const replies = (count: number): ScriptedReply[] => Array.from({ length: count }, () => ({ text: ["ok"] }))

async function start(model: string, extra: Plugin[] = [], script: ScriptedReply[] = replies(6)) {
  const provider = scriptedProvider(script)
  const run = await startEvie([provider.plugin, files(), shell(), models(), ...extra], { model })
  stop = run.stop
  const toolNames = (request = provider.requests.at(-1)!) => request.tools.map((tool) => tool.name)
  return { run, provider, toolNames }
}

test("a GPT model is given patch, and not edit or write, with the prompt written for GPT models", async () => {
  const { run, provider, toolNames } = await start("openai/gpt-5")

  await say(run, "hi")

  expect(toolNames()).toEqual(["read", "patch", "grep", "glob", "shell"])
  expect(provider.requests[0]?.system).toContain("use patch")
  expect(provider.requests[0]?.system).not.toContain("use edit")
})

test("any other model is given edit and write, and not patch, with the base prompt", async () => {
  const { run, provider, toolNames } = await start("anthropic/claude-sonnet-4.6")

  await say(run, "hi")

  expect(toolNames()).toEqual(["read", "write", "edit", "grep", "glob", "shell"])
  expect(provider.requests[0]?.system).toContain("use edit")
  expect(provider.requests[0]?.system).not.toContain("use patch")
})

test("the system prompt says which project and folder the session is in", async () => {
  const { run, provider } = await start("anthropic/claude-sonnet-4.6")
  const general = await run.evie.send({ type: "open_session" })

  await say(run, "hi")
  await say(run, "hi", general.session)

  expect(provider.requests[0]?.system).toContain(`Its folder is ${await realpath(run.projectFolder)}.`)
  expect(provider.requests[1]?.system).toContain("This session is not in a project")
})

test("a tool the model's profile leaves out cannot be called", async () => {
  const { run } = await start("openai/gpt-5", [], [
    { text: [], calls: [{ name: "edit", input: { path: "a.txt", oldString: "a", newString: "b" } }] },
    { text: ["ok"] },
  ])

  await say(run, "go")

  const results = stored(run.events).flatMap((event) => (event.type === "tool_result" ? [event.body.text] : []))
  expect(results).toEqual(['There is no tool named "edit". The tools are: read, patch, grep, glob, shell.'])
})

test("set_model is recorded, and the next request uses the new model, its prompt and its tools with the same conversation", async () => {
  const { run, provider, toolNames } = await start("anthropic/claude-sonnet-4.6")

  await say(run, "first")
  await run.evie.send({ type: "set_model", session: run.session, model: "openai/gpt-5" })
  await say(run, "second")

  expect(stored(run.events).filter((event) => event.type === "model_changed").map((event) => event.body)).toEqual([
    { model: "openai/gpt-5" },
  ])
  const [before, after] = provider.requests
  expect(before?.model).toBe("anthropic/claude-sonnet-4.6")
  expect(after?.model).toBe("openai/gpt-5")
  expect(toolNames(after)).toEqual(["read", "patch", "grep", "glob", "shell"])
  expect(after?.system).toContain("use patch")
  expect(after?.messages).toEqual([
    { role: "user", text: "first" },
    { role: "assistant", text: "ok" },
    { role: "user", text: "second" },
  ])
})

test("a model is switched in one session only", async () => {
  const { run, provider } = await start("anthropic/claude-sonnet-4.6")
  const other = await run.evie.send({ type: "open_session", folder: run.projectFolder })

  await run.evie.send({ type: "set_model", session: run.session, model: "openai/gpt-5" })
  await say(run, "here")
  await say(run, "there", other.session)

  expect(provider.requests.map((request) => request.model)).toEqual(["openai/gpt-5", "anthropic/claude-sonnet-4.6"])
})

test("set_model needs an open session and a model name", async () => {
  const { run } = await start("anthropic/claude-sonnet-4.6")

  await expect(run.evie.send({ type: "set_model", session: "no-such-session", model: "x/y" })).rejects.toThrow("is not open")
  await expect(run.evie.send({ type: "set_model", session: run.session, model: "  " })).rejects.toThrow("Give the name of a model")
})

test("the /model command switches the model, and with no name says which one is in use", async () => {
  const { run, provider } = await start("anthropic/claude-sonnet-4.6")
  const command = (args: string) => run.evie.send({ type: "run_command", session: run.session, name: "model", args })

  expect((await command("")).text).toBe("The model is anthropic/claude-sonnet-4.6 (profile: default).")
  expect((await command("openai/gpt-5")).text).toBe("The model is now openai/gpt-5 (profile: gpt).")
  await say(run, "hi")

  expect(provider.requests[0]?.model).toBe("openai/gpt-5")
})

test("a command that does not exist says which ones do", async () => {
  const { run } = await start("anthropic/claude-sonnet-4.6")

  await expect(run.evie.send({ type: "run_command", session: run.session, name: "nope", args: "" })).rejects.toThrow(
    'There is no command "/nope". The commands are: /model, /compact.',
  )
})

test("another plugin can add a profile for a further model family", async () => {
  const gemini: Plugin = {
    name: "gemini-profile",
    setup(host) {
      host.profiles.add({ name: "gemini", prefix: "google/gemini-", prompt: "You are Evie, tuned for Gemini.", fileTools: ["write"] })
    },
  }
  const { run, provider, toolNames } = await start("google/gemini-3-pro", [gemini])

  await say(run, "hi")

  expect(toolNames()).toEqual(["read", "write", "grep", "glob", "shell"])
  expect(provider.requests[0]?.system).toStartWith("You are Evie, tuned for Gemini.")
})

test("in the terminal, a line starting with / runs a command and is not sent to the model", async () => {
  const { run, provider } = await start("anthropic/claude-sonnet-4.6")
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield "/model openai/gpt-5"
      yield "/nope"
      yield "/Users/me/app/main.ts is failing, why?"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("The model is now openai/gpt-5 (profile: gpt).")
  expect(printed).toContain('[error] There is no command "/nope". The commands are: /model, /compact.')
  // A message that starts with a path is a message, not a command.
  expect(provider.requests).toHaveLength(1)
  expect(provider.requests[0]?.messages).toEqual([{ role: "user", text: "/Users/me/app/main.ts is failing, why?" }])
})
