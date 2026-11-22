import { afterEach, expect, test } from "bun:test"
import { chmod, mkdir, mkdtemp, readdir, realpath, rm, unlink, utimes, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { files, undo, type Plugin } from "../src/index.ts"
import { git } from "./support/git.ts"
import { say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedCall, type ScriptedReply } from "./support/scripted-provider.ts"

// What changed in a session, as a client is shown it: each turn's changed
// files from the copies undo keeps, the whole session's, and with git, the
// uncommitted changes and the branch's. The files are changed by the real
// file tools, as the scripted model asks.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

const write = (path: string, content: string): ScriptedCall => ({ name: "write", input: { path, content } })
const edit = (path: string, oldString: string, newString: string): ScriptedCall => ({ name: "edit", input: { path, oldString, newString } })
const patch = (patchText: string): ScriptedCall => ({ name: "patch", input: { patchText: `*** Begin Patch\n${patchText}\n*** End Patch` } })
// One turn in which the model asks for these tools, one after another, and then answers.
const turn = (...calls: ScriptedCall[]): ScriptedReply[] => [...calls.map((call) => ({ text: [], calls: [call] })), { text: ["Done."] }]

// Evie with the file tools, and undo unless it is left out, in a temporary
// project holding the given files.
// With `git`, the folder is a repository on main, with the files committed.
async function start(script: ScriptedReply[], starting: Record<string, string> = {}, options: { undo?: boolean; git?: boolean; plugins?: Plugin[] } = {}) {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  const projectFolder = await realpath(await mkdtemp(join(tmpdir(), "evie-project-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }), () => rm(projectFolder, { recursive: true, force: true }))
  for (const [name, text] of Object.entries(starting)) await writeFile(join(projectFolder, name), text)
  if (options.git) {
    await git(["init", "-q", "-b", "main"], projectFolder)
    await git(["add", "-A"], projectFolder)
    await git(["commit", "-q", "-m", "Start"], projectFolder)
  }
  const plugins: Plugin[] = [scriptedProvider(script).plugin, files(), ...(options.undo === false ? [] : [undo({ dataFolder: home })]), ...(options.plugins ?? [])]
  const run = await startEvie(plugins, { home, projectFolder })
  cleanups.push(run.stop)
  return { run, projectFolder }
}

test("a turn's changed files are listed with how many lines each added and removed, and its diff can be read", async () => {
  const { run } = await start(turn(edit("notes.md", "two", "2")), { "notes.md": "one\ntwo\nthree\n" })
  await say(run, "change it")

  const listed = await run.evie.send({ type: "list_changes", session: run.session })
  expect(listed.kept).toBe(true)
  expect(listed.latest).toBe(1)
  expect(listed.turns.map(({ turn, files }) => ({ turn, files }))).toEqual([{ turn: 1, files: [{ path: "notes.md", status: "changed", added: 1, removed: 1 }] }])
  // The turn is known by its start's and end's events, and dated by its end.
  const events = stored(run.events)
  const end = events.find((event) => event.type === "turn_end")!
  expect(listed.turns[0]).toMatchObject({ start: events.find((event) => event.type === "turn_start")!.seq, end: end.seq, time: end.time })

  const read = await run.evie.send({ type: "read_changes", session: run.session, of: "turn", turn: 1 })
  expect(read.files).toEqual([{ path: "notes.md", status: "changed", added: 1, removed: 1, patch: "@@ -1,3 +1,3 @@\n one\n-two\n+2\n three\n" }])
})

test("a file a turn added, one it removed and one it changed each say so, with their lines", async () => {
  const { run } = await start(turn(write("new.md", "fresh\nlines\n"), patch("*** Delete File: old.md"), edit("kept.md", "a", "b")), { "old.md": "gone\n", "kept.md": "a\n" })
  await say(run, "tidy up")

  const read = await run.evie.send({ type: "read_changes", session: run.session, of: "turn", turn: 1 })
  expect(read.files).toEqual([
    { path: "new.md", status: "added", added: 2, removed: 0, patch: "@@ -0,0 +1,2 @@\n+fresh\n+lines\n" },
    { path: "old.md", status: "removed", added: 0, removed: 1, patch: "@@ -1,1 +0,0 @@\n-gone\n" },
    { path: "kept.md", status: "changed", added: 1, removed: 1, patch: "@@ -1,1 +1,1 @@\n-a\n+b\n" },
  ])
})

test("each turn shows what it changed, even once a later turn has changed the same file again", async () => {
  const { run } = await start([...turn(edit("a.md", "1", "2")), { text: ["Just talking."] }, ...turn(edit("a.md", "2", "3"))], { "a.md": "1\n" })
  await say(run, "first")
  await say(run, "a turn that changes nothing")
  await say(run, "second")

  const listed = await run.evie.send({ type: "list_changes", session: run.session })
  expect(listed.latest).toBe(3)
  expect(listed.turns.map((each) => each.turn)).toEqual([1, 3])
  const first = await run.evie.send({ type: "read_changes", session: run.session, of: "turn", turn: 1 })
  expect(first.files[0]?.patch).toBe("@@ -1,1 +1,1 @@\n-1\n+2\n")
  const second = await run.evie.send({ type: "read_changes", session: run.session, of: "turn", turn: 3 })
  expect(second.files[0]?.patch).toBe("@@ -1,1 +1,1 @@\n-2\n+3\n")
  // A turn that changed nothing has nothing to show.
  expect((await run.evie.send({ type: "read_changes", session: run.session, of: "turn", turn: 2 })).files).toEqual([])
})

test("the whole session runs from before each file's first change to the file as it is now", async () => {
  const { run, projectFolder } = await start([...turn(edit("a.md", "1", "2"), write("b.md", "b\n")), ...turn(edit("a.md", "2", "3"))], { "a.md": "1\n" })
  await say(run, "first")
  await say(run, "second")
  // The owner changes it since, by hand.
  await writeFile(join(projectFolder, "a.md"), "4\n")

  const read = await run.evie.send({ type: "read_changes", session: run.session, of: "session" })
  expect(read.files).toEqual([
    { path: "a.md", status: "changed", added: 1, removed: 1, patch: "@@ -1,1 +1,1 @@\n-1\n+4\n" },
    { path: "b.md", status: "added", added: 1, removed: 0, patch: "@@ -0,0 +1,1 @@\n+b\n" },
  ])
  const listed = await run.evie.send({ type: "list_changes", session: run.session, whole: true })
  expect(listed.session).toEqual([
    { path: "a.md", status: "changed", added: 1, removed: 1 },
    { path: "b.md", status: "added", added: 1, removed: 0 },
  ])
  // What the last turn left in a.md is no longer there, and no copy of it is kept.
  expect(listed.turns.at(-1)?.files).toEqual([{ path: "a.md", status: "changed", added: 0, removed: 0, unknown: true }])
  // The first turn's is: the second turn kept a copy of it.
  expect(listed.turns[0]?.files).toEqual([
    { path: "a.md", status: "changed", added: 1, removed: 1 },
    { path: "b.md", status: "added", added: 1, removed: 0 },
  ])
})

test("a binary file, and one too large to compare, say so instead of a diff", async () => {
  const large = "a line of text\n".repeat(80_000)
  const { run } = await start(turn(write("picture.png", "PNG\u0000\u0001data"), write("large.txt", large)))
  await say(run, "make them")

  const read = await run.evie.send({ type: "read_changes", session: run.session, of: "turn", turn: 1 })
  expect(read.files).toEqual([
    { path: "picture.png", status: "added", added: 0, removed: 0, binary: true },
    { path: "large.txt", status: "added", added: 0, removed: 0, tooLarge: true },
  ])
})

test("a long file added is shown whole, however many lines it has", async () => {
  const long = Array.from({ length: 5000 }, (_, at) => `line ${at + 1}`).join("\n") + "\n"
  const { run } = await start(turn(write("long.txt", long)))
  await say(run, "make it")

  const [file] = (await run.evie.send({ type: "read_changes", session: run.session, of: "turn", turn: 1 })).files
  expect(file).toMatchObject({ path: "long.txt", status: "added", added: 5000, removed: 0 })
  expect(file?.patch?.split("\n").slice(0, 2)).toEqual(["@@ -0,0 +1,5000 @@", "+line 1"])
})

test("without undo, nothing keeps the files turns change, and the list says so", async () => {
  const { run } = await start(turn(write("new.md", "fresh\n")), {}, { undo: false })
  await say(run, "make it")

  expect(await run.evie.send({ type: "list_changes", session: run.session, whole: true })).toEqual({ kept: false, latest: 1, turns: [], session: [] })
  expect(await run.evie.send({ type: "read_changes", session: run.session, of: "turn", turn: 1 })).toEqual({ files: [] })
})

test("in a git project, the uncommitted work is every change since the last commit, untracked files included", async () => {
  const { run, projectFolder } = await start(turn(edit("a.md", "1", "2")), { "a.md": "1\n", "c.md": "c\n", ".gitignore": "*.log\n" }, { git: true })
  await say(run, "change it")
  // The owner, by hand: a new file, a file removed, and one git ignores.
  await writeFile(join(projectFolder, "u.md"), "u\n")
  await unlink(join(projectFolder, "c.md"))
  await writeFile(join(projectFolder, "debug.log"), "noise\n")

  const read = await run.evie.send({ type: "read_changes", session: run.session, of: "uncommitted" })
  expect(read.files).toEqual([
    { path: "a.md", status: "changed", added: 1, removed: 1, patch: "@@ -1,1 +1,1 @@\n-1\n+2\n" },
    { path: "c.md", status: "removed", added: 0, removed: 1, patch: "@@ -1,1 +0,0 @@\n-c\n" },
    { path: "u.md", status: "added", added: 1, removed: 0, patch: "@@ -0,0 +1,1 @@\n+u\n" },
  ])
})

test("past a few megabytes of diffs in one answer, the rest of the files are too large to show", async () => {
  const { run, projectFolder } = await start([], { "start.md": "start\n" }, { git: true })
  const big = "x".repeat(99) + "\n"
  for (const name of ["1", "2", "3", "4", "5", "6"]) await writeFile(join(projectFolder, `${name}.txt`), big.repeat(9000))

  const read = await run.evie.send({ type: "read_changes", session: run.session, of: "uncommitted" })
  expect(read.files.map((file) => (file.patch !== undefined ? "shown" : file.tooLarge ? "too large" : "?"))).toEqual(["shown", "shown", "shown", "shown", "too large", "too large"])
  // Its lines were counted all the same.
  expect(read.files[5]).toEqual({ path: "6.txt", status: "added", added: 9000, removed: 0, tooLarge: true })
})

test("past 300 files in one comparison, the rest are listed as too many, without being read", async () => {
  const { run, projectFolder } = await start([], { "start.md": "start\n" }, { git: true })
  for (let at = 1; at <= 305; at += 1) await writeFile(join(projectFolder, `${String(at).padStart(3, "0")}.txt`), `${at}\n`)

  const read = await run.evie.send({ type: "read_changes", session: run.session, of: "uncommitted" })
  expect(read.files).toHaveLength(305)
  expect(read.files[299]).toEqual({ path: "300.txt", status: "added", added: 1, removed: 0, patch: "@@ -0,0 +1,1 @@\n+300\n" })
  expect(read.files.slice(300)).toEqual(["301", "302", "303", "304", "305"].map((name) => ({ path: `${name}.txt`, status: "added", added: 0, removed: 0, tooMany: true })))
})

test("a folder that is not in a git repository has no uncommitted work or branch to compare, which the answer says in words", async () => {
  const { run } = await start([])
  for (const of of ["uncommitted", "branch"] as const) {
    expect(await run.evie.send({ type: "read_changes", session: run.session, of })).toEqual({ files: [], unavailable: "The project's folder is not in a git repository." })
  }
})

test("a branch's changes run from where it left its base to the folder as it is now, uncommitted work included", async () => {
  const { run, projectFolder } = await start(turn(write("b.md", "b\n")), { "a.md": "1\n", "c.md": "c\n" }, { git: true })
  // A branch with a commit of its own, made after main has moved on.
  await git(["checkout", "-q", "-b", "feature"], projectFolder)
  await writeFile(join(projectFolder, "a.md"), "2\n")
  await git(["commit", "-q", "-am", "Change a"], projectFolder)
  await git(["checkout", "-q", "main"], projectFolder)
  await writeFile(join(projectFolder, "c.md"), "changed on main\n")
  await git(["commit", "-q", "-am", "Change c"], projectFolder)
  await git(["checkout", "-q", "feature"], projectFolder)
  await say(run, "add b")

  const read = await run.evie.send({ type: "read_changes", session: run.session, of: "branch" })
  expect(read.base).toBe("main")
  expect(read.files).toEqual([
    { path: "a.md", status: "changed", added: 1, removed: 1, patch: "@@ -1,1 +1,1 @@\n-1\n+2\n" },
    { path: "b.md", status: "added", added: 1, removed: 0, patch: "@@ -0,0 +1,1 @@\n+b\n" },
  ])

  // Against another base, named.
  await git(["branch", "dev", "HEAD~1"], projectFolder)
  const againstDev = await run.evie.send({ type: "read_changes", session: run.session, of: "branch", base: "dev" })
  expect(againstDev.base).toBe("dev")
  expect(againstDev.files.map((file) => file.path)).toEqual(["a.md", "b.md"])
  await git(["branch", "-f", "dev", "HEAD"], projectFolder)
  expect((await run.evie.send({ type: "read_changes", session: run.session, of: "branch", base: "dev" })).files.map((file) => file.path)).toEqual(["b.md"])

  expect(await run.evie.send({ type: "read_changes", session: run.session, of: "branch", base: "nowhere" })).toEqual({ files: [], unavailable: 'There is no branch "nowhere" to compare this one with.' })
})

test("reading a repository's changes runs none of the commands the repository's own settings name", async () => {
  const { run, projectFolder } = await start([], { "a.md": "a\n", "b.md": "b\n", ".gitattributes": "* diff=evil filter=evil\n" }, { git: true })
  // A command that notes each time it is run, by the setting that ran it,
  // and passes what it is given through, as a filter would.
  const ran = await realpath(await mkdtemp(join(tmpdir(), "evie-ran-")))
  cleanups.push(() => rm(ran, { recursive: true, force: true }))
  const command = join(ran, "noted")
  await writeFile(command, `#!/bin/sh\ntouch "${ran}/$1"\ncat\n`)
  await chmod(command, 0o755)
  await mkdir(join(ran, "hooks"))
  await writeFile(join(ran, "hooks", "post-index-change"), `#!/bin/sh\ntouch "${ran}/hook"\n`)
  await chmod(join(ran, "hooks", "post-index-change"), 0o755)
  const settings: [string, string][] = [
    ["core.fsmonitor", `${command} fsmonitor`],
    ["core.hooksPath", join(ran, "hooks")],
    ["core.pager", `${command} pager`],
    ["pager.diff", `${command} pager`],
    ["diff.external", `${command} external`],
    ["diff.evil.command", `${command} diff-driver`],
    ["diff.evil.textconv", `${command} textconv`],
    ["filter.evil.clean", `${command} clean`],
    ["filter.evil.smudge", `${command} smudge`],
    ["filter.evil.process", `${command} process`],
    ["filter.evil.required", "true"],
  ]
  for (const [name, value] of settings) await git(["config", name, value], projectFolder)
  // One file changed, and one rewritten as it was, so git must look inside both.
  await writeFile(join(projectFolder, "b.md"), "b changed\n")
  await writeFile(join(projectFolder, "a.md"), "a\n")
  await utimes(join(projectFolder, "a.md"), new Date(), new Date(Date.now() + 5000))

  const uncommitted = await run.evie.send({ type: "read_changes", session: run.session, of: "uncommitted" })
  await run.evie.send({ type: "read_changes", session: run.session, of: "branch" })
  await run.evie.send({ type: "list_changes", session: run.session })
  expect(uncommitted.files.map((file) => file.path)).toEqual(["b.md"])
  expect((await readdir(ran)).filter((name) => name !== "noted" && name !== "hooks").sort()).toEqual([])
})

test("a file whose name holds a line break is not known, and the files after it are read as they are", async () => {
  const { run, projectFolder } = await start([], { "a\nb.md": "one\n", "c.md": "c\n", "b.md": "b\n" }, { git: true })
  await writeFile(join(projectFolder, "a\nb.md"), "two\n")
  await writeFile(join(projectFolder, "b.md"), "b changed\n")
  await writeFile(join(projectFolder, "c.md"), "c changed\n")

  const read = await run.evie.send({ type: "read_changes", session: run.session, of: "uncommitted" })
  expect(read.files).toEqual([
    { path: "a\nb.md", status: "changed", added: 0, removed: 0, unknown: true },
    { path: "b.md", status: "changed", added: 1, removed: 1, patch: "@@ -1,1 +1,1 @@\n-b\n+b changed\n" },
    { path: "c.md", status: "changed", added: 1, removed: 1, patch: "@@ -1,1 +1,1 @@\n-c\n+c changed\n" },
  ])
})

test("a submodule moved to another commit is changed, with nothing to compare inside it", async () => {
  const { run, projectFolder } = await start([], { "a.md": "a\n" }, { git: true })
  const sub = join(projectFolder, "sub")
  await mkdir(sub)
  await git(["init", "-q", "-b", "main"], sub)
  await writeFile(join(sub, "inside.md"), "one\n")
  await git(["add", "-A"], sub)
  await git(["commit", "-q", "-m", "One"], sub)
  await git(["add", "sub"], projectFolder)
  await git(["commit", "-q", "-m", "Add sub"], projectFolder)
  await writeFile(join(sub, "inside.md"), "two\n")
  await git(["commit", "-q", "-am", "Two"], sub)

  const read = await run.evie.send({ type: "read_changes", session: run.session, of: "uncommitted" })
  expect(read.files).toEqual([{ path: "sub", status: "changed", added: 0, removed: 0, unknown: true, submodule: true }])
})

test("a file taken out of git but left in the folder is removed, as git counts it, not added", async () => {
  const { run, projectFolder } = await start([], { "a.md": "a\n", "c.md": "c\n" }, { git: true })
  await git(["rm", "-q", "--cached", "c.md"], projectFolder)

  const read = await run.evie.send({ type: "read_changes", session: run.session, of: "uncommitted" })
  expect(read.files).toEqual([{ path: "c.md", status: "removed", added: 0, removed: 1, patch: "@@ -1,1 +0,0 @@\n-c\n" }])
})

test("without a base, a branch is compared with the base of its pull request, as the plugin that reads them gives it", async () => {
  const pullRequest: Plugin = {
    name: "stand-in",
    setup(host) {
      host.pullRequests.add({ name: "stand-in", read: async () => ({ number: 7, title: "Feature", state: "open", base: "dev", checks: [] }) })
    },
  }
  const { run, projectFolder } = await start([], { "a.md": "1\n" }, { git: true, plugins: [pullRequest] })
  await git(["branch", "dev"], projectFolder)
  await git(["checkout", "-q", "-b", "feature"], projectFolder)
  await writeFile(join(projectFolder, "a.md"), "2\n")
  await git(["commit", "-q", "-am", "Change a"], projectFolder)
  // main moves on, so comparing with it would show its change too.
  await git(["checkout", "-q", "main"], projectFolder)
  await writeFile(join(projectFolder, "main-only.md"), "m\n")
  await git(["add", "-A"], projectFolder)
  await git(["commit", "-q", "-m", "On main"], projectFolder)
  await git(["checkout", "-q", "dev"], projectFolder)
  await git(["merge", "-q", "main"], projectFolder)
  await git(["checkout", "-q", "feature"], projectFolder)

  const read = await run.evie.send({ type: "read_changes", session: run.session, of: "branch" })
  expect(read.base).toBe("dev")
})

test("a finished turn's files are read and compared once, and the whole session only when it is asked for", async () => {
  const read: number[] = []
  let projectFolder = ""
  const keeper: Plugin = {
    name: "keeper",
    setup(host) {
      host.changes.add({
        name: "keeper",
        turns: async () => [{ turn: 1, paths: [join(projectFolder, "a.md")] }],
        files: async (_session, turn) => {
          read.push(turn)
          return [{ path: join(projectFolder, "a.md"), before: Buffer.from("1\n"), after: Buffer.from("2\n") }]
        },
      })
    },
  }
  const started = await start([{ text: ["Nothing to do."] }], { "a.md": "2\n" }, { undo: false, plugins: [keeper] })
  projectFolder = started.projectFolder
  const { run } = started
  await say(run, "a turn")

  const once = await run.evie.send({ type: "list_changes", session: run.session })
  const twice = await run.evie.send({ type: "list_changes", session: run.session })
  expect(twice.turns).toEqual(once.turns)
  expect(once.turns[0]?.files).toEqual([{ path: "a.md", status: "changed", added: 1, removed: 1 }])
  expect(read).toEqual([1])
  // The whole session is left out until it is asked for.
  expect(once.session).toBeUndefined()
  expect((await run.evie.send({ type: "list_changes", session: run.session, whole: true })).session).toEqual([{ path: "a.md", status: "changed", added: 1, removed: 1 }])
})
