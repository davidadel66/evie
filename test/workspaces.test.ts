import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, readFile, realpath, rm, stat, writeFile } from "node:fs/promises"
import { homedir, tmpdir } from "node:os"
import { basename, join } from "node:path"
import { loadSettingsFiles, type Plugin } from "../src/index.ts"
import { git } from "./support/git.ts"
import { nextEvent, startEvie, stored, type Run } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function tempFolder(prefix: string) {
  const folder = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

const exists = (path: string) => stat(path).then(() => true, () => false)

async function start() {
  const run: Run = await startEvie([scriptedProvider([]).plugin])
  cleanups.push(run.stop)
  return run
}

// Evie started the way `evie serve` starts it, with its settings files, whose
// global one lists no plugins. `plugins` are loaded as well.
async function startWithSettings(plugins: Plugin[] = []) {
  const home = await tempFolder("evie-home-")
  await writeFile(join(home, "evie.json"), JSON.stringify({ plugins: [] }))
  const settings = await loadSettingsFiles({ env: { EVIE_HOME: home } })
  const run = await startEvie([scriptedProvider([]).plugin, ...plugins], { home, projectSettings: settings.projectSettings, trust: settings.trust, syncSetting: settings.syncSetting })
  cleanups.push(run.stop)
  const globalFile = async () => JSON.parse(await readFile(join(home, "evie.json"), "utf8"))
  return { run, home, globalFile }
}

// ---- Making a project -----------------------------------------------------------------------

test("new_project makes the folder when it is missing, with a .evie/evie.json that syncs it privately, and registers it named after the folder", async () => {
  const run = await start()
  const folder = join(await tempFolder("evie-place-"), "finances")

  const made = await run.evie.send({ type: "new_project", folder })

  expect(made).toEqual({ project: { id: expect.any(String), name: "finances", folder }, madeFolder: true, trusted: false, syncs: false })
  expect(JSON.parse(await readFile(join(folder, ".evie", "evie.json"), "utf8"))).toEqual({ sync: "private" })
  expect(await run.evie.send({ type: "list_projects" })).toContainEqual({ ...made.project, git: false })
})

test("a client is told the home folder of the computer Evie runs on, where it suggests a new project's folder", async () => {
  const run = await start()

  expect(await run.evie.send({ type: "read_home" })).toEqual({ folder: homedir() })
})

test("new_project keeps the project's .evie folder on this computer when asked to, and says it is not backed up", async () => {
  const { run } = await startWithSettings([{ name: "sync", setup() {} }])
  const folder = join(await tempFolder("evie-place-"), "diary")

  const made = await run.evie.send({ type: "new_project", folder, sync: "none" })

  expect(made).toMatchObject({ trusted: true, syncs: false })
  expect(JSON.parse(await readFile(join(folder, ".evie", "evie.json"), "utf8"))).toEqual({ sync: "none" })
  expect((await run.evie.send({ type: "list_projects" })).find((each) => each.id === made.project.id)).not.toHaveProperty("syncs")
  await expect(run.evie.send({ type: "new_project", folder: join(folder, "..", "other"), sync: "public" as never })).rejects.toThrow(
    '`sync` is where the project\'s .evie folder is backed up: "private", to a private repository, or "none", to keep it on this computer.',
  )
})

test("new_project takes a folder that is already there, and a name of the owner's choosing", async () => {
  const run = await start()
  const folder = await tempFolder("evie-place-")
  await writeFile(join(folder, "budget.csv"), "rent,1000\n")

  const made = await run.evie.send({ type: "new_project", folder, name: "  Family finances " })

  expect(made).toEqual({ project: { id: expect.any(String), name: "Family finances", folder }, madeFolder: false, trusted: false, syncs: false })
  expect(await readFile(join(folder, "budget.csv"), "utf8")).toBe("rent,1000\n")
})

test("a project new_project makes is trusted, so its own settings are used without asking", async () => {
  const { run, globalFile } = await startWithSettings()
  const folder = join(await tempFolder("evie-place-"), "notes")

  expect(await run.evie.send({ type: "new_project", folder })).toMatchObject({ trusted: true, syncs: false })
  const opened = await run.evie.send({ type: "open_session", folder, questions: true })

  expect(await globalFile()).toEqual({ plugins: [], trust: [folder] })
  expect(opened.notices).toEqual([])
  expect(stored(run.events).some((event) => event.type === "question")).toBe(false)
})

test("new_project refuses a folder that is already a project, and says how to sync it instead", async () => {
  const run = await start()

  await expect(run.evie.send({ type: "new_project", folder: run.project!.folder })).rejects.toThrow(
    `${run.project!.folder} is already the project "${run.project!.name}". To back up its .evie folder, set "sync": "private" in ` +
      `${join(run.project!.folder, ".evie", "evie.json")}.`,
  )
  expect(await exists(join(run.project!.folder, ".evie"))).toBe(false)
})

test("new_project refuses Evie's data folder, anything inside it, and a folder whose .evie is or holds the data folder", async () => {
  const above = await tempFolder("evie-above-")
  const home = join(above, ".evie")
  const run = await startEvie([scriptedProvider([]).plugin], { home })
  cleanups.push(run.stop)
  // Spelled the way the system's temporary folder is, which on some
  // systems goes through a link, to show the folders are compared as they really are.
  const unresolved = join(tmpdir(), basename(above), ".evie")

  for (const folder of [home, join(unresolved, "projects", "new")]) {
    await expect(run.evie.send({ type: "new_project", folder })).rejects.toThrow(
      `${folder} is in Evie's data folder, ${home}, where Evie keeps its own state. Choose a folder outside it.`,
    )
  }
  await expect(run.evie.send({ type: "new_project", folder: above })).rejects.toThrow(
    `The .evie folder of ${above} is, or holds, Evie's data folder, ${home}. Choose another folder for the project.`,
  )
  expect(await exists(join(home, "projects"))).toBe(false)
  expect(await run.evie.send({ type: "list_projects" })).toEqual([{ ...run.project!, git: false }])

  // A data folder kept inside a project's .evie folder.
  const deeper = await tempFolder("evie-above-")
  const inner = await startEvie([scriptedProvider([]).plugin], { home: join(deeper, ".evie", "state") })
  cleanups.push(inner.stop)
  await expect(inner.evie.send({ type: "new_project", folder: deeper })).rejects.toThrow(`The .evie folder of ${deeper} is, or holds, Evie's data folder`)
})

test("new_project refuses a folder with Evie settings of its own, which it would otherwise trust unread", async () => {
  const { run, globalFile } = await startWithSettings()
  const folder = await tempFolder("evie-cloned-")
  await mkdir(join(folder, ".evie"))
  await writeFile(join(folder, ".evie", "evie.json"), JSON.stringify({ plugins: ["./theirs.ts"] }))

  await expect(run.evie.send({ type: "new_project", folder })).rejects.toThrow(
    `${folder} already has Evie settings of its own, ${join(folder, ".evie", "evie.json")}. Open a session there with \`evie\` instead, ` +
      "which asks whether to trust them.",
  )
  expect(JSON.parse(await readFile(join(folder, ".evie", "evie.json"), "utf8"))).toEqual({ plugins: ["./theirs.ts"] })
  expect(await globalFile()).toEqual({ plugins: [] })
  expect(await run.evie.send({ type: "list_projects" })).toEqual([{ ...run.project!, git: false }])
})

test("new_project refuses a path that is not a full one, a file, a blank name, and a folder inside a git repository", async () => {
  const run = await start()
  const place = await tempFolder("evie-place-")
  await writeFile(join(place, "notes.txt"), "a file")
  await mkdir(join(place, "repository", ".git"), { recursive: true })

  await expect(run.evie.send({ type: "new_project", folder: "finances" })).rejects.toThrow(
    '"finances" is not a full path. Give the folder\'s full path, such as /Users/me/finances.',
  )
  await expect(run.evie.send({ type: "new_project", folder: join(place, "notes.txt") })).rejects.toThrow(`${join(place, "notes.txt")} is a file, not a folder.`)
  await expect(run.evie.send({ type: "new_project", folder: join(place, "new"), name: "  " })).rejects.toThrow("`name` is the project's name, and must be text that is not blank.")
  await expect(run.evie.send({ type: "new_project", folder: join(place, "repository", "docs") })).rejects.toThrow(
    `${join(place, "repository", "docs")} is inside the git repository at ${join(place, "repository")}, so it belongs to that ` +
      "repository's project. Make the project at the repository's own folder instead.",
  )
  expect(await exists(join(place, "new"))).toBe(false)
  expect(await exists(join(place, "repository", "docs"))).toBe(false)
  expect(await run.evie.send({ type: "list_projects" })).toEqual([{ ...run.project!, git: false }])
})

// ---- Keeping .evie/ out of the project's own git --------------------------------------------

test("opening a session in a git repository adds /.evie/ to its local exclude list once, and changes nothing it commits", async () => {
  const run = await start()
  const repository = await tempFolder("evie-repo-")
  await git(["init", "--quiet"], repository)
  const exclude = join(repository, ".git", "info", "exclude")
  await writeFile(exclude, "*.log")
  await mkdir(join(repository, ".evie"))
  await writeFile(join(repository, ".evie", "dashboard.md"), "# Usage\n")

  await run.evie.send({ type: "open_session", folder: repository })
  await run.evie.send({ type: "open_session", folder: repository })

  expect(await readFile(exclude, "utf8")).toBe("*.log\n/.evie/\n")
  expect(await git(["status", "--porcelain", "--untracked-files=all"], repository)).toBe("")
})

test("a worktree's .evie/ is kept out through the exclude list it shares with its main repository", async () => {
  const run = await start()
  const main = await tempFolder("evie-repo-")
  await git(["init", "--quiet"], main)
  await git(["commit", "--quiet", "--allow-empty", "-m", "start"], main)
  const worktree = join(await tempFolder("evie-worktrees-"), "feature")
  await git(["worktree", "add", "--quiet", "-b", "feature", worktree], main)
  await mkdir(join(worktree, ".evie"))
  await writeFile(join(worktree, ".evie", "notes.md"), "notes\n")

  await run.evie.send({ type: "open_session", folder: worktree })

  expect(await git(["status", "--porcelain", "--untracked-files=all"], worktree)).toBe("")
  expect(await readFile(join(main, ".git", "info", "exclude"), "utf8")).toEndWith("/.evie/\n")
})

test("new_project at a code repository's folder keeps the .evie/ it makes out of the repository at once", async () => {
  const run = await start()
  const repository = await tempFolder("evie-repo-")
  await git(["init", "--quiet"], repository)

  await run.evie.send({ type: "new_project", folder: repository })

  expect(await exists(join(repository, ".evie", "evie.json"))).toBe(true)
  expect(await git(["status", "--porcelain", "--untracked-files=all"], repository)).toBe("")
})

test("a project made but not trusted, as when the global settings file cannot be written, says so and how it will be trusted", async () => {
  const { run, home } = await startWithSettings()
  // Stands for a settings file that cannot be replaced.
  await mkdir(join(home, "evie.json.new"))
  const folder = join(await tempFolder("evie-place-"), "notes")

  const making = run.evie.send({ type: "new_project", folder })

  await expect(making).rejects.toThrow(`The project "notes" was made at ${folder}, but could not be trusted: `)
  await expect(making).rejects.toThrow(/Evie asks whether to trust it when a session first opens there\.$/)
  expect((await run.evie.send({ type: "list_projects" })).map((project) => project.name)).toContain("notes")
})

test("a folder whose .evie/ already holds something, such as a skill, is made a project but not trusted, so its first session asks", async () => {
  const { run, globalFile } = await startWithSettings()
  const folder = await tempFolder("evie-cloned-")
  await mkdir(join(folder, ".evie", "skills", "deploy"), { recursive: true })
  await writeFile(join(folder, ".evie", "skills", "deploy", "SKILL.md"), "---\nname: deploy\ndescription: Deploys.\n---\nRun theirs.\n")

  const made = await run.evie.send({ type: "new_project", folder })

  expect(made).toMatchObject({ project: { folder }, madeFolder: false, trusted: false, syncs: false })
  expect(JSON.parse(await readFile(join(folder, ".evie", "evie.json"), "utf8"))).toEqual({ sync: "private" })
  expect(await globalFile()).toEqual({ plugins: [] })
  const asked = nextEvent(run, "question")
  const opening = run.evie.send({ type: "open_session", folder, questions: true })
  expect((await asked).body.choices).toEqual(["Yes, trust this project", "No, ignore its settings file"])
  await run.evie.send({ type: "answer_question", session: (await asked).session, id: (await asked).body.id, choice: null })
  await opening
})

test("new_project refuses a folder whose .evie is a file, before making anything", async () => {
  const run = await start()
  const folder = await tempFolder("evie-place-")
  await writeFile(join(folder, ".evie"), "not a folder")

  await expect(run.evie.send({ type: "new_project", folder })).rejects.toThrow(
    `${join(folder, ".evie")} is a file. Evie keeps what it adds to a project in a folder of that name, so move the file elsewhere, then make the project.`,
  )
  expect(await readFile(join(folder, ".evie"), "utf8")).toBe("not a folder")
  expect(await run.evie.send({ type: "list_projects" })).toEqual([{ ...run.project!, git: false }])
})

test("a folder inside the data folder whose name starts with two dots is still inside it", async () => {
  const run = await start()
  const folder = join(run.home, "..scratch", "notes")

  await expect(run.evie.send({ type: "new_project", folder })).rejects.toThrow(`${folder} is in Evie's data folder`)
  expect(await exists(join(run.home, "..scratch"))).toBe(false)
})

test("a repository that commits its .evie/ on purpose is left alone", async () => {
  const run = await start()
  const repository = await tempFolder("evie-repo-")
  await git(["init", "--quiet"], repository)
  await mkdir(join(repository, ".evie"))
  await writeFile(join(repository, ".evie", "evie.json"), "{}\n")
  await git(["add", ".evie/evie.json"], repository)
  await git(["commit", "--quiet", "-m", "Share Evie's settings"], repository)

  await run.evie.send({ type: "open_session", folder: repository })

  expect(await readFile(join(repository, ".git", "info", "exclude"), "utf8")).not.toContain("/.evie/")
})

test("new_project says the project's .evie folder is backed up only when the sync plugin is loaded and the project trusted", async () => {
  // Stands for the sync plugin, which it is known by.
  const { run } = await startWithSettings([{ name: "sync", setup() {} }])
  const place = await tempFolder("evie-place-")
  await mkdir(join(place, "cloned", ".evie"), { recursive: true })
  await writeFile(join(place, "cloned", ".evie", "notes.md"), "theirs\n")

  expect(await run.evie.send({ type: "new_project", folder: join(place, "fresh") })).toMatchObject({ trusted: true, syncs: true })
  expect(await run.evie.send({ type: "new_project", folder: join(place, "cloned") })).toMatchObject({ trusted: false, syncs: false })
})

// ---- A project's icon and colour ------------------------------------------------------------

test("the owner picks a project's icon and colour, which every list of projects gives from then on, after Evie starts again too", async () => {
  const home = await tempFolder("evie-home-")
  const run = await startEvie([scriptedProvider([]).plugin], { home })
  const project = run.project!

  expect(await run.evie.send({ type: "set_project_icon", project: project.id, icon: "leaf", colour: "moss" })).toBeUndefined()
  expect(await run.evie.send({ type: "list_projects" })).toEqual([{ ...project, git: false, icon: "leaf", colour: "moss" }])
  await run.evie.send({ type: "set_project_icon", project: project.id, icon: "cooking-pot", colour: "rust" })
  await run.evie.close()

  const again = await startEvie([scriptedProvider([]).plugin], { home, projectFolder: run.projectFolder })
  cleanups.push(async () => {
    await again.stop()
    await rm(run.projectFolder, { recursive: true, force: true })
  })
  expect(await again.evie.send({ type: "list_projects" })).toEqual([{ ...project, git: false, icon: "cooking-pot", colour: "rust" }])
})

test("a project's icon is given by its Phosphor name and its colour is one of the six, and anything else is refused, as is a project that is not there", async () => {
  const run = await start()
  const project = run.project!.id

  await expect(run.evie.send({ type: "set_project_icon", project, icon: "Leaf Icon", colour: "moss" })).rejects.toThrow(
    '"Leaf Icon" is not the name of an icon. Give a Phosphor icon\'s name, in lower case with dashes, such as "folder" or "cooking-pot".',
  )
  await expect(run.evie.send({ type: "set_project_icon", project, icon: "leaf", colour: "green" as never })).rejects.toThrow(
    '"green" is not one of the project colours. Give one of: moss, mustard, maroon, rust, slate, plum.',
  )
  await expect(run.evie.send({ type: "set_project_icon", project: "no-such-project", icon: "leaf", colour: "moss" })).rejects.toThrow(
    'There is no project with the id "no-such-project".',
  )
  expect(await run.evie.send({ type: "list_projects" })).toEqual([{ ...run.project!, git: false }])
})

// ---- Whether a project is backed up ---------------------------------------------------------

test("a project is listed as backed up while its .evie folder syncs privately: it is trusted, its setting says so, and the sync plugin is loaded", async () => {
  // Stands for the sync plugin, which it is known by.
  const { run } = await startWithSettings([{ name: "sync", setup() {} }])
  const place = await tempFolder("evie-place-")
  await mkdir(join(place, "cloned", ".evie"), { recursive: true })
  await writeFile(join(place, "cloned", ".evie", "notes.md"), "theirs\n")
  const fresh = (await run.evie.send({ type: "new_project", folder: join(place, "fresh") })).project
  const cloned = (await run.evie.send({ type: "new_project", folder: join(place, "cloned") })).project
  const kept = (await run.evie.send({ type: "new_project", folder: join(place, "kept") })).project
  await writeFile(join(kept.folder, ".evie", "evie.json"), JSON.stringify({ sync: "none" }))

  const listed = await run.evie.send({ type: "list_projects" })

  expect(listed.find((each) => each.id === fresh.id)).toEqual({ ...fresh, git: false, syncs: true })
  expect(listed.find((each) => each.id === cloned.id)).toEqual({ ...cloned, git: false })
  expect(listed.find((each) => each.id === kept.id)).toEqual({ ...kept, git: false })
  expect(listed.find((each) => each.id === run.project!.id)).not.toHaveProperty("syncs")
})

test("without the sync plugin, no project is listed as backed up, whatever its setting", async () => {
  const { run } = await startWithSettings()
  const made = (await run.evie.send({ type: "new_project", folder: join(await tempFolder("evie-place-"), "fresh") })).project

  expect((await run.evie.send({ type: "list_projects" })).find((each) => each.id === made.id)).toEqual({ ...made, git: false })
})
