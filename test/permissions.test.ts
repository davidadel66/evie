import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, readFile, realpath, rm, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { dirname, join } from "node:path"
import { loadSettingsFiles, permissions, type PermissionsOptions, type Plugin, type Rule } from "../src/index.ts"
import { say, startEvie, stored, type Run } from "./support/harness.ts"
import { scriptedProvider, type ScriptedCall, type ScriptedReply } from "./support/scripted-provider.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// Stand-ins for the real tools, under the same names. None of them touches
// a file or runs a command: each only records that it ran.
function standIns() {
  const ran: string[] = []
  const names = ["read", "grep", "glob", "write", "edit", "patch", "shell", "web_fetch", "deploy"]
  const plugin: Plugin = {
    name: "stand-ins",
    setup(host) {
      for (const name of names) {
        host.tools.add({
          name,
          description: `A stand-in for ${name}.`,
          inputSchema: { type: "object" },
          run: async (input) => {
            ran.push(`${name} ${input.command ?? input.path ?? input.url ?? ""}`.trim())
            return "done"
          },
        })
      }
    },
  }
  return { plugin, ran }
}

const YES = "Yes, once"
const NO = "No"

// Evie with the permissions plugin and the stand-in tools. Each call given
// to `attempt` is one turn in which the model asks for that tool. A question
// is answered with the next of `answers`; with none left, or with a client
// that shows no questions, it is refused.
async function start(options: PermissionsOptions = {}, client: { questions?: boolean } = { questions: true }) {
  const tools = standIns()
  const script: ScriptedReply[] = []
  const model = scriptedProvider(script)
  const run = await startEvie([model.plugin, tools.plugin, permissions(options)], client)
  cleanups.push(run.stop)

  const asked: { text: string; choices: string[] }[] = []
  const answers: (string | ((choices: string[]) => string))[] = []
  run.evie.subscribe((event) => {
    if (event.type !== "question") return
    asked.push({ text: event.body.text, choices: event.body.choices })
    const next = answers.shift()
    const choice = next === undefined ? null : typeof next === "function" ? next(event.body.choices) : next
    void run.evie.send({ type: "answer_question", session: event.session, id: event.body.id, choice })
  })

  // Has the model ask for one tool, and returns what the tool call came back with.
  async function attempt(call: ScriptedCall, answer?: string | ((choices: string[]) => string), session = run.session) {
    if (answer !== undefined) answers.push(answer)
    script.push({ text: [], calls: [call] }, { text: ["ok"] })
    await say(run, "go", session)
    return stored(run.events).findLast((event) => event.type === "tool_result")!.body.text as string
  }
  const mode = (name: string, session = run.session) => run.evie.send({ type: "run_command", session, name: "mode", args: name })
  return { run, attempt, asked, ran: tools.ran, mode, folder: run.project!.folder }
}

const shell = (command: string, workdir?: string): ScriptedCall => ({ name: "shell", input: { command, ...(workdir && { workdir }) } })
const edit = (path: string): ScriptedCall => ({ name: "edit", input: { path } })
const read = (path: string): ScriptedCall => ({ name: "read", input: { path } })
const always = (choices: string[]) => choices[1]!

// ---- Manual mode, the default ----------------------------------------------------------

test("reading and searching inside the project are allowed without asking", async () => {
  const { attempt, asked, ran } = await start()

  await attempt(read("src/app.ts"))
  await attempt({ name: "grep", input: { pattern: "todo" } })
  await attempt({ name: "glob", input: { pattern: "**/*.ts", path: "src" } })

  expect(asked).toEqual([])
  expect(ran).toEqual(["read src/app.ts", "grep", "glob src"])
})

test("a change to a file is asked about, and a yes lets that one change through", async () => {
  const { attempt, asked, ran } = await start()

  expect(await attempt(edit("src/app.ts"), YES)).toBe("done")
  expect(await attempt(edit("src/app.ts"), YES)).toBe("done")

  expect(asked).toHaveLength(2)
  expect(asked[0]).toEqual({
    text: "Let Evie change src/app.ts?",
    choices: ["Yes, once", "Always in this project: allow changing files inside the project", "No"],
  })
  expect(ran).toEqual(["edit src/app.ts", "edit src/app.ts"])
})

