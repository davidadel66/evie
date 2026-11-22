import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { startServer } from "../src/index.ts"
import { openTokens } from "../src/server/tokens.ts"
import { startEvie } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

// A project's folder as the web app's session details show it, from the
// home folder, and opened on the computer evie runs on, as T3 Code's
// "Open in" opens it, and only from that computer.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function folder(prefix: string) {
  const made = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(made, { recursive: true, force: true }))
  return made
}

test("each project is listed with its folder from the home folder, as ~/code/recipe-box, when it is inside it", async () => {
  const home = await folder("evie-user-")
  await mkdir(join(home, "code", "recipe-box"), { recursive: true })
  const run = await startEvie([scriptedProvider([]).plugin], { projectFolder: join(home, "code", "recipe-box"), env: { HOME: home } })
  cleanups.push(run.stop)
  const elsewhere = await folder("evie-elsewhere-")
  const other = await run.evie.send({ type: "open_session", folder: elsewhere })

  const listed = await run.evie.send({ type: "list_projects" })
  expect(listed.find((each) => each.id === run.project!.id)).toMatchObject({ folder: join(home, "code", "recipe-box"), fromHome: "~/code/recipe-box" })
  expect(listed.find((each) => each.id === other.project!.id)).not.toHaveProperty("fromHome")
})

test("a project's folder is opened on the computer evie runs on", async () => {
  const opened: string[] = []
  const run = await startEvie([scriptedProvider([]).plugin], { openFolder: async (path) => void opened.push(path) })
  cleanups.push(run.stop)
  await run.evie.send({ type: "open_folder", project: run.project!.id })
  expect(opened).toEqual([run.project!.folder])
  await expect(run.evie.send({ type: "open_folder", project: "nowhere" })).rejects.toThrow('There is no project with the id "nowhere".')
})

test("a project's folder can be opened only from the computer evie runs on, as keys are typed only there", async () => {
  const opened: string[] = []
  const run = await startEvie([scriptedProvider([]).plugin], { openFolder: async (path) => void opened.push(path) })
  const tokens = await openTokens(run.home)
  const server = startServer({ evie: run.evie, tokens, host: "127.0.0.1", port: 0, fromThisComputer: () => false })
  cleanups.push(async () => {
    await server.stop()
    tokens.close()
    await run.stop()
  })
  const token = (tokens.add("phone") as { secret: string }).secret
  const socket = new WebSocket(new URL("/ws", server.url).href.replace(/^http/, "ws"), { headers: { Authorization: `Bearer ${token}` } } as never)
  await new Promise((resolve, reject) => {
    socket.onopen = resolve
    socket.onerror = reject
  })
  const reply = new Promise<{ error?: string }>((resolve) => {
    socket.onmessage = (message) => {
      const parsed = JSON.parse(String(message.data))
      if ("reply" in parsed) resolve(parsed)
    }
  })
  socket.send(JSON.stringify({ id: "one", command: { type: "open_folder", project: run.project!.id } }))
  expect(await reply).toMatchObject({ error: "A project's folder can be opened only on the computer evie runs on." })
  socket.close()
  expect(opened).toEqual([])
})
