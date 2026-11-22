import { afterEach, expect, test } from "bun:test"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { files, undo as undoPlugin } from "../src/index.ts"
import { holdChanges, type Changes, type ChangesState } from "../web/src/changes.ts"
import { openConnection } from "../web/src/connection.ts"
import { holdConversation } from "../web/src/conversation.ts"
import { say } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

// What a session changed, as the web app follows it, apart from its
// screen: listed once the session is open, and again as each turn ends
// and when its changes are undone; and each comparison read on demand.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function start(script: ScriptedReply[]) {
  const home = await mkdtemp(join(tmpdir(), "evie-changes-"))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  const served = await serveEvie([scriptedProvider(script).plugin, files(), undoPlugin({ dataFolder: home })])
  cleanups.push(served.stop)
  const connection = openConnection({ url: served.server.url, token: served.token, firstWait: 10 })
  cleanups.push(async () => connection.close())
  const held = holdChanges(connection, served.run.session)
  cleanups.push(async () => held.release())
  return { served, connection, changes: held.changes }
}

async function until(changes: Changes, ready: (state: ChangesState) => boolean) {
  while (!ready(changes.state())) await Bun.sleep(5)
  return changes.state()
}

const writes = (path: string, content: string): ScriptedReply[] => [{ text: [], calls: [{ name: "write", input: { path, content } }] }, { text: ["Written."] }]

test("a session's changes are listed once it is open, and again as each turn ends, and each comparison is read when asked for", async () => {
  const { served, changes } = await start([...writes("a.md", "one\n"), ...writes("b.md", "two\n")])
  expect((await until(changes, (state) => state.listed !== undefined)).listed?.turns).toEqual([])

  await say(served.run, "Write a")
  const once = await until(changes, (state) => state.listed?.turns.length === 1)
  expect(once.listed?.latest).toBe(1)
  await say(served.run, "Write b")
  const twice = await until(changes, (state) => state.listed?.turns.length === 2)
  expect(twice.listed?.session.map((file) => file.path)).toEqual(["a.md", "b.md"])

  expect(await changes.read("turn:1")).toEqual({ files: [{ path: "a.md", status: "added", added: 1, removed: 0, patch: "@@ -0,0 +1,1 @@\n+one\n" }] })
  // The latest turn is the session's last, as the list says it.
  expect((await changes.read("latest")).files.map((file) => file.path)).toEqual(["b.md"])
  expect((await changes.read("session")).files.map((file) => file.path)).toEqual(["a.md", "b.md"])
})

test("changes undone are listed no more", async () => {
  const { served, connection, changes } = await start(writes("a.md", "one\n"))
  await say(served.run, "Write a")
  await until(changes, (state) => state.listed?.turns.length === 1)

  const { conversation, release } = holdConversation(connection, served.run.session)
  cleanups.push(async () => release())
  await conversation.run("undo", "")
  expect((await until(changes, (state) => state.listed?.turns.length === 0)).listed?.session).toEqual([])
})