test("a permission prompt says it is one, so that a client can show it as one, as the web app shows it in place", async () => {
  const { run, attempt } = await start()

  await attempt(edit("src/app.ts"), YES)

  const question = stored(run.events).find((event) => event.type === "question")
  expect(question?.body).toEqual({ id: expect.any(String), text: "Let Evie change src/app.ts?", choices: [YES, "Always in this project: allow changing files inside the project", NO], freeText: true, kind: "permission" })
})

test("a no stops the call, and the model is told why", async () => {
  const { attempt, ran } = await start()

  expect(await attempt(edit("src/app.ts"), NO)).toStartWith("The owner said no to this.")
  expect(await attempt(edit("src/app.ts"), "use the other file")).toStartWith('The owner said no to this, and added: "use the other file".')
  expect(ran).toEqual([])
})

test("a plain yes or no typed in words means what it says", async () => {
  const { attempt, ran } = await start()

  expect(await attempt(edit("a.ts"), "yes")).toBe("done")
  expect(await attempt(edit("b.ts"), "n")).toStartWith("The owner said no to this. Do not")
  expect(ran).toEqual(["edit a.ts"])
})

test("when nobody can be asked, a call that needs a yes is denied", async () => {
  const { attempt, ran } = await start({}, {})

  expect(await attempt(edit("src/app.ts"))).toStartWith("Not done: this needs the owner's say-so")
  expect(await attempt(read("src/app.ts"))).toBe("done")
  expect(ran).toEqual(["read src/app.ts"])
})

test("an always answer is remembered for the project, and saved", async () => {
  const saved: [string, Rule[]][] = []
  const { run, attempt, asked, folder } = await start({ remember: (project, rules) => void saved.push([project, rules]) })

  await attempt(edit("src/app.ts"), always)
  await attempt(edit("docs/guide.md"))
  // Another session in the same project is covered too.
  const other = await run.evie.send({ type: "open_session", folder, questions: true })
  await attempt(edit("src/other.ts"), undefined, other.session)

  expect(asked).toHaveLength(1)
  expect(saved).toEqual([[folder, [{ action: "edit", match: `${folder}/**`, effect: "allow" }]]])
})

test("reading or changing a file outside the project is asked about", async () => {
  const { attempt, asked } = await start()

  await attempt(read("/somewhere/else/notes.md"), YES)
  await attempt(edit("../sibling/file.ts"), always)

  expect(asked[0]?.text).toBe("Let Evie read /somewhere/else/notes.md, which is outside the project?")
  expect(asked[1]?.choices[1]).toStartWith("Always in this project: allow changing files directly in ")
})

test("a patch is judged by every file it names", async () => {
  const { attempt, asked, folder } = await start({ rules: [{ action: "edit", match: "src/*", effect: "allow" }] })
  const patch = (...files: string[]): ScriptedCall => ({ name: "patch", input: { patchText: files.map((file) => `*** Update File: ${file}\n@@\n-a\n+b`).join("\n") } })

  await attempt(patch("src/a.ts", "src/b.ts"))
  await attempt(patch("src/a.ts", "/somewhere/else/hosts", "README.md"), YES)

  expect(asked).toHaveLength(1)
  expect(asked[0]?.text).toBe("Let Evie change these files outside the project?\n\n  /somewhere/else/hosts\n  README.md")
  expect(folder).toBeDefined()
})

// ---- Shell commands -----------------------------------------------------------------------

test("a command that only looks runs without asking, and any other command is asked about", async () => {
  const { attempt, asked, ran } = await start()

  await attempt(shell("ls -la"))
  await attempt(shell("git status"))
  await attempt(shell("git log --oneline | head -5"))
  await attempt(shell("bun test"), YES)

  expect(asked.map((question) => question.text)).toEqual(["Run this command?\n\n  bun test"])
  expect(ran).toHaveLength(4)
})

