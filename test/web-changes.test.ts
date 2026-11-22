import { afterEach, expect, test } from "bun:test"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { files, startServer, undo as undoPlugin, type Command, type Evie } from "../src/index.ts"
import { holdChanges, type Changes, type ChangesState } from "../web/src/changes.ts"
import { openConnection } from "../web/src/connection.ts"
import { holdConversation } from "../web/src/conversation.ts"
import { diffOf, sidesLoader } from "../web/src/diff-view.ts"
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

// With `refuse`, the connection is to a server in front of the core whose
// commands pass through it, which can make one fail, as a server with a
// fault would.
async function start(script: ScriptedReply[], refuse?: (command: Command) => boolean) {
  const home = await mkdtemp(join(tmpdir(), "evie-changes-"))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  const served = await serveEvie([scriptedProvider(script).plugin, files(), undoPlugin({ dataFolder: home })])
  cleanups.push(served.stop)
  let url = served.server.url
  if (refuse) {
    const core = served.run.evie
    const evie: Evie = {
      send: (command) => (refuse(command) ? Promise.reject(new Error("The disk is busy.")) : core.send(command)) as never,
      subscribe: (listener) => core.subscribe(listener),
      close: async () => {},
    }
    const server = startServer({ evie, tokens: served.tokens, host: "127.0.0.1", port: 0 })
    cleanups.push(() => server.stop())
    url = server.url
  }
  const connection = openConnection({ url, token: served.token, firstWait: 10 })
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
  await until(changes, (state) => state.listed?.turns.length === 2)

  expect(await changes.read("turn:1")).toEqual({ files: [{ path: "a.md", status: "added", added: 1, removed: 0, patch: "@@ -0,0 +1,1 @@\n+one\n" }] })
  // The latest turn is the session's last, as the list says it.
  expect((await changes.read("latest")).files.map((file) => file.path)).toEqual(["b.md"])
  expect((await changes.read("session")).files.map((file) => file.path)).toEqual(["a.md", "b.md"])
})

test("a file's whole sides are read for what the diff panel compares, the latest turn as the list says it, and handed to the diff view to open the lines folded between changes", async () => {
  const numbered = (change: (line: number) => string) => Array.from({ length: 20 }, (_, at) => `${change(at + 1)}\n`).join("")
  const first = numbered((line) => `line ${line}`)
  const second = numbered((line) => (line === 3 || line === 17 ? `changed ${line}` : `line ${line}`))
  const { served, changes } = await start([...writes("a.md", first), ...writes("a.md", second)])
  await say(served.run, "Write a")
  await say(served.run, "Change it")
  await until(changes, (state) => state.listed?.turns.length === 2)

  const file = (await changes.read("latest")).files[0]!
  // Only the lines around the changes come with the diff: lines 7 to 13 are folded between them.
  const diff = diffOf(file)!
  expect(diff.hunks.map((hunk) => hunk.collapsedBefore)).toEqual([0, 7])
  expect(diff.isPartial).toBe(true)
  expect(await changes.sides("latest", "a.md")).toEqual({ before: first, after: second })
  expect(await sidesLoader((path, from) => changes.sides("latest", path, from))(diff)).toEqual({ oldFile: { name: "a.md", contents: first }, newFile: { name: "a.md", contents: second } })
  expect(await changes.sides("session", "a.md")).toEqual({ before: null, after: second })
})

test("changes undone are listed no more", async () => {
  const { served, connection, changes } = await start(writes("a.md", "one\n"))
  await say(served.run, "Write a")
  await until(changes, (state) => state.listed?.turns.length === 1)

  const { conversation, release } = holdConversation(connection, served.run.session)
  cleanups.push(async () => release())
  await conversation.run("undo", "")
  expect((await until(changes, (state) => state.listed?.turns.length === 0)).listed?.turns).toEqual([])
})

test("the whole session's files are listed only while a part of the page that shows them holds the changes", async () => {
  const { served, connection, changes } = await start([...writes("a.md", "one\n"), ...writes("b.md", "two\n")])
  await say(served.run, "Write a")
  expect((await until(changes, (state) => state.listed?.turns.length === 1)).listed?.session).toBeUndefined()

  const whole = holdChanges(connection, served.run.session, { whole: true })
  cleanups.push(async () => whole.release())
  expect(whole.changes).toBe(changes)
  await until(changes, (state) => state.listed?.session !== undefined)
  await say(served.run, "Write b")
  const both = await until(changes, (state) => state.listed?.turns.length === 2)
  expect(both.listed?.session?.map((file) => file.path)).toEqual(["a.md", "b.md"])
})

test("changes that could not be listed are asked for again at the next turn's end, not as the reply streams in", async () => {
  let asked = 0
  const reply = Array.from({ length: 40 }, (_, at) => `word${at} `)
  const { served, changes } = await start([{ text: reply }], (command) => command.type === "list_changes" && ++asked > 0)
  await until(changes, (state) => state.problem !== undefined)
  const before = asked
  await say(served.run, "Say a lot")
  await Bun.sleep(100)
  expect(asked - before).toBe(1)
})
