import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, realpath, rm, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { permissions, runTerminal, skills, type Plugin } from "../src/index.ts"
import { say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

const RELEASE = `---
name: release
description: How to cut a release of this project.
---
1. Update the changelog.
2. Run scripts/tag.sh.
`

async function writeSkill(root: string, name: string, text: string, files: Record<string, string> = {}) {
  await mkdir(join(root, name), { recursive: true })
  await writeFile(join(root, name, "SKILL.md"), text)
  for (const [file, contents] of Object.entries(files)) {
    await mkdir(join(root, name, file, ".."), { recursive: true })
    await writeFile(join(root, name, file), contents)
  }
}

// Evie with the skills plugin and a data folder of its own, so that no test
// reads the skills of whoever runs it.
async function start(script: ScriptedReply[], options: { trusted?: (folder: string) => boolean; plugins?: (home: string) => Plugin[]; questions?: boolean } = {}) {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  const model = scriptedProvider(script)
  const run = await startEvie([model.plugin, skills({ dataFolder: home, ...(options.trusted && { trusted: options.trusted }) }), ...(options.plugins?.(home) ?? [])], {
    home,
    ...(options.questions && { questions: true }),
  })
  cleanups.push(run.stop)
  const slash = async (name: string, args = "") => (await run.evie.send({ type: "run_command", session: run.session, name, args })).text
  const lastResult = () => stored(run.events).findLast((event) => event.type === "tool_result")!.body.text as string
  return { run, model, home, script, slash, lastResult, mine: join(home, "skills"), projects: join(run.projectFolder, ".evie", "skills") }
}
const turnEnds = (run: { events: Parameters<typeof stored>[0] }) => stored(run.events).filter((event) => event.type === "turn_end").length
async function turns(run: Parameters<typeof turnEnds>[0] & { evie: { subscribe(listener: () => void): () => void } }, count: number) {
  while (turnEnds(run) < count) await new Promise<void>((resolve) => { const stop = run.evie.subscribe(() => { stop(); resolve() }) })
}

// ---- What the model is given ----------------------------------------------------------------

test("the model is shown each skill's name and description, and loads the instructions only when it needs them", async () => {
  const { run, model, mine, lastResult } = await start([{ text: [], calls: [{ name: "skill", input: { name: "release" } }] }, { text: ["Releasing."] }])
  await writeSkill(mine, "release", RELEASE, { "scripts/tag.sh": "git tag v1\n", "notes.md": "Remember the changelog.\n" })

  await say(run, "cut a release")

  const system = model.requests[0]!.system!
  expect(system).toContain("Skills the owner has set up.")
  expect(system).toContain("- release: How to cut a release of this project.")
  // The instructions are not in the prompt. They arrive when the skill is loaded.
  expect(system).not.toContain("Update the changelog")
  expect(lastResult()).toBe(
    `The skill "release", from ${join(mine, "release")}. It is the owner's own instruction for this kind of task, so follow it:\n\n1. Update the changelog.\n2. Run scripts/tag.sh.\n\n` +
      `Other files in this skill, to read or run when its instructions say so:\n- ${join(mine, "release", "notes.md")}\n- ${join(mine, "release", "scripts", "tag.sh")}`,
  )
})

test("asking for a skill that does not exist says which ones do, and with no skills nothing is added to the prompt", async () => {
  const none = await start([{ text: ["ok"] }])
  await say(none.run, "hi")
  expect(none.model.requests[0]?.system ?? "").not.toContain("Skills the owner has set up")

  const { run, mine, lastResult } = await start([{ text: [], calls: [{ name: "skill", input: { name: "deploy" } }] }, { text: ["No such skill."] }])
  await writeSkill(mine, "release", RELEASE)
  await say(run, "deploy it")
  expect(lastResult()).toBe('There is no skill named "deploy". The skills are: release.')
})

test("a project's own skills are used only when the owner has trusted the project, and then win over the owner's of the same name", async () => {
  const projectSkill = "---\ndescription: How this project releases.\n---\nAsk the release manager.\n"
  const untrusted = await start([{ text: ["ok"] }])
  await writeSkill(untrusted.mine, "release", RELEASE)
  await writeSkill(untrusted.projects, "release", projectSkill)
  await writeSkill(untrusted.projects, "deploy", "Deploy it.\n")
  await say(untrusted.run, "hi")
  expect(untrusted.model.requests[0]?.system).toContain("- release: How to cut a release of this project.")
  expect(untrusted.model.requests[0]?.system).not.toContain("deploy")

  const trusted = await start([{ text: ["ok"] }], { trusted: () => true })
  await writeSkill(trusted.mine, "release", RELEASE)
  await writeSkill(trusted.projects, "release", projectSkill)
  await writeSkill(trusted.projects, "deploy", "Deploy it.\n")
  await say(trusted.run, "hi")
  expect(trusted.model.requests[0]?.system).toContain("- deploy: (no description)\n- release: How this project releases.")
})

// ---- The owner starting one --------------------------------------------------------------------

test("a slash and a skill's name puts the skill in the conversation and sends what was typed after it", async () => {
  const { run, model, mine, slash } = await start([{ text: ["Changelog fixed."] }])
  await writeSkill(mine, "release", RELEASE)

  expect(await slash("release", "fix the changelog first")).toBe("Using the skill release.")
  await turns(run, 1)

  expect(stored(run.events).map((event) => event.type)).toEqual(["note", "turn_start", "user_message", "assistant_message", "turn_end"])
  const messages = model.requests[0]!.messages
  expect(messages[0]?.text).toStartWith('[Note from Evie: The owner asked for a skill, for what they say next. The skill "release", from ')
  expect(messages[0]?.text).toContain("1. Update the changelog.")
  expect(messages[1]).toEqual({ role: "user", text: "fix the changelog first" })
})

test("a skill started with nothing typed after its name is still started", async () => {
  const { run, model, mine, slash } = await start([{ text: ["Starting the release."] }])
  await writeSkill(mine, "release", RELEASE)

  await slash("release")
  await turns(run, 1)

  expect(model.requests[0]?.messages.at(-1)).toEqual({ role: "user", text: 'Use the skill "release".' })
})

test("a skill written during a session can be started at once, with no restart", async () => {
  const { run, mine, slash } = await start([{ text: ["Reviewing."] }])
  await expect(slash("review", "the last commit")).rejects.toThrow('There is no command "/review". The commands are: /skills, /compact.')

  await writeSkill(mine, "review", "---\ndescription: How to review a change.\n---\nRead the diff first.\n")
  expect(await slash("review", "the last commit")).toBe("Using the skill review.")
  await turns(run, 1)
})

test("a command of the same name wins over a skill, and /skills says how to reach each one", async () => {
  // A command another plugin registered, with the name a skill also has.
  const other: Plugin = { name: "other", setup: (host) => host.commands.add({ name: "status", description: "Shows the status.", run: () => "All quiet." }) }
  const { mine, slash } = await start([], { plugins: () => [other] })
  expect(await slash("skills")).toStartWith("You have no skills. A skill is a folder holding a SKILL.md")

  await writeSkill(mine, "release", RELEASE)
  await writeSkill(mine, "status", "---\ndescription: How to write a status report.\n---\nKeep it short.\n")

  expect(await slash("status")).toBe("All quiet.")
  expect(await slash("skills")).toBe("/release  How to cut a release of this project.\n(/status is a command, so ask for this one in words)  How to write a status report.")
})

test("in the terminal, a slash and a skill's name starts the skill and the reply is shown", async () => {
  const { run, mine } = await start([{ text: ["Changelog fixed."] }])
  await writeSkill(mine, "release", RELEASE)
  let printed = ""

  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield "/release fix the changelog"
      while (!printed.includes("Changelog fixed.")) await Bun.sleep(1)
      // The terminal is ready for the next line, which it would not be if it had lost count.
      yield "/skills"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("Using the skill release.\n")
  expect(printed).toContain("Changelog fixed.")
  expect(printed).toContain("/release  How to cut a release of this project.")
})

// ---- Scripts in a skill ---------------------------------------------------------------------------

test("a command may run a script from the owner's skills, with their say-so, and reach nothing else in the data folder", async () => {
  const ran: string[] = []
  // A stand-in for the shell tool, which only records the command.
  const shellStandIn: Plugin = {
    name: "shell-stand-in",
    setup: (host) => host.tools.add({ name: "shell", description: "A stand-in.", inputSchema: { type: "object" }, run: async (input) => (ran.push(String(input.command)), "ran") }),
  }
  const { run, script, mine, home, lastResult } = await start([], { plugins: (data) => [shellStandIn, permissions({ mode: "auto", dataFolder: data })], questions: true })
  await writeSkill(mine, "release", RELEASE, { "scripts/tag.sh": "git tag v1\n" })
  const asked: string[] = []
  run.evie.subscribe((event) => {
    if (event.type !== "question") return
    asked.push(event.body.text)
    void run.evie.send({ type: "answer_question", session: event.session, id: event.body.id, choice: "Yes, once" })
  })
  const shell = async (command: string) => {
    script.push({ text: [], calls: [{ name: "shell", input: { command } }] }, { text: ["ok"] })
    await say(run, "go")
    return lastResult()
  }
  const tag = join(mine, "release", "scripts", "tag.sh")

  // The script is outside the project, so it is asked about even in auto mode.
  expect(await shell(`sh ${tag}`)).toBe("ran")
  expect(asked).toHaveLength(1)
  const escapes = [
    `cat ${join(home, "evie.db")}`,
    `cat ${mine}/../evie.db`,
    `sh ${tag}; cat ${join(home, ".env")}`,
    // From inside the skills folder, the rest of the data folder is two steps up.
    `cd ${join(mine, "release")} && cat ../../.env`,
    // A link made there could point anywhere, and so could a path that climbs through one.
    `ln -s .. ${join(mine, "release", "up")}`,
    `cat ${mine}/release/up/../.env`,
    // Writing a skill is for the file tools, which ask in their own way.
    `echo changed > ${join(mine, "release", "SKILL.md")}`,
    `cp notes.md ${join(mine, "release", "SKILL.md")}`,
  ]
  for (const command of escapes) {
    expect(await shell(command)).toStartWith("Blocked by the guard:")
  }
  expect(ran).toEqual([`sh ${tag}`])
})

// ---- Found in review --------------------------------------------------------------------------------

test("a skill that is a link to somewhere else is not used, so it cannot hand the model another file", async () => {
  const outside = await realpath(await mkdtemp(join(tmpdir(), "evie-outside-")))
  cleanups.push(() => rm(outside, { recursive: true, force: true }))
  await writeSkill(outside, "linked", "---\ndescription: Kept somewhere else.\n---\nDo it this way.\n")
  const { run, model, home, mine, slash, script, lastResult } = await start([])
  await writeFile(join(home, ".env"), "OPENROUTER_API_KEY=secret-value\n")
  await writeSkill(mine, "release", RELEASE)
  // A skill whose SKILL.md is a link to the keys, and a skill folder that is a link.
  await mkdir(join(mine, "leak"))
  await symlink(join(home, ".env"), join(mine, "leak", "SKILL.md"))
  await symlink(join(outside, "linked"), join(mine, "linked"))

  expect(await slash("skills")).toBe("/release  How to cut a release of this project.")
  script.push({ text: [], calls: [{ name: "skill", input: { name: "leak" } }] }, { text: ["No such skill."] })
  await say(run, "load it")

  expect(lastResult()).toBe('There is no skill named "leak". The skills are: release.')
  expect(JSON.stringify(model.requests)).not.toContain("secret-value")
})

test("a very long skill is cut, and a skill whose name cannot be typed after a slash says so", async () => {
  const { mine, slash, run, script, lastResult } = await start([])
  await writeSkill(mine, "long", `---\ndescription: A long one.\n---\n${"word ".repeat(10_000)}\n`)
  await writeSkill(mine, "2fa", "---\ndescription: Two-factor setup.\n---\nDo it.\n")

  expect(await slash("skills")).toBe("2fa (ask for this one in words)  Two-factor setup.\n/long  A long one.")
  script.push({ text: [], calls: [{ name: "skill", input: { name: "long" } }] }, { text: ["Loaded."] })
  await say(run, "load it")
  expect(lastResult()).toContain("[The rest of this file was left out: it is longer than 32 KiB.]")
  expect(lastResult().length).toBeLessThan(34_000)
})

test("a client can list the commands it can run in a session, each with what it does, and the owner's skills among them, except those a slash cannot reach", async () => {
  const other: Plugin = { name: "other", setup: (host) => host.commands.add({ name: "status", description: "Shows the status.", run: () => "All quiet." }) }
  const { run, mine } = await start([], { plugins: () => [other] })
  await writeSkill(mine, "release", RELEASE)
  // A command has the name already, and a slash cannot start a name like this one.
  await writeSkill(mine, "status", "---\ndescription: How to write a status report.\n---\nKeep it short.\n")
  await writeSkill(mine, "2fa", "---\ndescription: Two-factor setup.\n---\nDo it.\n")
  await writeSkill(mine, "notes", "---\nname: notes\n---\nKeep notes.\n")

  expect(await run.evie.send({ type: "list_commands", session: run.session })).toEqual([
    { name: "skills", description: "List your skills. To start one, type a slash and its name, with anything you want to add." },
    { name: "status", description: "Shows the status." },
    { name: "compact", description: "Replace the older part of the conversation with a summary, to save room." },
    { name: "notes", description: "", kind: "skill" },
    { name: "release", description: "How to cut a release of this project.", kind: "skill" },
  ])
})

test("a command whose names cannot be listed is still listed, without them, and the other commands' names are kept", async () => {
  const broken: Plugin = {
    name: "broken",
    setup: (host) =>
      host.commands.add({
        name: "notes",
        description: "Opens a note.",
        run: () => "",
        names: async () => {
          throw new Error("the notes folder could not be read")
        },
      }),
  }
  const { run, mine } = await start([], { plugins: () => [broken] })
  await writeSkill(mine, "release", RELEASE)

  expect(await run.evie.send({ type: "list_commands", session: run.session })).toEqual([
    { name: "skills", description: "List your skills. To start one, type a slash and its name, with anything you want to add." },
    { name: "notes", description: "Opens a note." },
    { name: "compact", description: "Replace the older part of the conversation with a summary, to save room." },
    { name: "release", description: "How to cut a release of this project.", kind: "skill" },
  ])
})
