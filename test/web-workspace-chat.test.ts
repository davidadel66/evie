import { afterEach, expect, test } from "bun:test"
import type { SessionSummary } from "../src/core/types.ts"
import { ask, files } from "../src/index.ts"
import { openConnection } from "../web/src/connection.ts"
import { openConversation, type Conversation, type ConversationState } from "../web/src/conversation.ts"
import type { Item } from "../web/src/transcript.ts"
import { stepKind, stepWords } from "../web/src/step-words.ts"
import { chatLines, chatTitle, fullSize, givenBackWords, sideChatOf } from "../web/src/workspace-chat.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

// The side chat's rules, apart from its screen: what its header says, where
// it opens full size, and what it shows of a conversation, in short, also
// against evie's server with the scripted model, connected with a token as
// the connection's own tests do.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function start(script: ScriptedReply[], options: { questions?: boolean } = {}) {
  const model = scriptedProvider(script)
  const served = await serveEvie([model.plugin, files(), ask()], options)
  cleanups.push(served.stop)
  const connection = openConnection({ url: served.server.url, token: served.token, firstWait: 10 })
  cleanups.push(() => connection.close())
  const conversation = openConversation(connection, served.run.session)
  cleanups.push(async () => conversation.close())
  await until(conversation, (state) => state.opened)
  return { served, conversation }
}

async function until(conversation: Conversation, ready: (state: ConversationState) => boolean) {
  while (!ready(conversation.state())) await Bun.sleep(5)
  return conversation.state()
}

const summary = (name: string | null): SessionSummary => ({ session: "s1", project: "p1", name, state: "active", lastActive: 1 })

test("the side chat is titled by its session, and a chat not started yet by the workspace it is in", () => {
  expect(chatTitle(summary("Add the cache savings"), "evie")).toBe("Add the cache savings")
  expect(chatTitle(summary(null), "evie")).toBe("New session")
  expect(chatTitle(undefined, "recipe-site")).toBe("New chat in recipe-site")
})

test("the side chat opens full size in the session view: its session, or a chat not started yet as a new session in the workspace's project", () => {
  expect(fullSize("s1", "p1")).toBe("#session=s1")
  expect(fullSize("new", "p1")).toBe("#draft=p1")
})

const call = (id: string, name: string, input: string) => ({ id, name, kind: stepKind(name), sentence: stepWords({ name, input }), target: input, input, fields: {}, time: 1, result: "ok", short: "ok", running: false, failed: false })

test("the side chat shows the owner's messages, evie's words, and each run of work or note as one quiet line in plain words, which says the files a turn changed", () => {
  const items: Item[] = [
    { key: "e1", turn: 1, kind: "owner", text: "Add how much the cache saved", attachments: [{ path: "attachments/a.png", name: "a.png", type: "image/png", size: 10 }] },
    { key: "fold-9", turn: 1, kind: "fold", label: "Worked for 4s", items: [
      { key: "e2", turn: 1, kind: "reply", text: "Looking at the dashboard.", streaming: false },
      { key: "s-c1", turn: 1, kind: "steps", calls: [call("c1", "read", "usage.html"), call("c2", "edit", "usage.html")], label: "Used read and edit", running: false },
      { key: "s-c3", turn: 1, kind: "steps", calls: [call("c3", "edit", "usage.html")], label: "Used edit", running: false },
    ] },
    { key: "e8", turn: 1, kind: "reply", text: "Done. There is a new figure.", streaming: false },
    { key: "changes-9", turn: 1, kind: "changes", number: 1, files: [{ path: "usage.html", status: "changed", added: 3, removed: 1 }] },
    { key: "e10", turn: 2, kind: "owner", text: "Now the tokens", sending: [new File(["x"], "b.txt")] },
    { key: "s-c4", turn: 2, kind: "steps", calls: [call("c4", "shell", "bun test")], label: "Running bun", running: true },
    { key: "thinking", turn: 2, kind: "thinking" },
    { key: "e11", turn: 2, kind: "note", text: "Stopped." },
    { key: "e12", turn: 2, kind: "question", id: "q1", text: "Which week?", choices: ["This", "Last"], waiting: false },
    { key: "e13", turn: 2, kind: "failed", error: "The model's provider could not be reached." },
    { key: "job1", turn: 2, kind: "note", text: "Background job 1 started: Find recipes with no time" },
    { key: "e14", turn: 2, kind: "note", text: "Note to evie: Background job 1 (subagent: Find recipes with no time) has finished." },
    { key: "job1-end", turn: 2, kind: "note", text: "Background job 1 finished after 4s: Find recipes with no time" },
    { key: "job2-end", turn: 2, kind: "note", text: "Background job 2 failed after 1s: bun test" },
  ]

  expect(chatLines(items)).toEqual([
    { key: "e1", kind: "owner", text: "Add how much the cache saved", files: ["a.png"] },
    // Edits count the files changed, each once.
    { key: "fold-9", kind: "step", text: "Read 1 file and changed 1 file" },
    { key: "e8", kind: "text", text: "Done. There is a new figure." },
    { key: "e10", kind: "owner", text: "Now the tokens", files: ["b.txt"] },
    { key: "s-c4", kind: "step", text: "Ran bun test" },
    { key: "e11", kind: "step", text: "Stopped." },
    { key: "e12", kind: "text", text: "Which week?" },
    { key: "e13", kind: "failed", text: "The model's provider could not be reached." },
    { key: "job1-end", kind: "step", text: "Background work finished" },
    { key: "job2-end", kind: "step", text: "Background work failed" },
  ])
})

test("a turn in the side chat shows as the owner's message, one quiet line for evie's work, and its reply", async () => {
  const { conversation } = await start([{ text: ["Looking at the notes."], calls: [{ name: "read", input: { path: "notes.md" } }] }, { text: ["Done. The notes are short."] }])

  await conversation.send("Read the notes")
  const ended = await until(conversation, (state) => !state.turnRunning && state.events.some((event) => event.type === "turn_end"))

  expect(sideChatOf(ended).map(({ key: _, ...line }) => line)).toEqual([
    { kind: "owner", text: "Read the notes", files: [] },
    { kind: "step", text: "Read notes.md" },
    { kind: "text", text: "Done. The notes are short." },
  ])
})

// A conversation that has been opened, with nothing in it yet.
const opened: ConversationState = { opened: true, readingEarlier: false, stopping: false, waiting: [], running: [], givenBack: undefined, model: undefined, context: undefined, problem: undefined, events: [], earlier: false, replySoFar: "", turnRunning: false, jobs: [], pending: [], notes: [], project: "p1" }

test("each command running from the side chat shows as a quiet line at its end, until it has run", () => {
  expect(sideChatOf({ ...opened, running: ["compact", "sync"] })).toEqual([
    { key: "running-0", kind: "step", text: "Running /compact" },
    { key: "running-1", kind: "step", text: "Running /sync" },
  ])
  expect(sideChatOf(opened)).toEqual([])
})

test("a message given back to the small box says why, and that its files, which the box cannot hold, were not kept", () => {
  expect(givenBackWords({ text: "Plan the beds", problem: "Not connected to evie's server." })).toBe("Not connected to evie's server.")
  expect(givenBackWords({ text: "Plan the beds", problem: "Not connected to evie's server.", files: [{ file: new File(["x"], "beds.png") }] })).toBe(
    "Not connected to evie's server. Its attached files were not kept here: open this chat full size to attach them again.",
  )
})