test("each command in a line is judged on its own, so a harmless start does not carry the rest", async () => {
  const { attempt, asked } = await start()

  await attempt(shell("git status && git push origin dev"), always)
  await attempt(shell("cat notes.txt > copy.txt"), NO)
  await attempt(shell("echo $(rm notes.txt)"), NO)
  // The always answer covered the push, and nothing else.
  await attempt(shell("git push origin main"))
  await attempt(shell("git commit -m wip"), NO)

  expect(asked).toHaveLength(4)
  expect(asked[0]?.choices[1]).toBe('Always in this project: allow commands that start with "git push"')
})

test("a command run in a folder outside the project is asked about even when it only looks", async () => {
  const { attempt, asked } = await start()

  await attempt(shell("ls", "/somewhere/else"), YES)

  expect(asked[0]?.text).toBe("Run this command?\n\n  ls\n(in /somewhere/else)")
})

// ---- The other modes ------------------------------------------------------------------------

test("in auto mode Evie works freely inside the project and asks before reaching outside it", async () => {
  const { attempt, asked, ran } = await start({ mode: "auto" })

  await attempt(edit("src/app.ts"))
  await attempt(shell("bun test && git commit -m done"))
  await attempt({ name: "deploy", input: {} })
  await attempt(shell("git push origin dev"), YES)
  await attempt(shell("gh pr create --fill"), YES)
  await attempt(shell("gh pr view 12"))
  await attempt(shell("curl -X POST https://example.com/hook -d ok"), YES)
  await attempt(shell("curl https://example.com/page"))
  await attempt(edit("/somewhere/else/hosts"), NO)

  expect(asked.map((question) => question.text.split("\n").at(-1)?.trim())).toEqual([
    "git push origin dev",
    "gh pr create --fill",
    "curl -X POST https://example.com/hook -d ok",
    "Let Evie change /somewhere/else/hosts, which is outside the project?",
  ])
  expect(ran).toHaveLength(8)
})

test("in read-only mode nothing is changed and nothing is asked", async () => {
  const { attempt, asked, ran } = await start({ mode: "read-only", rules: [{ action: "edit", effect: "allow" }, { action: "shell", match: "bun test*", effect: "allow" }] })

  expect(await attempt(edit("src/app.ts"))).toStartWith("Not allowed: Evie is in read-only mode")
  expect(await attempt(shell("bun test"))).toStartWith("Not allowed: Evie is in read-only mode")
  expect(await attempt({ name: "deploy", input: {} })).toStartWith("Not allowed: Evie is in read-only mode")
  expect(await attempt(read("/somewhere/else/notes.md"))).toStartWith("Not allowed: Evie is in read-only mode")
  await attempt(read("src/app.ts"))
  await attempt(shell("git diff"))

  expect(asked).toEqual([])
  expect(ran).toEqual(["read src/app.ts", "shell git diff"])
})

test("/mode shows the mode and changes it, for that session only", async () => {
  const { run, attempt, asked, mode, folder } = await start()
  const other = await run.evie.send({ type: "open_session", folder, questions: true })

  expect((await mode("")).text).toStartWith("The mode is manual.")
  expect((await mode("auto")).text).toBe("The mode is now auto.")
  expect((await mode("yolo")).text).toStartWith('There is no mode "yolo".')
  await attempt(edit("src/app.ts"))
  await attempt(edit("src/app.ts"), NO, other.session)

  expect(asked).toHaveLength(1)
  expect((await mode("", other.session)).text).toStartWith("The mode is manual.")
})

test("a tool the plugin knows nothing about is asked about in manual mode", async () => {
  const { attempt, asked } = await start()

  await attempt({ name: "deploy", input: { target: "production" } }, always)
  await attempt({ name: "deploy", input: { target: "production" } })

  expect(asked).toEqual([
    { text: 'Let Evie use the tool deploy?\n\n  {"target":"production"}', choices: ["Yes, once", "Always in this project: allow the tool deploy", "No"] },
  ])
})

// ---- Rules ------------------------------------------------------------------------------------

