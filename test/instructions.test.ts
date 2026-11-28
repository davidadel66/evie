import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, readFile, realpath, rm, stat, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { instructions, models, type Plugin } from "../src/index.ts"
import { say, startEvie } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// Starts Evie with the instructions plugin. `global` is what the owner's own
// instruction file holds, and `project` the files in the project folder.
async function start(files: { global?: string; project?: Record<string, string> } = {}, plugins?: (home: string) => Plugin[]) {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  if (files.global !== undefined) await writeFile(join(home, "AGENTS.md"), files.global)
  const model = scriptedProvider(Array.from({ length: 6 }, () => ({ text: ["ok"] })))
  const chosen = plugins ? plugins(home) : [models(), instructions({ dataFolder: home })]
  const run = await startEvie([model.plugin, ...chosen], { home })
  cleanups.push(run.stop)
  for (const [name, text] of Object.entries(files.project ?? {})) await writeFile(join(run.projectFolder, name), text)
  // Sends a message and returns the system prompt the model was given.
  async function system(session = run.session) {
    await say(run, "hi", session)
    return model.requests.at(-1)!.system ?? ""
  }
  return { run, model, home, system }
}

test("a project's AGENTS.md is added to what the model is told, after the system prompt", async () => {
  const { system, run } = await start({ project: { "AGENTS.md": "Use tabs, never spaces.\n" } })

  const prompt = await system()

  expect(prompt).toStartWith("Facts about this session.")
  const file = join(await realpath(run.projectFolder), "AGENTS.md")
  expect(prompt).toEndWith(
    `# Instructions for this project\n\nThese were written for this project. Follow them. The contents of ${file} are given here in full, ` +
      `so you already have them: do not read that file to find them.\n\nUse tabs, never spaces.`,
  )
})

test("CLAUDE.md is used when a project has no AGENTS.md, and AGENTS.md wins when it has both", async () => {
  const onlyClaude = await start({ project: { "CLAUDE.md": "From the Claude file." } })
  expect(await onlyClaude.system()).toContain("From the Claude file.")

  const both = await start({ project: { "CLAUDE.md": "From the Claude file.", "AGENTS.md": "From the agents file." } })
  const prompt = await both.system()
  expect(prompt).toContain("From the agents file.")
  expect(prompt).not.toContain("From the Claude file.")
})

test("the owner's own instruction file is added in every session, before the project's", async () => {
  const { run, system, home } = await start({ global: "Always answer briefly.", project: { "AGENTS.md": "Use tabs." } })
  const general = await run.evie.send({ type: "open_session" })

  const inProject = await system()
  const inGeneral = await system(general.session)

  expect(inProject).toContain(
    `# Instructions from the owner\n\nThe owner wrote these for every project. Follow them. The contents of ${join(home, "AGENTS.md")} ` +
      `are given here in full, so you already have them: do not read that file to find them.\n\nAlways answer briefly.`,
  )
  expect(inProject.indexOf("Always answer briefly.")).toBeLessThan(inProject.indexOf("Use tabs."))
  expect(inGeneral).toContain("Always answer briefly.")
  expect(inGeneral).not.toContain("Use tabs.")
})

test("with no instruction files the system prompt is as it was", async () => {
  const withPlugin = await start()
  const without = await start({}, () => [models()])

  // The two sessions are in different folders, and may have started a minute apart.
  const strip = (text: string) => text.replace(/evie-project-\w+/g, "project").replace(/The session started on .*/, "")
  expect(strip(await withPlugin.system())).toBe(strip(await without.system()))
})

test("an empty instruction file adds nothing", async () => {
  const { system } = await start({ project: { "AGENTS.md": "  \n\n" } })

  expect(await system()).not.toContain("# Instructions")
})

test("the instructions are the same from one request to the next, so the start of every request matches", async () => {
  const { system } = await start({ global: "Be brief.", project: { "AGENTS.md": "Use tabs." } })

  expect(await system()).toBe(await system())
})

test("an instruction file edited during a session is used from the next request", async () => {
  const { run, system } = await start({ project: { "AGENTS.md": "Use tabs." } })

  expect(await system()).toContain("Use tabs.")
  await writeFile(join(run.projectFolder, "AGENTS.md"), "Use two spaces.")

  const after = await system()
  expect(after).toContain("Use two spaces.")
  expect(after).not.toContain("Use tabs.")
})

test("a very long instruction file is cut, and the model is told where the rest is", async () => {
  const long = Array.from({ length: 4000 }, (_, index) => `Rule ${index + 1}: keep it short.`).join("\n")
  const { run, system } = await start({ project: { "AGENTS.md": long } })

  const prompt = await system()

  expect(prompt).toContain("Rule 1: keep it short.")
  expect(prompt).not.toContain("Rule 4000:")
  expect(prompt).toContain("are given here up to a size limit")
  expect(prompt).toEndWith(
    `[This file is longer than 32 KiB, so the rest was left out. Read ${join(await realpath(run.projectFolder), "AGENTS.md")} if you need it.]`,
  )
})

