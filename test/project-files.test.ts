import { afterEach, expect, test } from "bun:test"
import { chmod, mkdir, mkdtemp, readdir, realpath, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { listProjectFiles } from "../src/core/project-files.ts"
import { git } from "./support/git.ts"
import { startEvie } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

// The files of a project the owner can mention with @ in the message box:
// what git would list, never its own folder or node_modules, and only so
// many of them.

// A data folder elsewhere, for the listings that are not about it.
const DATA = "/nowhere/evie-data"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function folderWith(files: string[]): Promise<string> {
  const folder = await realpath(await mkdtemp(join(tmpdir(), "evie-files-")))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  for (const file of files) {
    await mkdir(join(folder, file, ".."), { recursive: true })
    await writeFile(join(folder, file), "x\n")
  }
  return folder
}

test("in a git repository, the files are those git lists, tracked or not, with what it ignores left out", async () => {
  const folder = await folderWith([".gitignore", "README.md", "src/app.ts", "src/app.test.ts", "notes/draft.md", "dist/app.js", "debug.log", "node_modules/left/index.js"])
  await writeFile(join(folder, ".gitignore"), "dist/\n*.log\n")
  await git(["init", "-q"], folder)
  await git(["add", ".gitignore", "README.md", "src"], folder)
  await git(["commit", "-q", "-m", "Start"], folder)
  // A package folder that someone committed, which is still not offered.
  await git(["add", "-f", "node_modules"], folder)

  const listed = await listProjectFiles(folder, { dataFolder: DATA })

  expect(listed.more).toBe(false)
  expect([...listed.files].sort()).toEqual([".gitignore", "README.md", "notes/draft.md", "src/app.test.ts", "src/app.ts"])
})

test("outside git, every file is listed, the nearest first, but git's folder, node_modules, the .evie folder, evie's data folder and private folders such as .ssh", async () => {
  const folder = await folderWith([
    "plan.md",
    "receipts/2026/march.pdf",
    "receipts/april.pdf",
    ".git/config",
    "node_modules/left/index.js",
    "site/node_modules/x.js",
    ".evie/evie.json",
    ".ssh/id_ed25519",
    ".aws/credentials",
    ".gnupg/pubring.kbx",
    "evie-data/.env",
  ])

  const listed = await listProjectFiles(folder, { dataFolder: join(folder, "evie-data") })

  expect(listed).toEqual({ files: ["plan.md", "receipts/april.pdf", "receipts/2026/march.pdf"], more: false })
})

test("in git too, a private folder that the repository commits is left out", async () => {
  const folder = await folderWith(["README.md", ".ssh/config", ".evie/skills/plan.md"])
  await git(["init", "-q"], folder)
  await git(["add", "-f", "."], folder)

  expect((await listProjectFiles(folder, { dataFolder: DATA })).files).toEqual(["README.md"])
})

test("git is asked with none of the commands a repository's settings name, and a listing cut short by its time limit says there are more", async () => {
  const folder = await folderWith(["README.md"])
  const noted = join(folder, "..", `${folder.split("/").at(-1)}-ran`)
  await mkdir(noted)
  cleanups.push(() => rm(noted, { recursive: true, force: true }))
  await writeFile(join(noted, "fsmonitor"), `#!/bin/sh\ntouch "${noted}/ran"\n`)
  await chmod(join(noted, "fsmonitor"), 0o755)
  await git(["init", "-q"], folder)
  await git(["config", "core.fsmonitor", join(noted, "fsmonitor")], folder)

  expect(await listProjectFiles(folder, { dataFolder: DATA })).toEqual({ files: ["README.md"], more: false })
  expect(await readdir(noted)).toEqual(["fsmonitor"])
  expect((await listProjectFiles(folder, { dataFolder: DATA, gitWithin: 1 })).more).toBe(true)
})

test("only so many files are listed, and the listing says there are more", async () => {
  const many = Array.from({ length: 30 }, (_, at) => `file-${String(at).padStart(2, "0")}.md`)
  const plain = await folderWith(many)
  const repository = await folderWith(many)
  await git(["init", "-q"], repository)

  for (const folder of [plain, repository]) {
    const listed = await listProjectFiles(folder, { dataFolder: DATA, limit: 10 })
    expect(listed.files).toHaveLength(10)
    expect(listed.more).toBe(true)
  }
  expect((await listProjectFiles(plain, { dataFolder: DATA, limit: 30 })).more).toBe(false)
})

test("a client lists a project's files by its id", async () => {
  const run = await startEvie([scriptedProvider([]).plugin])
  cleanups.push(run.stop)
  await writeFile(join(run.projectFolder, "plan.md"), "Beds.\n")

  expect(await run.evie.send({ type: "list_files", project: run.project!.id })).toEqual({ files: ["plan.md"], more: false })
  await expect(run.evie.send({ type: "list_files", project: "no-such-project" })).rejects.toThrow('There is no project with the id "no-such-project".')
})
