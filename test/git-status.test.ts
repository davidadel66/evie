import { afterEach, expect, test } from "bun:test"
import { mkdtemp, realpath, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { git } from "./support/git.ts"
import { startEvie } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

// How a project's branch stands, as the web app's session details read it
// to decide what the git button offers and what the Changes row sums: the
// branch, work not committed, the remote and the upstream, the main
// branch, and the lines changed. Each case is a real repository, with a
// bare one beside it as its remote.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function folder(prefix: string) {
  const made = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(made, { recursive: true, force: true }))
  return made
}

// Evie in a project whose folder is a repository on main, with one commit.
async function start(options: { git?: boolean } = {}) {
  const projectFolder = await folder("evie-project-")
  await writeFile(join(projectFolder, "a.md"), "one\ntwo\n")
  if (options.git !== false) {
    await git(["init", "-q", "-b", "main"], projectFolder)
    await git(["add", "-A"], projectFolder)
    await git(["commit", "-q", "-m", "Start"], projectFolder)
  }
  const run = await startEvie([scriptedProvider([]).plugin], { home: await folder("evie-home-"), projectFolder })
  cleanups.push(run.stop)
  const read = () => run.evie.send({ type: "read_git_status", project: run.project!.id })
  return { run, projectFolder, read }
}

// A bare repository beside the project, as its remote, with main pushed to it.
async function withRemote(projectFolder: string) {
  const remote = await folder("evie-remote-")
  await git(["init", "-q", "--bare", "-b", "main"], remote)
  await git(["remote", "add", "origin", remote], projectFolder)
  await git(["push", "-q", "-u", "origin", "main"], projectFolder)
  return remote
}

test("a repository on main with no remote is level, with nothing to push and nothing changed", async () => {
  const { read } = await start()
  expect(await read()).toEqual({ branch: "main", uncommitted: false, remote: false, main: { name: "main", on: true, ahead: 0 }, changes: { against: null, added: 0, removed: 0 }, branches: ["main"] })
})

test("on main, the Changes row sums the work not committed, files git does not track included", async () => {
  const { read, projectFolder } = await start()
  await writeFile(join(projectFolder, "a.md"), "one\n2\nthree\n")
  await writeFile(join(projectFolder, "new.md"), "new\n")
  expect(await read()).toMatchObject({ uncommitted: true, changes: { against: null, added: 3, removed: 1 } })
})

test("a branch pushed to its remote says how far it is from its upstream and from main, and sums its changes against main, work not committed included", async () => {
  const { read, projectFolder } = await start()
  await withRemote(projectFolder)
  await git(["checkout", "-q", "-b", "feat/breads"], projectFolder)
  await writeFile(join(projectFolder, "a.md"), "one\ntwo\nthree\n")
  await git(["commit", "-q", "-am", "Add three"], projectFolder)
  await git(["push", "-q", "-u", "origin", "feat/breads"], projectFolder)

  const level = await read()
  expect(level).toEqual({
    branch: "feat/breads",
    uncommitted: false,
    remote: true,
    upstream: { ahead: 0, behind: 0 },
    main: { name: "main", on: false, ahead: 1 },
    changes: { against: "main", added: 1, removed: 0 },
    branches: expect.any(Array),
  })
  expect([...level!.branches].sort()).toEqual(["feat/breads", "main"])

  // A commit not pushed yet, and work not committed, which the changes count too.
  await writeFile(join(projectFolder, "a.md"), "one\ntwo\nthree\nfour\n")
  await git(["commit", "-q", "-am", "Add four"], projectFolder)
  await writeFile(join(projectFolder, "b.md"), "b\n")
  expect(await read()).toMatchObject({ uncommitted: true, upstream: { ahead: 1, behind: 0 }, main: { name: "main", on: false, ahead: 2 }, changes: { against: "main", added: 3, removed: 0 } })
})

test("a branch never pushed has no upstream, and one whose upstream has gone has none either", async () => {
  const { read, projectFolder } = await start()
  const remote = await withRemote(projectFolder)
  await git(["checkout", "-q", "-b", "feat/rolls"], projectFolder)
  await git(["commit", "-q", "--allow-empty", "-m", "Rolls"], projectFolder)
  const unpushed = await read()
  expect(unpushed).toMatchObject({ branch: "feat/rolls", remote: true, main: { name: "main", on: false, ahead: 1 } })
  expect(unpushed!.upstream).toBeUndefined()

  await git(["push", "-q", "-u", "origin", "feat/rolls"], projectFolder)
  expect((await read())!.upstream).toEqual({ ahead: 0, behind: 0 })
  await git(["branch", "-q", "-D", "feat/rolls"], remote)
  await git(["fetch", "-q", "--prune", "origin"], projectFolder)
  expect((await read())!.upstream).toBeUndefined()
})

test("a commit checked out by itself is on no branch", async () => {
  const { read, projectFolder } = await start()
  await git(["checkout", "-q", "--detach"], projectFolder)
  const status = await read()
  expect(status!.branch).toBeUndefined()
  expect(status).toMatchObject({ uncommitted: false, main: { name: "main", on: false, ahead: 0 } })
})

test("a folder that is not in a git repository has no git status", async () => {
  const { read } = await start({ git: false })
  expect(await read()).toBeNull()
})

test("the changes are counted against the branch given, as the diff panel is given its pull request's base, or against main when there is no such branch", async () => {
  const { run, projectFolder } = await start()
  await git(["branch", "dev"], projectFolder)
  await git(["checkout", "-q", "-b", "feat/breads"], projectFolder)
  await writeFile(join(projectFolder, "a.md"), "one\ntwo\nthree\n")
  await git(["commit", "-q", "-am", "Add three"], projectFolder)
  await git(["checkout", "-q", "dev"], projectFolder)
  await writeFile(join(projectFolder, "c.md"), "c\n")
  await git(["add", "c.md"], projectFolder)
  await git(["commit", "-q", "-m", "Add c"], projectFolder)
  await git(["checkout", "-q", "feat/breads"], projectFolder)

  const read = (base?: string) => run.evie.send({ type: "read_git_status", project: run.project!.id, ...(base !== undefined && { base }) })
  expect((await read("dev"))!.changes).toEqual({ against: "dev", added: 1, removed: 0 })
  expect((await read("nowhere"))!.changes).toEqual({ against: "main", added: 1, removed: 0 })
  expect((await read())!.changes).toEqual({ against: "main", added: 1, removed: 0 })
})
