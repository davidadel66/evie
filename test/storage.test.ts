import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, realpath, rm, stat, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { startEvie, type Run } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

// A workspace's Storage: the files in its project's .evie folder, listed a
// folder at a time and read one at a time, never anything outside it.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function tempFolder(prefix: string) {
  const folder = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

async function start(): Promise<Run & { evieFolder: string }> {
  const run = await startEvie([scriptedProvider([]).plugin])
  cleanups.push(run.stop)
  return { ...run, evieFolder: join(run.projectFolder, ".evie") }
}

const changedAt = async (path: string) => Math.floor((await stat(path)).mtimeMs)

test("Storage lists what a project's .evie folder holds, folders first, each file with its size, each with when it changed, and leaves out its own git repository", async () => {
  const run = await start()
  await mkdir(join(run.evieFolder, "files", "receipts"), { recursive: true })
  await mkdir(join(run.evieFolder, ".git"))
  await writeFile(join(run.evieFolder, "evie.json"), '{ "sync": "private" }\n')
  await writeFile(join(run.evieFolder, "files", "notes.md"), "# Beds\n")
  await writeFile(join(run.evieFolder, "Agenda.txt"), "plant\n")

  const top = await run.evie.send({ type: "list_storage", project: run.project!.id })
  const inFiles = await run.evie.send({ type: "list_storage", project: run.project!.id, folder: "files" })

  expect(top).toEqual([
    { name: "files", path: "files", kind: "folder", changed: await changedAt(join(run.evieFolder, "files")) },
    { name: "Agenda.txt", path: "Agenda.txt", kind: "file", size: 6, changed: await changedAt(join(run.evieFolder, "Agenda.txt")) },
    { name: "evie.json", path: "evie.json", kind: "file", size: 22, changed: await changedAt(join(run.evieFolder, "evie.json")) },
  ])
  expect(inFiles).toEqual([
    { name: "receipts", path: "files/receipts", kind: "folder", changed: await changedAt(join(run.evieFolder, "files", "receipts")) },
    { name: "notes.md", path: "files/notes.md", kind: "file", size: 7, changed: await changedAt(join(run.evieFolder, "files", "notes.md")) },
  ])
})

test("a project with no .evie folder yet has nothing in Storage, and a folder that is not there is refused", async () => {
  const run = await start()

  expect(await run.evie.send({ type: "list_storage", project: run.project!.id })).toEqual([])
  await expect(run.evie.send({ type: "list_storage", project: run.project!.id, folder: "files" })).rejects.toThrow(
    "There is no folder files in this project's .evie folder.",
  )
  await expect(run.evie.send({ type: "list_storage", project: "no-such-project" })).rejects.toThrow('There is no project with the id "no-such-project".')
})

test("Storage refuses a folder outside the .evie folder: one that climbs out, a full path, and a link that leads out, which is not listed", async () => {
  const run = await start()
  const outside = await tempFolder("evie-outside-")
  await writeFile(join(outside, "secret.txt"), "not yours\n")
  await mkdir(join(run.evieFolder, "files"), { recursive: true })
  await writeFile(join(run.evieFolder, "files", "kept.txt"), "kept\n")
  await symlink(outside, join(run.evieFolder, "away"))
  await symlink(join(run.evieFolder, "files"), join(run.evieFolder, "shortcut"))
  const project = run.project!.id

  for (const folder of ["..", "files/../..", outside, "away"]) {
    await expect(run.evie.send({ type: "list_storage", project, folder })).rejects.toThrow(
      `${folder} is outside this project's .evie folder. Storage holds only what is inside it.`,
    )
  }
  const top = await run.evie.send({ type: "list_storage", project })
  expect(top.map((entry) => entry.name)).toEqual(["files", "shortcut"])
  expect((await run.evie.send({ type: "list_storage", project, folder: "shortcut" })).map((entry) => entry.path)).toEqual(["shortcut/kept.txt"])
})

test("a text file in Storage is read as its text, and an image as its bytes with their type", async () => {
  const run = await start()
  await mkdir(join(run.evieFolder, "files"), { recursive: true })
  await writeFile(join(run.evieFolder, "files", "notes.md"), "# Beds\n\nTomatoes by the wall.\n")
  // The eight bytes every PNG file starts with, and a little more.
  const png = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 13])
  await writeFile(join(run.evieFolder, "files", "Plan.PNG"), png)
  const project = run.project!.id

  expect(await run.evie.send({ type: "read_storage", project, path: "files/notes.md" })).toEqual({
    path: "files/notes.md",
    size: 30,
    changed: await changedAt(join(run.evieFolder, "files", "notes.md")),
    kind: "text",
    text: "# Beds\n\nTomatoes by the wall.\n",
  })
  expect(await run.evie.send({ type: "read_storage", project, path: "files/Plan.PNG" })).toEqual({
    path: "files/Plan.PNG",
    size: 12,
    changed: await changedAt(join(run.evieFolder, "files", "Plan.PNG")),
    kind: "image",
    type: "image/png",
    data: png.toString("base64"),
  })
})

test("Storage reads no file over 1 MB, none that is neither text nor an image, no folder and nothing outside the .evie folder", async () => {
  const run = await start()
  const outside = await tempFolder("evie-outside-")
  await writeFile(join(outside, "secret.txt"), "not yours\n")
  await mkdir(join(run.evieFolder, "files"), { recursive: true })
  await writeFile(join(run.evieFolder, "files", "big.txt"), "a".repeat(1024 * 1024 + 1))
  await writeFile(join(run.evieFolder, "files", "just.txt"), "a".repeat(1024 * 1024))
  await writeFile(join(run.evieFolder, "files", "tool.bin"), Buffer.from([0x7f, 0x45, 0x4c, 0x46, 0, 1]))
  await symlink(join(outside, "secret.txt"), join(run.evieFolder, "files", "secret.txt"))
  const project = run.project!.id
  const read = (path: string) => run.evie.send({ type: "read_storage", project, path })

  expect((await read("files/just.txt")).size).toBe(1024 * 1024)
  await expect(read("files/big.txt")).rejects.toThrow("files/big.txt is larger than the 1 MB Storage shows. Open it on the computer evie runs on instead.")
  await expect(read("files/tool.bin")).rejects.toThrow("files/tool.bin is neither text nor an image, so Storage cannot show it.")
  await expect(read("files")).rejects.toThrow("There is no file files in this project's .evie folder.")
  await expect(read("files/gone.txt")).rejects.toThrow("There is no file files/gone.txt in this project's .evie folder.")
  for (const path of ["../secret.txt", join(outside, "secret.txt"), "files/secret.txt"]) {
    await expect(read(path)).rejects.toThrow(`${path} is outside this project's .evie folder. Storage holds only what is inside it.`)
  }
})