test("a rule decides before the mode does, and the last rule that matches wins", async () => {
  const { attempt, asked, ran } = await start({
    mode: "auto",
    rules: [
      { action: "shell", match: "bun *", effect: "ask" },
      { action: "shell", match: "bun test*", effect: "allow" },
      { action: "edit", match: "docs/*", effect: "ask" },
    ],
  })

  await attempt(shell("bun test --watch"))
  await attempt(shell("bun install"), YES)
  await attempt(edit("docs/guide.md"), YES)
  await attempt(edit("src/app.ts"))

  expect(asked).toHaveLength(2)
  expect(ran).toHaveLength(4)
})

test("a rule that denies always wins: over a later rule, an always answer and auto mode", async () => {
  const { attempt, asked, ran } = await start({
    mode: "auto",
    rules: [
      { action: "edit", match: "*.env", effect: "deny" },
      { action: "edit", effect: "allow" },
      { action: "shell", match: "git push*", effect: "deny" },
    ],
    remembered: {},
  })

  expect(await attempt(edit("config/prod.env"))).toStartWith("Not allowed: one of the owner's rules denies this.")
  expect(await attempt(shell("git add . && git push"))).toStartWith("Not allowed: one of the owner's rules denies this.")
  expect(asked).toEqual([])
  expect(ran).toEqual([])
})

test("what the guard blocks is denied in every mode, without asking", async () => {
  const { attempt, asked, ran } = await start({ mode: "auto", rules: [{ action: "shell", effect: "allow" }] })

  expect(await attempt(shell("sudo rm -rf /"))).toStartWith("Blocked by the guard:")
  expect(await attempt(read("~/.ssh/id_ed25519"))).toStartWith("Blocked by the guard:")
  expect(asked).toEqual([])
  expect(ran).toEqual([])
})

// ---- Settings files ---------------------------------------------------------------------------