test("the instructions are still given when nothing else sets a system prompt", async () => {
  const { system } = await start({ project: { "AGENTS.md": "Use tabs." } }, (home) => [instructions({ dataFolder: home })])

  expect(await system()).toStartWith("# Instructions for this project")
})

test("the instructions are kept whichever order the plugins are listed in", async () => {
  const { system } = await start({ project: { "AGENTS.md": "Use tabs." } }, (home) => [instructions({ dataFolder: home }), models()])

  const prompt = await system()
  expect(prompt).toStartWith("Facts about this session.")
  expect(prompt).toContain("Use tabs.")
})

// ---- The owner's notes for a project ---------------------------------------------------------------

test("the owner's notes for a project are given after the repository's file, and say they win", async () => {
  const { run, system } = await start({ project: { "AGENTS.md": "Use tabs." } })
  await mkdir(join(run.projectFolder, ".evie"))
  await writeFile(join(run.projectFolder, ".evie", "instructions.md"), "Ask before pushing.\n")

  const prompt = await system()

  expect(prompt).toContain("# Instructions for this project")
  expect(prompt).toContain("# The owner's notes for this project")
  expect(prompt.indexOf("Use tabs.")).toBeLessThan(prompt.indexOf("Ask before pushing."))
  expect(prompt).toContain("the notes win")
})

test("notes alone are given when the repository has no instruction file", async () => {
  const { run, system } = await start()
  await mkdir(join(run.projectFolder, ".evie"))
  await writeFile(join(run.projectFolder, ".evie", "instructions.md"), "Answer in French.")

  const prompt = await system()

  expect(prompt).toContain("Answer in French.")
  expect(prompt).not.toContain("# Instructions for this project")
})

test("read_instructions names the repository's file in use, the one ignored, and the notes", async () => {
  const { run } = await start({ project: { "AGENTS.md": "From agents.", "CLAUDE.md": "From claude." } })
  const project = run.project!.id

  expect(await run.evie.send({ type: "read_instructions", project })).toEqual({
    repo: { name: "AGENTS.md", text: "From agents.", cut: false },
    ignored: ["CLAUDE.md"],
    notes: "",
    canWrite: true,
  })
})

test("read_instructions for a project with no instruction files has no repo file", async () => {
  const { run } = await start()

  expect(await run.evie.send({ type: "read_instructions", project: run.project!.id })).toEqual({ ignored: [], notes: "", canWrite: true })
})

test("write_instructions keeps the notes in .evie/instructions.md and never touches the repository's file", async () => {
  const { run, system } = await start({ project: { "CLAUDE.md": "Team rules." } })
  const project = run.project!.id

  await run.evie.send({ type: "write_instructions", project, text: "Ask before pushing." })

  expect(await readFile(join(run.projectFolder, ".evie", "instructions.md"), "utf8")).toBe("Ask before pushing.\n")
  expect(await readFile(join(run.projectFolder, "CLAUDE.md"), "utf8")).toBe("Team rules.")
  expect((await run.evie.send({ type: "read_instructions", project })).notes).toBe("Ask before pushing.\n")
  // The next message already has them.
  expect(await system()).toContain("Ask before pushing.")
})

test("saving notes of only spaces removes the file", async () => {
  const { run } = await start()
  const project = run.project!.id
  await run.evie.send({ type: "write_instructions", project, text: "Something." })

  await run.evie.send({ type: "write_instructions", project, text: "  \n" })

  await expect(stat(join(run.projectFolder, ".evie", "instructions.md"))).rejects.toThrow()
})

test("notes longer than 32 KiB are refused, saying what to do", async () => {
  const { run } = await start()

  await expect(run.evie.send({ type: "write_instructions", project: run.project!.id, text: "x".repeat(33 * 1024) })).rejects.toThrow("Shorten them and save again.")
})

test("notes are not written through a link, and not into a .evie folder that is a link", async () => {
  const { run, home } = await start()
  const project = run.project!.id
  const elsewhere = join(home, "elsewhere.md")
  await mkdir(join(run.projectFolder, ".evie"))
  await symlink(elsewhere, join(run.projectFolder, ".evie", "instructions.md"))
  await expect(run.evie.send({ type: "write_instructions", project, text: "Hello." })).rejects.toThrow("is a link")
  expect(await Bun.file(elsewhere).exists()).toBe(false)

  await rm(join(run.projectFolder, ".evie"), { recursive: true })
  await symlink(home, join(run.projectFolder, ".evie"))
  await expect(run.evie.send({ type: "write_instructions", project, text: "Hello." })).rejects.toThrow("not one evie keeps files in")
  expect((await run.evie.send({ type: "read_instructions", project })).canWrite).toBe(false)
})
