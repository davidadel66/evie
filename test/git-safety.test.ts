import { afterEach, expect, test } from "bun:test"
import { chmod, mkdir, mkdtemp, readdir, realpath, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { git } from "./support/git.ts"
import { startEvie } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

// The git the core runs for a project, to keep its .evie folder out of the
// project's repository, and to say which branch it is on and whether it is
// in git at all, runs none of the commands the repository's own settings
// name.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function folder(prefix: string) {
  const made = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(made, { recursive: true, force: true }))
  return made
}

test("making a project, opening a session in it and listing projects runs nothing a repository's settings name", async () => {
  const noted = await folder("evie-ran-")
  const command = join(noted, "noted")
  await writeFile(command, `#!/bin/sh\ntouch "${noted}/$1"\ncat\n`)
  await chmod(command, 0o755)
  await mkdir(join(noted, "hooks"))
  await writeFile(join(noted, "hooks", "post-index-change"), `#!/bin/sh\ntouch "${noted}/hook"\n`)
  await chmod(join(noted, "hooks", "post-index-change"), 0o755)
  const repository = await folder("evie-repository-")
  await git(["init", "-q", "-b", "main"], repository)
  await writeFile(join(repository, "a.md"), "a\n")
  await writeFile(join(repository, ".gitattributes"), "* filter=evil\n")
  await git(["add", "-A"], repository)
  await git(["commit", "-q", "-m", "Start"], repository)
  for (const [name, value] of [
    ["core.fsmonitor", `${command} fsmonitor`],
    ["core.hooksPath", join(noted, "hooks")],
    ["filter.evil.clean", `${command} clean`],
    ["filter.evil.process", `${command} process`],
    ["core.pager", `${command} pager`],
  ]) {
    await git(["config", name!, value!], repository)
  }

  const run = await startEvie([scriptedProvider([]).plugin])
  cleanups.push(run.stop)
  const made = await run.evie.send({ type: "new_project", folder: join(repository, "inside") }).catch((cause: Error) => cause)
  // A folder below the top of a repository is refused, so the top is opened as a project instead.
  expect(made).toBeInstanceOf(Error)
  await run.evie.send({ type: "open_session", folder: repository })
  const listed = await run.evie.send({ type: "list_projects" })
  expect(listed.find((project) => project.folder === repository)).toMatchObject({ branch: "main", git: true })
  expect((await readdir(noted)).filter((name) => name !== "noted" && name !== "hooks")).toEqual([])
})