async function withSettings(global: Record<string, unknown>, project?: Record<string, unknown>) {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  const folder = await realpath(await mkdtemp(join(tmpdir(), "evie-project-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }), () => rm(folder, { recursive: true, force: true }))
  await writeFile(join(home, "evie.json"), JSON.stringify({ model: "some/model", plugins: ["permissions"], ...global, ...(project && { trust: [folder] }) }))
  if (project) {
    await mkdir(join(folder, ".evie"))
    await writeFile(join(folder, ".evie", "evie.json"), JSON.stringify(project))
  }
  const settings = await loadSettingsFiles({ env: { EVIE_HOME: home } })
  const tools = standIns()
  const script: ScriptedReply[] = []
  const run = await startEvie([scriptedProvider(script).plugin, tools.plugin, ...settings.plugins], { home, projectFolder: folder, projectSettings: settings.projectSettings, questions: true })
  cleanups.push(run.stop)
  const asked: string[][] = []
  let answer: (choices: string[]) => string | null = () => null
  run.evie.subscribe((event) => {
    if (event.type !== "question") return
    asked.push(event.body.choices)
    void run.evie.send({ type: "answer_question", session: event.session, id: event.body.id, choice: answer(event.body.choices) })
  })
  async function attempt(call: ScriptedCall, answering: (choices: string[]) => string | null = () => null, session = run.session) {
    answer = answering
    script.push({ text: [], calls: [call] }, { text: ["ok"] })
    await say(run as Run, "go", session)
    return stored(run.events).findLast((event) => event.type === "tool_result")!.body.text as string
  }
  const globalFile = async () => JSON.parse(await readFile(join(home, "evie.json"), "utf8"))
  return { run, attempt, asked, ran: tools.ran, folder, home, globalFile }
}

test("the mode and the rules come from the global settings file", async () => {
  const { attempt, asked, ran } = await withSettings({
    permissions: { mode: "auto", rules: [{ action: "shell", match: "bun install*", effect: "ask" }] },
  })

  await attempt(edit("src/app.ts"))
  await attempt(shell("bun install"), () => YES)

  expect(asked).toHaveLength(1)
  expect(ran).toHaveLength(2)
})

test("a trusted project's own file can set its mode and add rules, which come after the global ones", async () => {
  const { run, attempt, asked, ran } = await withSettings(
    { permissions: { mode: "read-only", rules: [{ action: "shell", match: "bun *", effect: "ask" }] } },
    { permissions: { mode: "auto", rules: [{ action: "shell", match: "bun test*", effect: "allow" }] } },
  )
  const general = await run.evie.send({ type: "open_session", questions: true })

  await attempt(shell("bun test"))
  await attempt(edit("src/app.ts"))
  // The general session has the global mode.
  expect(await attempt(edit("/tmp/notes.md"), undefined, general.session)).toStartWith("Not allowed: Evie is in read-only mode")

  expect(asked).toEqual([])
  expect(ran).toHaveLength(2)
})

test("an always answer is written to the global settings file, and is there when Evie starts again", async () => {
  const first = await withSettings({ permissions: { mode: "manual" } })
  await first.attempt(shell("bun test"), always)

  const rule = { action: "shell", match: "bun test *", effect: "allow" }
  expect(await first.globalFile()).toEqual({
    model: "some/model",
    plugins: ["permissions"],
    permissions: { mode: "manual", projects: { [first.folder]: [rule] } },
  })

  // Started again on the same settings, in the same project.
  const settings = await loadSettingsFiles({ env: { EVIE_HOME: first.home } })
  const tools = standIns()
  const again = await startEvie(
    [scriptedProvider([{ text: [], calls: [shell("bun test --watch")] }, { text: ["ok"] }]).plugin, tools.plugin, ...settings.plugins],
    { home: first.home, projectFolder: first.folder, projectSettings: settings.projectSettings },
  )
  cleanups.push(again.stop)
  await say(again, "go")
  expect(tools.ran).toEqual(["shell bun test --watch"])
  expect(dirname(first.folder)).toBeDefined()
})

test("permissions settings that make no sense stop Evie at start, and say what is wrong", async () => {
  const load = async (value: unknown) => {
    const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
    cleanups.push(() => rm(home, { recursive: true, force: true }))
    await writeFile(join(home, "evie.json"), JSON.stringify({ permissions: value }))
    return loadSettingsFiles({ env: { EVIE_HOME: home } })
  }

  await expect(load({ mode: "yolo" })).rejects.toThrow('"permissions" has a "mode" that is not one of: read-only, manual, auto.')
  await expect(load({ rules: [{ action: "shell", effect: "maybe" }] })).rejects.toThrow('"permissions" has "rules" that are not a list of rules.')
  await expect(load({ projects: { "/some/project": "all" } })).rejects.toThrow('"permissions" has "projects" that do not give a list of rules')
})

// ---- Found in review --------------------------------------------------------------------------

test("a patch whose file lines are indented is judged like any other", async () => {
  const patch = (file: string): ScriptedCall => ({ name: "patch", input: { patchText: `*** Begin Patch\n   *** Add File: ${file}  \n+hello\n*** End Patch` } })
  const manual = await start()
  const readOnly = await start({ mode: "read-only" })

  await manual.attempt(patch("/somewhere/else/x.txt"), NO)
  expect(manual.asked[0]?.text).toBe("Let Evie change /somewhere/else/x.txt, which is outside the project?")
  expect(await readOnly.attempt(patch("src/x.txt"))).toStartWith("Not allowed: Evie is in read-only mode")
  expect(await manual.attempt(patch("~/.ssh/authorized_keys"))).toStartWith("Blocked by the guard:")
  // A patch in which no file can be found is not waved through.
  expect(await readOnly.attempt({ name: "patch", input: { patchText: "nothing recognisable" } })).toStartWith("Not allowed")
  expect([...manual.ran, ...readOnly.ran]).toEqual([])
})

test("an always answer never allows more than its words say", async () => {
  const { attempt, asked } = await start()

  // Setting a variable runs nothing, so it is not asked about and cannot become a rule for everything.
  await attempt(shell("DIR=src\nls $DIR"))
  expect(asked).toEqual([])

  await attempt(shell("npm test 2>&1"), always)
  expect(asked.at(-1)?.choices[1]).toBe('Always in this project: allow commands that start with "npm test"')

  // A command not written plainly is remembered whole.
  await attempt(shell("git -C sub push"), always)
  expect(asked.at(-1)?.choices[1]).toBe('Always in this project: allow exactly the command "git -C sub push"')
  await attempt(shell("git commit -m wip"), NO)
  expect(asked).toHaveLength(3)

  // "ls" is not "lsblk".
  const ruled = await start({ rules: [{ action: "shell", match: "make *", effect: "allow" }] })
  await ruled.attempt(shell("make"))
  await ruled.attempt(shell("make build"))
  await ruled.attempt(shell("makepkg"), NO)
  expect(ruled.asked).toHaveLength(1)
})

test("a command that names a place outside the project is treated as reaching outside it", async () => {
  const manual = await start()
  const auto = await start({ mode: "auto" })
  const readOnly = await start({ mode: "read-only" })

  await manual.attempt(shell("cat /etc/passwd"), NO)
  await manual.attempt(shell("cat ../../other/notes.md"), NO)
  await auto.attempt(shell("rm -rf ~/Documents/work"), NO)
  await auto.attempt(shell("mv notes.md $HOME/notes.md"), NO)
  await auto.attempt(shell("cp --target-directory=/usr/local/bin tool"), NO)
  expect(await readOnly.attempt(shell("cat ~/.config/gh/hosts.yml"))).toStartWith("Not allowed")
  // Inside the project, an address, and the usual place to throw output away are all fine.
  await auto.attempt(shell("rm -rf build/cache"))
  await auto.attempt(shell("curl https://example.com/a/../b"))
  await auto.attempt(shell("bun test 2> /dev/null"))

  expect(manual.asked).toHaveLength(2)
  expect(auto.asked).toHaveLength(3)
  expect(auto.ran).toHaveLength(3)
})

test("a command that looks harmless but writes a file or runs a program is not treated as only looking", async () => {
  const { attempt, ran } = await start({ mode: "read-only" })
  const more = [
    "sort -o out.txt in.txt",
    "uniq in.txt out.txt",
    "tree -o listing.txt",
    "git diff --output=changes.txt",
    "git log --output=log.txt",
    "rg --pre ./script pattern",
    "git remote -v add evil https://example.com/x.git",
    "git remote set-url origin https://example.com/x.git",
    "GIT_EXTERNAL_DIFF=./script git diff",
    "date -s tomorrow",
    "cat notes.md > copy.md",
  ]
  for (const command of more) expect(await attempt(shell(command))).toStartWith("Not allowed")

  for (const command of ["git remote -v", "sort names.txt | uniq -c", "git diff --stat"]) await attempt(shell(command))
  expect(ran).toHaveLength(3)
})

test("in auto mode a command that sends, publishes or deploys is asked about however it is introduced", async () => {
  const { attempt, asked, ran } = await start({ mode: "auto" })
  const outward = [
    "git -C sub push",
    "env X=1 git push",
    "time git push",
    "if true; then git push; fi",
    "for r in a b; do git push $r; done",
    "( ( git push ) )",
    "npx vercel deploy",
    "bunx wrangler deploy",
    "curl --request POST https://example.com",
    "curl -sd x https://example.com",
    "curl --json '{}' https://example.com",
    "wget --post-data=x https://example.com",
    "nc example.com 80",
  ]
  for (const command of outward) await attempt(shell(command), NO)

  expect(asked).toHaveLength(outward.length)
  expect(ran).toEqual([])
})

test("an always answer for a file outside the project covers that folder and no deeper", async () => {
  const { attempt, asked } = await start()

  await attempt(edit("/somewhere/else/notes.txt"), always)
  await attempt(edit("/somewhere/else/todo.txt"))
  await attempt(edit("/somewhere/else/deeper/down/secret.txt"), NO)
  // Searching a folder is an answer about everything under it.
  await attempt({ name: "grep", input: { pattern: "x", path: "/another/place" } }, always)
  await attempt(read("/another/place/deep/file.txt"))

  expect(asked.map((question) => question.choices[1])).toEqual([
    "Always in this project: allow changing files directly in /somewhere/else",
    "Always in this project: allow changing files directly in /somewhere/else/deeper/down",
    "Always in this project: allow reading anything under /another/place",
  ])
})

test("a link inside the project that points outside it counts as outside", async () => {
  const outside = await realpath(await mkdtemp(join(tmpdir(), "evie-outside-")))
  cleanups.push(() => rm(outside, { recursive: true, force: true }))
  const { attempt, asked, folder } = await start({ mode: "auto" })
  await symlink(outside, join(folder, "linked"))

  await attempt(edit("linked/file.txt"), NO)

  expect(asked[0]?.text).toBe(`Let Evie change ${join(outside, "file.txt")}, which is outside the project?`)
})

test("a rule that denies is not got round by writing the same thing another way", async () => {
  const { attempt, asked, ran } = await start({
    mode: "auto",
    rules: [
      { action: "shell", match: "git push *", effect: "deny" },
      { action: "read", match: ".env", effect: "deny" },
      { action: "edit", match: "*.pem", effect: "deny" },
    ],
  })

  for (const call of [
    shell("git -C . push"),
    shell("FOO=1 git push origin dev"),
    shell("cat .env"),
    shell("cat config/.env | head"),
    read("config/deep/.env"),
    read("/somewhere/else/.env"),
    edit("keys/server.pem"),
    shell("cp keys/server.pem /tmp/x"),
  ]) {
    expect(await attempt(call)).toStartWith("Not allowed: one of the owner's rules denies this.")
  }
  expect(asked).toEqual([])
  expect(ran).toEqual([])
})

test("read-only mode asks nothing and lets no rule allow a change", async () => {
  const { attempt, asked, ran } = await start({
    mode: "read-only",
    rules: [{ action: "deploy", effect: "allow" }, { action: "read", match: "docs/**", effect: "ask" }],
    remembered: {},
  })

  expect(await attempt({ name: "deploy", input: {} })).toStartWith("Not allowed")
  expect(await attempt(read("docs/guide.md"))).toStartWith("Not allowed")
  expect(asked).toEqual([])
  expect(ran).toEqual([])
})

test("a project that lists the permissions plugin itself still gets its own mode and rules", async () => {
  const { attempt, asked, ran } = await withSettings(
    { permissions: { mode: "manual" } },
    { plugins: ["permissions"], permissions: { mode: "auto", rules: [{ action: "edit", match: "*.env", effect: "deny" }] } },
  )

  await attempt(edit("src/app.ts"))
  expect(await attempt(edit("config/prod.env"))).toStartWith("Not allowed: one of the owner's rules denies this.")

  expect(asked).toEqual([])
  expect(ran).toEqual(["edit src/app.ts"])
})

// ---- Found in the second review ----------------------------------------------------------------

test("read-only mode tells a command that only looks from one that does more, as plainly written", async () => {
  const { attempt, ran } = await start({ mode: "read-only" })
  const more = [
    "npx ls",
    "bunx cat notes.md",
    "./ls",
    "cat ~root/notes",
    "sort -uo package.json names.txt",
    "tree -ao listing.txt",
    "git --git-dir=/elsewhere/.git status",
    "cat {/etc/passwd,notes.md}",
    "grep -f/etc/passwd notes.md",
    'cat "$PWD/../notes.md"',
    "cd && cat notes.md",
    "echo x>../notes.md",
  ]
  for (const command of more) expect(await attempt(shell(command))).toStartWith("Not allowed")

  const looks = ['rg "a|b" src', "ls src 2>/dev/null", "ls 2>&1", "cd src && ls", "rg '=>' src", 'echo "a && b"', "ls -la \\\n  src"]
  for (const command of looks) expect(await attempt(shell(command))).toBe("done")
  expect(ran).toHaveLength(looks.length)
})

test("a rule that denies holds behind a wrapper with options, a full path, and other ways of naming a file", async () => {
  const { attempt, ran } = await start({
    mode: "auto",
    rules: [
      { action: "shell", match: "git push *", effect: "deny" },
      { action: "read", match: ".env", effect: "deny" },
      { action: "edit", match: "secrets/**", effect: "deny" },
    ],
  })

  for (const call of [
    shell("nice -n 5 git push"),
    shell("env -i git push"),
    shell("command -p git push"),
    shell("/usr/bin/git push"),
    shell("git --git-dir .git push"),
    shell("cat<.env"),
    shell("cat {.env,notes.md}"),
    shell("bun --env-file=.env run start"),
    shell("cat .env", "config"),
    shell("rm key.txt", "secrets"),
    shell("echo x>secrets/key.txt"),
  ]) {
    expect(await attempt(call)).toStartWith("Not allowed: one of the owner's rules denies this.")
  }
  expect(ran).toEqual([])
})

test("in auto mode more ways of publishing, deploying and writing outside the project are asked about", async () => {
  const { attempt, asked, ran } = await start({ mode: "auto" })
  const commands = [
    "twine upload dist/pkg.whl",
    "gem push pkg.gem",
    "helm upgrade app chart",
    "pulumi up",
    "docker compose push",
    "docker buildx build --push .",
    "glab mr create",
    "bun x vercel deploy",
    "pnpm dlx vercel",
    "http POST example.com/hook",
    "tar -xf a.tar -C../..",
    "make -C../other clean",
    "echo x>/etc/x",
    "cd && rm -rf build",
  ]
  for (const command of commands) await attempt(shell(command), NO)

  expect(asked).toHaveLength(commands.length)
  expect(ran).toEqual([])
})

test("a rule that allows a kind of command does not stretch to reaching outside the project", async () => {
  const rules: Rule[] = [
    { action: "shell", match: "echo *", effect: "allow" },
    { action: "shell", match: "cat *", effect: "allow" },
    { action: "shell", match: "rm -rf ~/old-cache", effect: "allow" },
  ]
  const manual = await start({ rules })
  const readOnly = await start({ mode: "read-only", rules })

  await manual.attempt(shell("echo done > notes.md"))
  await manual.attempt(shell("echo x > ~/.zshrc"), NO)
  await manual.attempt(shell("cat /etc/passwd"), NO)
  // A rule for exactly one command still allows that command.
  await manual.attempt(shell("rm -rf ~/old-cache"))
  expect(await readOnly.attempt(shell("cat /etc/passwd"))).toStartWith("Not allowed")

  expect(manual.asked).toHaveLength(2)
  expect(manual.ran).toHaveLength(2)
})

test("an always answer names three words where two would cover too much", async () => {
  const { attempt, asked } = await start()

  await attempt(shell("gh pr create --fill"), always)
  await attempt(shell("bun run build"), always)
  await attempt(shell("gh pr merge 12"), NO)
  await attempt(shell("bun run deploy"), NO)

  expect(asked.map((question) => question.choices[1])).toEqual([
    'Always in this project: allow commands that start with "gh pr create"',
    'Always in this project: allow commands that start with "bun run build"',
    'Always in this project: allow commands that start with "gh pr merge"',
    'Always in this project: allow commands that start with "bun run deploy"',
  ])
})

test("a rule written with a path that goes through a link still matches", async () => {
  const real = await realpath(await mkdtemp(join(tmpdir(), "evie-real-")))
  const holder = await realpath(await mkdtemp(join(tmpdir(), "evie-links-")))
  cleanups.push(() => rm(real, { recursive: true, force: true }), () => rm(holder, { recursive: true, force: true }))
  await symlink(real, join(holder, "link"))
  const { attempt, asked } = await start({ mode: "auto", rules: [{ action: "read", match: `${join(holder, "link")}/**`, effect: "deny" }] })

  expect(await attempt(read(join(holder, "link", "a.txt")))).toStartWith("Not allowed: one of the owner's rules denies this.")
  expect(await attempt(shell(`cat ${join(holder, "link", "a.txt")}`))).toStartWith("Not allowed: one of the owner's rules denies this.")
  expect(asked).toEqual([])
})
