import { afterEach, expect, test } from "bun:test"
import { chmod, mkdtemp, readFile, realpath, rm, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { files, undo, type Plugin } from "../src/index.ts"
import { nextEvent, say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedCall, type ScriptedReply } from "./support/scripted-provider.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

const write = (path: string, content: string): ScriptedCall => ({ name: "write", input: { path, content } })
const edit = (path: string, oldString: string, newString: string): ScriptedCall => ({ name: "edit", input: { path, oldString, newString } })
const patch = (patchText: string): ScriptedCall => ({ name: "patch", input: { patchText: `*** Begin Patch\n${patchText}\n*** End Patch` } })
// One turn in which the model asks for these tools, one after another, and then answers.
const turn = (...calls: ScriptedCall[]): ScriptedReply[] => [...calls.map((call) => ({ text: [], calls: [call] })), { text: ["Done."] }]

// Evie with the real file tools and undo, in a temporary project that
// starts with the given files.
async function start(script: ScriptedReply[], starting: Record<string, string> = {}, again?: { home: string; projectFolder: string }, extra: Plugin[] = []) {
  const home = again?.home ?? (await realpath(await mkdtemp(join(tmpdir(), "evie-home-"))))
  const projectFolder = again?.projectFolder ?? (await realpath(await mkdtemp(join(tmpdir(), "evie-project-"))))
  if (!again) cleanups.push(() => rm(home, { recursive: true, force: true }), () => rm(projectFolder, { recursive: true, force: true }))
  for (const [name, text] of Object.entries(starting)) await writeFile(join(projectFolder, name), text)
  const model = scriptedProvider(script)
  const run = await startEvie([model.plugin, files(), undo({ dataFolder: home }), ...extra], { home, projectFolder })
  cleanups.push(run.stop)
  const file = (name: string) => readFile(join(projectFolder, name), "utf8").catch(() => undefined)
  const undoIt = async (session = run.session) => (await run.evie.send({ type: "run_command", session, name: "undo", args: "" })).text
  return { run, model, home, projectFolder, file, undoIt }
}

test("/undo puts a file back as it was before the turn that changed it", async () => {
  const { run, file, undoIt } = await start(turn(edit("notes.md", "one", "two"), edit("notes.md", "two", "three")), { "notes.md": "one\n" })

  await say(run, "change it")
  expect(await file("notes.md")).toBe("three\n")

  expect(await undoIt()).toBe("Put back: notes.md.")
  expect(await file("notes.md")).toBe("one\n")
})

test("a file the turn created is removed, and a file it deleted comes back", async () => {
  const { run, file, undoIt } = await start(turn(write("new.md", "fresh\n"), patch("*** Delete File: old.md")), { "old.md": "keep me\n" })

  await say(run, "tidy up")
  expect(await file("new.md")).toBe("fresh\n")
  expect(await file("old.md")).toBeUndefined()

  expect(await undoIt()).toBe("Put back: old.md. Removed, because the turn created it: new.md.")
  expect(await file("new.md")).toBeUndefined()
  expect(await file("old.md")).toBe("keep me\n")
})

test("undo can be repeated to go back a turn at a time, until there is nothing left", async () => {
  const { run, file, undoIt } = await start([...turn(edit("a.md", "1", "2")), { text: ["Just talking."] }, ...turn(edit("a.md", "2", "3"), write("b.md", "b\n"))], { "a.md": "1\n" })

  await say(run, "first")
  await say(run, "a turn that changes nothing")
  await say(run, "second")

  expect(await undoIt()).toBe("Put back: a.md. Removed, because the turn created it: b.md.")
  expect(await file("a.md")).toBe("2\n")
  expect(await undoIt()).toBe("Put back: a.md.")
  expect(await file("a.md")).toBe("1\n")
  expect(await undoIt()).toStartWith("There is nothing to undo")
})

test("the model is told what was undone, so it does not assume its changes are still there", async () => {
  const { run, model, undoIt } = await start([...turn(edit("notes.md", "one", "two")), { text: ["Understood."] }], { "notes.md": "one\n" })

  await say(run, "change it")
  await undoIt()
  await say(run, "try something else")

  const note =
    "[Note from Evie: The owner undid the file changes of an earlier turn. Put back: notes.md. These files no longer hold what you wrote, so read a file again before you change it.]"
  expect(model.requests.at(-1)?.messages.slice(-3)).toEqual([
    { role: "assistant", text: "Done." },
    { role: "user", text: note },
    { role: "user", text: "try something else" },
  ])
  expect(stored(run.events).filter((event) => event.type === "note")).toHaveLength(1)
})

test("a file the owner has changed since is left alone, and undo says so", async () => {
  const { run, projectFolder, file, undoIt } = await start(turn(edit("mine.md", "one", "two"), edit("other.md", "a", "b")), { "mine.md": "one\n", "other.md": "a\n" })

  await say(run, "change both")
  await writeFile(join(projectFolder, "mine.md"), "the owner's own edit\n")

  expect(await undoIt()).toBe("Put back: other.md. Left alone, because something other than Evie's file tools changed it: mine.md.")
  expect(await file("mine.md")).toBe("the owner's own edit\n")
  expect(await file("other.md")).toBe("a\n")
})

test("an edit that failed leaves nothing to undo", async () => {
  const { run, undoIt } = await start(turn(edit("notes.md", "not in the file", "x")), { "notes.md": "one\n" })

  await say(run, "change it")

  expect(await undoIt()).toStartWith("There is nothing to undo")
})

test("what a shell command changed is not undone, and undo says so", async () => {
  // A stand-in for the shell tool, which changes a file as a command might.
  const shellStandIn: Plugin = {
    name: "shell-stand-in",
    setup(host) {
      host.tools.add({
        name: "shell",
        description: "A stand-in.",
        inputSchema: { type: "object" },
        run: async (_input, context) => {
          await writeFile(join(context.folder!, "built.txt"), "from a command\n")
          return "ok"
        },
      })
    },
  }
  const { run, file, undoIt } = await start(turn(edit("notes.md", "one", "two"), { name: "shell", input: { command: "make" } }), { "notes.md": "one\n" }, undefined, [shellStandIn])

  await say(run, "build")

  expect(await undoIt()).toBe("Put back: notes.md. Commands run through the shell in that turn were not undone.")
  expect(await file("built.txt")).toBe("from a command\n")
})

test("undo still works after Evie has been stopped and the session continued", async () => {
  const first = await start(turn(edit("notes.md", "one", "two")), { "notes.md": "one\n" })
  await say(first.run, "change it")
  await first.run.evie.close()

  const second = await start([], {}, first)
  const continued = await second.run.evie.send({ type: "open_session", folder: first.projectFolder, continue: true })

  expect(await second.undoIt(continued.session)).toBe("Put back: notes.md.")
  expect(await second.file("notes.md")).toBe("one\n")
})

test("undo is refused while a turn is running", async () => {
  const { run, file, undoIt } = await start([...turn(edit("notes.md", "one", "two")), { hang: true, text: ["Thinking"] }], { "notes.md": "one\n" })
  await say(run, "change it")

  const writing = nextEvent(run, "assistant_delta")
  await run.evie.send({ type: "send_message", session: run.session, text: "and now think" })
  await writing

  expect(await undoIt()).toBe("Evie is in the middle of a turn. Undo when it has finished.")
  expect(await file("notes.md")).toBe("two\n")
})

test("a note added during a turn is written at the next step, never between a tool call and its result", async () => {
  // A command that tells the model something, and a tool slow enough to run it during.
  let release = () => {}
  let started = () => {}
  const running = new Promise<void>((resolve) => (started = resolve))
  const helpers: Plugin = {
    name: "helpers",
    setup(host) {
      host.commands.add({ name: "remind", description: "Tells the model something.", run: (args, context) => (context.tell(args), `busy: ${context.busy}`) })
      host.tools.add({
        name: "slow",
        description: "Takes a while.",
        inputSchema: { type: "object" },
        run: async () => {
          started()
          await new Promise<void>((resolve) => (release = resolve))
          return "finished"
        },
      })
    },
  }
  const { run, model } = await start(turn({ name: "slow", input: {} }), {}, undefined, [helpers])

  const ended = nextEvent(run, "turn_end")
  await run.evie.send({ type: "send_message", session: run.session, text: "go" })
  await running
  expect((await run.evie.send({ type: "run_command", session: run.session, name: "remind", args: "the tests moved" })).text).toBe("busy: true")
  release()
  await ended

  expect(stored(run.events).map((event) => event.type)).toEqual(["turn_start", "user_message", "assistant_message", "tool_result", "note", "assistant_message", "turn_end"])
  expect(model.requests[1]?.messages.at(-1)).toEqual({ role: "user", text: "[Note from Evie: the tests moved]" })
})

// ---- Found in review ---------------------------------------------------------------------------

test("a file the owner edits part way through a turn is not put back over their edit", async () => {
  // A stand-in for the owner editing the file by hand between two of Evie's edits.
  const ownerEdits: Plugin = {
    name: "owner-edits",
    setup(host) {
      host.tools.add({
        name: "wait",
        description: "Waits.",
        inputSchema: { type: "object" },
        run: async (_input, context) => {
          const path = join(context.folder!, "notes.md")
          await writeFile(path, `${await readFile(path, "utf8")}the owner's own line\n`)
          return "waited"
        },
      })
    },
  }
  const { run, file, undoIt } = await start(
    turn(edit("notes.md", "one", "two"), { name: "wait", input: {} }, edit("notes.md", "two", "three")),
    { "notes.md": "one\n" },
    undefined,
    [ownerEdits],
  )

  await say(run, "change it twice")

  expect(await undoIt()).toBe("Left alone, because something other than Evie's file tools changed it: notes.md.")
  expect(await file("notes.md")).toBe("three\nthe owner's own line\n")
})

test("one file reached by two names is one file to undo", async () => {
  const { run, projectFolder, file } = await start([], { "notes.md": "one\n" })
  await symlink(join(projectFolder, "notes.md"), join(projectFolder, "link.md"))
  const again = await start(turn(edit("notes.md", "one", "two"), edit("link.md", "two", "three")), {}, { home: run.home, projectFolder })

  await say(again.run, "change it by both names")
  expect(await file("notes.md")).toBe("three\n")

  expect(await again.undoIt()).toBe("Put back: notes.md.")
  expect(await file("notes.md")).toBe("one\n")
})

test("a file that cannot be put back is said so, and can be undone once that is fixed", async () => {
  const { run, projectFolder, file, undoIt } = await start(turn(edit("locked.md", "one", "two"), edit("free.md", "a", "b")), { "locked.md": "one\n", "free.md": "a\n" })
  await say(run, "change both")
  await chmod(join(projectFolder, "locked.md"), 0o444)

  const first = await undoIt()
  expect(first).toStartWith("Put back: free.md. Could not be put back: locked.md (")
  expect(first).toEndWith("Run /undo again once that is sorted out.")
  expect(await file("free.md")).toBe("a\n")

  await chmod(join(projectFolder, "locked.md"), 0o644)
  expect(await undoIt()).toBe("Put back: locked.md.")
  expect(await file("locked.md")).toBe("one\n")
})

test("a file Evie was stopped part way through changing is left alone, and its copy is kept", async () => {
  // A plugin whose after_tool hook breaks, so undo never learns what the file held afterwards.
  const breaks: Plugin = {
    name: "breaks",
    setup(host) {
      host.on("after_tool", () => {
        throw new Error("stopped here")
      })
    },
  }
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  const projectFolder = await realpath(await mkdtemp(join(tmpdir(), "evie-project-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }), () => rm(projectFolder, { recursive: true, force: true }))
  await writeFile(join(projectFolder, "notes.md"), "one\n")
  const model = scriptedProvider(turn(edit("notes.md", "one", "two")))
  const run = await startEvie([model.plugin, files(), breaks, undo({ dataFolder: home })], { home, projectFolder })
  cleanups.push(run.stop)

  await say(run, "change it")
  const said = (await run.evie.send({ type: "run_command", session: run.session, name: "undo", args: "" })).text

  expect(said).toStartWith("Left alone, because Evie was stopped part way through changing it: notes.md. What was there before is kept in ")
  const keptIn = said.slice(said.indexOf("kept in ") + "kept in ".length, -1)
  expect(await readFile(join(keptIn, "0"), "utf8")).toBe("one\n")
  expect(await readFile(join(projectFolder, "notes.md"), "utf8")).toBe("two\n")
})
