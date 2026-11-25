import { afterEach, expect, test } from "bun:test"
import { existsSync } from "node:fs"
import { mkdir, mkdtemp, realpath, rm, stat, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { registeredEarlier, startEvie, type Run } from "./support/harness.ts"
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

  expect(top).toEqual({
    entries: [
      { name: "files", path: "files", kind: "folder", changed: await changedAt(join(run.evieFolder, "files")) },
      { name: "Agenda.txt", path: "Agenda.txt", kind: "file", size: 6, changed: await changedAt(join(run.evieFolder, "Agenda.txt")) },
      { name: "evie.json", path: "evie.json", kind: "file", size: 22, changed: await changedAt(join(run.evieFolder, "evie.json")) },
    ],
    more: 0,
  })
  expect(inFiles).toEqual({
    entries: [
      { name: "receipts", path: "files/receipts", kind: "folder", changed: await changedAt(join(run.evieFolder, "files", "receipts")) },
      { name: "notes.md", path: "files/notes.md", kind: "file", size: 7, changed: await changedAt(join(run.evieFolder, "files", "notes.md")) },
    ],
    more: 0,
  })
})

test("a project with no .evie folder yet has nothing in Storage, and a folder that is not there is refused", async () => {
  const run = await start()

  expect(await run.evie.send({ type: "list_storage", project: run.project!.id })).toEqual({ entries: [], more: 0 })
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
  expect(top.entries.map((entry) => entry.name)).toEqual(["files", "shortcut"])
  expect((await run.evie.send({ type: "list_storage", project, folder: "shortcut" })).entries.map((entry) => entry.path)).toEqual(["shortcut/kept.txt"])
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

test("a .evie that is itself a link to another folder is refused, since the folder it leads to is not the project's", async () => {
  const run = await start()
  const outside = await tempFolder("evie-outside-")
  await writeFile(join(outside, "hosts"), "127.0.0.1 localhost\n")
  await symlink(outside, run.evieFolder)
  const project = run.project!.id
  const refused = `This project's .evie folder is a link to ${outside}, so Storage does not show it: it shows only a .evie folder of the project's own.`

  await expect(run.evie.send({ type: "list_storage", project })).rejects.toThrow(refused)
  await expect(run.evie.send({ type: "read_storage", project, path: "hosts" })).rejects.toThrow(refused)
})

test("Storage refuses a .evie folder that is evie's data folder, holds it, or is inside it, since the data folder holds the keys", async () => {
  const place = await tempFolder("evie-place-")
  const refused = "This project's .evie folder is, holds, or is inside evie's data folder, which holds its keys, so Storage does not show it."
  // Evie never makes a project at such a folder now, but one an earlier evie
  // made there is still refused. A project at the folder above the data
  // folder, as one at the home folder is above ~/.evie.
  const above = await startEvie([scriptedProvider([]).plugin], { home: join(place, ".evie") })
  cleanups.push(above.stop)
  const atPlace = registeredEarlier(above.home, place)
  await writeFile(join(place, ".evie", ".env"), "OPENROUTER_API_KEY=made-up\n")
  await expect(above.evie.send({ type: "list_storage", project: atPlace.id })).rejects.toThrow(refused)
  await expect(above.evie.send({ type: "read_storage", project: atPlace.id, path: ".env" })).rejects.toThrow(refused)

  // One whose .evie holds the data folder, and one inside the data folder.
  const holder = await tempFolder("evie-holder-")
  const holding = await startEvie([scriptedProvider([]).plugin], { home: join(holder, ".evie", "data") })
  cleanups.push(holding.stop)
  await expect(holding.evie.send({ type: "list_storage", project: registeredEarlier(holding.home, holder).id })).rejects.toThrow(refused)
  const home = await tempFolder("evie-home-")
  await mkdir(join(home, "kept", ".evie"), { recursive: true })
  const within = await startEvie([scriptedProvider([]).plugin], { home })
  cleanups.push(within.stop)
  await expect(within.evie.send({ type: "list_storage", project: registeredEarlier(home, join(home, "kept")).id })).rejects.toThrow(refused)
})

test("the .evie folder's own git repository is refused at any depth of path, and a folder is named the same however it is written", async () => {
  const run = await start()
  await mkdir(join(run.evieFolder, ".git"), { recursive: true })
  await writeFile(join(run.evieFolder, ".git", "config"), "[remote]\n")
  await mkdir(join(run.evieFolder, "files"), { recursive: true })
  await writeFile(join(run.evieFolder, "files", "notes.md"), "# Beds\n")
  const project = run.project!.id

  for (const folder of [".git", "./.git/", "files/../.git"]) {
    await expect(run.evie.send({ type: "list_storage", project, folder })).rejects.toThrow(
      `${folder} is the .evie folder's own git repository, which backs it up, and not one of its files.`,
    )
  }
  await expect(run.evie.send({ type: "read_storage", project, path: ".git/config" })).rejects.toThrow(
    ".git/config is the .evie folder's own git repository, which backs it up, and not one of its files.",
  )
  for (const folder of [".", "./", ""]) expect((await run.evie.send({ type: "list_storage", project, folder })).entries.map((entry) => entry.path)).toEqual(["files"])
  for (const folder of ["files/", "./files", "files//"]) expect((await run.evie.send({ type: "list_storage", project, folder })).entries.map((entry) => entry.path)).toEqual(["files/notes.md"])
  expect((await run.evie.send({ type: "read_storage", project, path: "./files//notes.md" })).path).toBe("files/notes.md")
})

test("a folder is listed up to 500 entries, by name, and says how many more it holds", async () => {
  const run = await start()
  await mkdir(join(run.evieFolder, "many"), { recursive: true })
  for (let at = 1; at <= 503; at++) await writeFile(join(run.evieFolder, "many", `note-${at}.md`), "")

  const listed = await run.evie.send({ type: "list_storage", project: run.project!.id, folder: "many" })

  expect(listed.entries.length).toBe(500)
  expect(listed.entries.at(-1)!.name).toBe("note-500.md")
  expect(listed.more).toBe(3)
})

test("a file, or a folder with everything in it, is deleted from Storage for good, and a folder's files are counted first", async () => {
  const run = await start()
  await mkdir(join(run.evieFolder, "youtube", "Karpathy"), { recursive: true })
  await mkdir(join(run.evieFolder, "youtube", "Veritasium", "Shorts"), { recursive: true })
  await writeFile(join(run.evieFolder, "youtube", "Karpathy", "Intro to LLMs.md"), "# Intro\n")
  await writeFile(join(run.evieFolder, "youtube", "Karpathy", "GPT from scratch.md"), "# GPT\n")
  await writeFile(join(run.evieFolder, "youtube", "Veritasium", "Shorts", "Why.md"), "# Why\n")
  await writeFile(join(run.evieFolder, "youtube", "index.md"), "# Saved\n")
  await writeFile(join(run.evieFolder, "notes.md"), "# Beds\n")
  const project = run.project!.id
  const top = async () => (await run.evie.send({ type: "list_storage", project })).entries.map((entry) => entry.path)

  expect(await run.evie.send({ type: "count_storage", project, path: "youtube" })).toEqual({ files: 4, folders: 3 })
  expect(await run.evie.send({ type: "count_storage", project, path: "notes.md" })).toEqual({ files: 1, folders: 0 })

  expect(await run.evie.send({ type: "delete_storage", project, path: "youtube/Karpathy/Intro to LLMs.md" })).toBeUndefined()
  expect((await run.evie.send({ type: "list_storage", project, folder: "youtube/Karpathy" })).entries.map((entry) => entry.name)).toEqual(["GPT from scratch.md"])
  await run.evie.send({ type: "delete_storage", project, path: "youtube" })
  expect(await top()).toEqual(["notes.md"])
  await run.evie.send({ type: "delete_storage", project, path: "./notes.md" })
  expect(await top()).toEqual([])
})

test("Storage deletes nothing outside the .evie folder, not the folder itself, nor its own git repository, nor what is not there", async () => {
  const run = await start()
  const outside = await tempFolder("evie-outside-")
  await writeFile(join(outside, "secret.txt"), "not yours\n")
  await mkdir(join(run.evieFolder, ".git"), { recursive: true })
  await writeFile(join(run.evieFolder, ".git", "config"), "[remote]\n")
  await writeFile(join(run.evieFolder, "notes.md"), "# Beds\n")
  await symlink(outside, join(run.evieFolder, "away"))
  await symlink(run.evieFolder, join(run.evieFolder, "self"))
  const project = run.project!.id
  const refusals: [string, string][] = [
    ["..", ".. is outside this project's .evie folder. Storage holds only what is inside it."],
    [join(outside, "secret.txt"), `${join(outside, "secret.txt")} is outside this project's .evie folder. Storage holds only what is inside it.`],
    ["away/secret.txt", "away/secret.txt is outside this project's .evie folder. Storage holds only what is inside it."],
    ["", "Storage deletes what is inside this project's .evie folder, never the folder itself."],
    [".", "Storage deletes what is inside this project's .evie folder, never the folder itself."],
    [".git", ".git is the .evie folder's own git repository, which backs it up, and not one of its files."],
    [".git/config", ".git/config is the .evie folder's own git repository, which backs it up, and not one of its files."],
    ["self/.git", "self/.git is the .evie folder's own git repository, which backs it up, and not one of its files."],
    ["self/.git/config", "self/.git/config is the .evie folder's own git repository, which backs it up, and not one of its files."],
    ["gone.txt", "There is no file or folder gone.txt in this project's .evie folder."],
    ["gone/notes.md", "There is no file or folder gone/notes.md in this project's .evie folder."],
    ["notes.md/inside", "There is no file or folder notes.md/inside in this project's .evie folder."],
  ]

  for (const [path, refused] of refusals) {
    await expect(run.evie.send({ type: "delete_storage", project, path })).rejects.toThrow(refused)
    await expect(run.evie.send({ type: "count_storage", project, path })).rejects.toThrow(refused)
  }
  expect(await Bun.file(join(outside, "secret.txt")).text()).toBe("not yours\n")
  expect(await Bun.file(join(run.evieFolder, ".git", "config")).text()).toBe("[remote]\n")
  expect(await Bun.file(join(run.evieFolder, "notes.md")).text()).toBe("# Beds\n")
})

test("a link in Storage is deleted itself, never what it leads to, and a file reached through a link inside the .evie folder is the file", async () => {
  const run = await start()
  const outside = await tempFolder("evie-outside-")
  await writeFile(join(outside, "secret.txt"), "not yours\n")
  await mkdir(join(run.evieFolder, "files"), { recursive: true })
  await writeFile(join(run.evieFolder, "files", "kept.txt"), "kept\n")
  await writeFile(join(run.evieFolder, "files", "old.txt"), "old\n")
  await symlink(join(run.evieFolder, "files"), join(run.evieFolder, "shortcut"))
  await symlink(outside, join(run.evieFolder, "away"))
  const project = run.project!.id

  expect(await run.evie.send({ type: "count_storage", project, path: "shortcut" })).toEqual({ files: 1, folders: 0 })
  await run.evie.send({ type: "delete_storage", project, path: "shortcut/old.txt" })
  await run.evie.send({ type: "delete_storage", project, path: "shortcut" })
  await run.evie.send({ type: "delete_storage", project, path: "away" })

  expect((await run.evie.send({ type: "list_storage", project })).entries.map((entry) => entry.path)).toEqual(["files"])
  expect((await run.evie.send({ type: "list_storage", project, folder: "files" })).entries.map((entry) => entry.path)).toEqual(["files/kept.txt"])
  expect(await Bun.file(join(outside, "secret.txt")).text()).toBe("not yours\n")
})

test("the .evie folder's own git repository is not listed or read through a link to the .evie folder either", async () => {
  const run = await start()
  await mkdir(join(run.evieFolder, ".git"), { recursive: true })
  await writeFile(join(run.evieFolder, ".git", "config"), "[remote]\n")
  await writeFile(join(run.evieFolder, "notes.md"), "# Beds\n")
  await symlink(run.evieFolder, join(run.evieFolder, "self"))
  await symlink(join(run.evieFolder, ".git"), join(run.evieFolder, "history"))
  const project = run.project!.id

  expect((await run.evie.send({ type: "list_storage", project })).entries.map((entry) => entry.path)).toEqual(["self", "notes.md"])
  expect((await run.evie.send({ type: "list_storage", project, folder: "self" })).entries.map((entry) => entry.path)).toEqual(["self/self", "self/notes.md"])
  await expect(run.evie.send({ type: "list_storage", project, folder: "self/.git" })).rejects.toThrow(
    "self/.git is the .evie folder's own git repository, which backs it up, and not one of its files.",
  )
  await expect(run.evie.send({ type: "read_storage", project, path: "self/.git/config" })).rejects.toThrow(
    "self/.git/config is the .evie folder's own git repository, which backs it up, and not one of its files.",
  )
  await expect(run.evie.send({ type: "read_storage", project, path: "history/config" })).rejects.toThrow(
    "history/config is the .evie folder's own git repository, which backs it up, and not one of its files.",
  )
})

test("the .evie folder's own git repository is not deleted by its name in other letters, as on a Mac, whose disk ignores case", async () => {
  const run = await start()
  await mkdir(join(run.evieFolder, ".git"), { recursive: true })
  await writeFile(join(run.evieFolder, ".git", "config"), "[remote]\n")
  const project = run.project!.id
  const ignoresCase = existsSync(join(run.evieFolder, ".GIT"))

  for (const path of [".GIT", ".Git", ".GIT/config"]) {
    const refused = ignoresCase ? `${path} is the .evie folder's own git repository, which backs it up, and not one of its files.` : `There is no file or folder ${path} in this project's .evie folder.`
    await expect(run.evie.send({ type: "count_storage", project, path })).rejects.toThrow(refused)
    await expect(run.evie.send({ type: "delete_storage", project, path })).rejects.toThrow(refused)
  }
  expect(await Bun.file(join(run.evieFolder, ".git", "config")).text()).toBe("[remote]\n")
})

test("a .git at the top of the .evie folder is never listed, even as a link to a folder inside it", async () => {
  const run = await start()
  await mkdir(join(run.evieFolder, "kept"), { recursive: true })
  await symlink(join(run.evieFolder, "kept"), join(run.evieFolder, ".git"))

  expect((await run.evie.send({ type: "list_storage", project: run.project!.id })).entries.map((entry) => entry.path)).toEqual(["kept"])
})
