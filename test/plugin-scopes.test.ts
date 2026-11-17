import { afterEach, expect, test } from "bun:test"
import { mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import type { Plugin, Project } from "../src/index.ts"
import { say, startEvie } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function otherFolder() {
  const folder = await realpath(await mkdtemp(join(tmpdir(), "evie-other-project-")))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

// A plugin that registers one tool and records each time it is set up.
function toolPlugin(name: string, setups: string[] = []) {
  const plugin: Plugin = {
    name,
    setup(host) {
      setups.push(`${name} for ${host.project?.name ?? "general"}`)
      host.tools.add({ name, description: `The ${name} tool.`, inputSchema: { type: "object" }, run: async () => "ok" })
    },
  }
  return { plugin, setups }
}

async function start(plugins: Plugin[], projectPlugins?: (project: Project) => Plugin[]) {
  const model = scriptedProvider(Array.from({ length: 10 }, () => ({ text: ["ok"] })))
  const run = await startEvie([model.plugin, ...plugins], projectPlugins ? { projectPlugins } : {})
  cleanups.push(run.stop)
  // The names of the tools the model was offered in a session's next turn.
  async function offered(session: string) {
    await say(run, "hi", session)
    return model.requests.at(-1)!.tools.map((tool) => tool.name)
  }
  return { run, offered }
}

test("a plugin is told which project it is being set up for, or that it is for the general session", async () => {
  const seen: (Project | null)[] = []
  const { run } = await start([{ name: "watcher", setup: (host) => void seen.push(host.project) }])

  expect(seen).toEqual([null, run.project])
})

test("the first session in a project sets up its plugins, and later sessions reuse them", async () => {
  const global = toolPlugin("global")
  const { run } = await start([global.plugin])
  const before = global.setups.length

  await run.evie.send({ type: "open_session", folder: run.projectFolder })
  await run.evie.send({ type: "open_session", project: run.project!.id })

  expect(global.setups.length).toBe(before)
  expect(global.setups.filter((setup) => setup !== "global for general")).toHaveLength(1)
})

test("a global plugin's tools are offered in every project and in the general session", async () => {
  const { run, offered } = await start([toolPlugin("everywhere").plugin])
  const other = await run.evie.send({ type: "open_session", folder: await otherFolder() })
  const general = await run.evie.send({ type: "open_session" })

  expect(await offered(run.session)).toEqual(["everywhere"])
  expect(await offered(other.session)).toEqual(["everywhere"])
  expect(await offered(general.session)).toEqual(["everywhere"])
})

test("a project's own plugin is offered only in that project", async () => {
  const special = await otherFolder()
  const { run, offered } = await start([toolPlugin("everywhere").plugin], (project) =>
    project.folder === special ? [toolPlugin("only-here").plugin] : [],
  )
  const inSpecial = await run.evie.send({ type: "open_session", folder: special })
  const general = await run.evie.send({ type: "open_session" })

  expect(await offered(inSpecial.session)).toEqual(["everywhere", "only-here"])
  expect(await offered(run.session)).toEqual(["everywhere"])
  expect(await offered(general.session)).toEqual(["everywhere"])
})

test("a project's own plugins are set up after the global ones", async () => {
  const setups: string[] = []
  const { run } = await start([toolPlugin("global", setups).plugin], () => [toolPlugin("own", setups).plugin])
  const name = run.project!.name

  expect(setups).toEqual(["global for general", `global for ${name}`, `own for ${name}`])
})

test("a project whose plugins fail to set up refuses the session, and says which plugin", async () => {
  const broken: Plugin = {
    name: "broken",
    setup() {
      throw new Error("could not connect")
    },
  }
  const special = await otherFolder()
  const { run } = await start([], (project) => (project.folder === special ? [broken] : []))

  await expect(run.evie.send({ type: "open_session", folder: special })).rejects.toThrow(
    'The plugin "broken" could not be set up: could not connect',
  )
})
