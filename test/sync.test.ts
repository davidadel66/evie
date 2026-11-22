import { afterEach, expect, test } from "bun:test"
import { chmod, mkdir, mkdtemp, readdir, readFile, realpath, rm, stat, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { files, shell, show, sync, type Plugin } from "../src/index.ts"
import { git, ran } from "./support/git.ts"
import { say, startEvie } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

// The sync plugin with a stand-in for GitHub: `gh` is answered here, and
// each repository it makes is a bare repository in a temporary folder. The
// real git does the rest, with none of the settings of the user running the
// tests. Nothing here reaches GitHub.

const cleanups: (() => Promise<void> | void)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

const exists = (path: string) => stat(path).then(() => true, () => false)

async function tempFolder(prefix: string) {
  const folder = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

type Ran = { code: number; stdout: string; stderr: string }

// The owner's GitHub account, as `gh` sees it: the repositories on it, by
// name, each a bare repository in `folder`.
async function standInGitHub() {
  const folder = await tempFolder("evie-github-")
  const repositories = new Map<string, { isPrivate: boolean }>()
  const bare = (name: string) => join(folder, `${name}.git`)
  const no = (stderr: string): Ran => ({ code: 1, stdout: "", stderr })
  // What `gh` says to anything while it is failing, such as when it is not logged in.
  let failing: string | undefined
  async function gh(args: string[]): Promise<Ran> {
    if (failing) return no(failing)
    const [area, action, name = ""] = args
    if (area === "repo" && action === "view") {
      const found = repositories.get(name)
      if (!found) return no(`GraphQL: Could not resolve to a Repository with the name 'someone/${name}'.`)
      return { code: 0, stdout: JSON.stringify({ isPrivate: found.isPrivate, url: bare(name), sshUrl: bare(name) }), stderr: "" }
    }
    if (area === "repo" && action === "create") {
      if (repositories.has(name)) return no("GraphQL: Name already exists on this account (createRepository)")
      await own(name, { isPrivate: args.includes("--private") })
      await git(["remote", "add", args[args.indexOf("--remote") + 1]!, bare(name)], args[args.indexOf("--source") + 1]!)
      return { code: 0, stdout: `https://github.com/someone/${name}\n`, stderr: "" }
    }
    if (area === "config" && action === "get") return { code: 0, stdout: "https\n", stderr: "" }
    return no(`The stand-in for gh does not know "gh ${args.join(" ")}".`)
  }
  async function own(name: string, how: { isPrivate: boolean }) {
    await git(["init", "--bare", "--quiet", "--initial-branch=main", bare(name)], folder)
    repositories.set(name, how)
  }
  // The commits on a repository's main branch, newest first, each with the files it holds.
  async function commits(name: string) {
    const log = await ran(["git", "--git-dir", bare(name), "log", "--format=%H %s", "main"], folder)
    if (log.code !== 0) return []
    return Promise.all(
      log.stdout.trim().split("\n").map(async (line) => {
        const [hash, ...subject] = line.split(" ")
        const files = (await git(["--git-dir", bare(name), "ls-tree", "-r", "--name-only", hash!], folder)).trim().split("\n")
        return { subject: subject.join(" "), files }
      }),
    )
  }
  return { folder, gh, own, commits, repositories, failWith: (text: string | undefined) => (failing = text) }
}

// Evie with the file tools, the shell and the sync plugin, and a project
// whose .evie folder holds its settings and syncs as `setting` says, which
// is "private" unless given, and nothing when given as undefined.
// With `dataFolderInside`, the project's .evie folder is evie's data folder,
// as it is for a project at the home folder.
// `scriptFor` makes the model's replies once the project folder is known.
// `pushes` stands in for `git push`, when given.
async function start(
  options: {
    setting?: "private" | "none" | undefined
    script?: ScriptedReply[]
    after?: number
    every?: number
    dataFolderInside?: true
    pushes?: (signal: AbortSignal | undefined) => Promise<Ran>
  } = {},
  scriptFor?: (projectFolder: string) => Promise<ScriptedReply[]>,
) {
  const setting = "setting" in options ? options.setting : "private"
  const github = await standInGitHub()
  const projectFolder = await tempFolder("evie-project-")
  const home = options.dataFolderInside ? join(projectFolder, ".evie") : await tempFolder("evie-home-")
  await mkdir(join(projectFolder, ".evie"), { recursive: true })
  await writeFile(join(projectFolder, ".evie", "evie.json"), JSON.stringify(setting === undefined ? {} : { sync: setting }))
  // Every command the plugin runs, in order.
  const commands: string[][] = []
  const stopped = new AbortController()
  cleanups.push(() => stopped.abort())
  const plugin: Plugin = sync({
    dataFolder: home,
    setting: async () => setting,
    after: options.after ?? 60_000,
    every: options.every ?? 60 * 60_000,
    run: async (command, cwd, signal, env) => {
      commands.push(command)
      if (command[0] === "gh") return github.gh(command.slice(1))
      if (options.pushes && command.includes("push")) return options.pushes(signal)
      return ran(command, cwd, env)
    },
    signal: stopped.signal,
  })
  const model = scriptedProvider(scriptFor ? await scriptFor(projectFolder) : (options.script ?? []))
  const run = await startEvie([model.plugin, files(), shell(), show({ dataFolder: home }), plugin], { home, projectFolder })
  cleanups.push(run.stop)
  const evieFolder = join(projectFolder, ".evie")
  const syncNow = async () => (await run.evie.send({ type: "run_command", session: run.session, name: "sync", args: "" })).text
  return { run, github, projectFolder, evieFolder, commands, syncNow, name: `evie-${run.project!.name.toLowerCase()}` }
}

// ---- The first sync ------------------------------------------------------------------------------

test("the first /sync makes .evie/ a git repository on main, makes a private GitHub repository named for the project, and pushes to it", async () => {
  const { github, evieFolder, commands, syncNow, name } = await start()

  const said = await syncNow()

  expect(said).toBe(`Made the private GitHub repository ${name} for ${evieFolder}, and pushed what it holds there.`)
  expect(commands).toContainEqual(["gh", "repo", "create", name, "--private", "--source", evieFolder, "--remote", "origin"])
  expect(await github.commits(name)).toEqual([{ subject: "Sync from evie", files: ["evie.json"] }])
  expect((await readFile(join(evieFolder, ".git", "HEAD"), "utf8")).trim()).toBe("ref: refs/heads/main")
})

test("a later /sync commits and pushes what changed, and with nothing changed, pushes nothing", async () => {
  const { github, evieFolder, commands, syncNow, name } = await start()
  await syncNow()
  await mkdir(join(evieFolder, "dashboards"))
  await writeFile(join(evieFolder, "dashboards", "usage.md"), "# Usage\n")

  expect(await syncNow()).toBe(`Pushed the changes in ${evieFolder} to ${name} on GitHub.`)
  expect(await github.commits(name)).toEqual([
    { subject: "Sync from evie", files: ["dashboards/usage.md", "evie.json"] },
    { subject: "Sync from evie", files: ["evie.json"] },
  ])

  const pushes = () => commands.filter((command) => command.includes("push")).length
  const before = pushes()
  expect(await syncNow()).toBe(`Nothing to sync: ${evieFolder} has not changed since it was last pushed.`)
  expect(pushes()).toBe(before)
  expect(await github.commits(name)).toHaveLength(2)
})

test("syncing runs none of the commands the .evie folder's own git settings name, and pushes all the same", async () => {
  const { github, evieFolder, syncNow, name } = await start()
  await syncNow()
  const noted = await tempFolder("evie-ran-")
  const command = join(noted, "noted")
  await writeFile(command, `#!/bin/sh\ntouch "${noted}/$1"\ncat\n`)
  await chmod(command, 0o755)
  await mkdir(join(noted, "hooks"))
  for (const hook of ["pre-commit", "commit-msg", "post-commit", "pre-push", "post-index-change"]) {
    await writeFile(join(noted, "hooks", hook), `#!/bin/sh\ntouch "${noted}/hook-${hook}"\n`)
    await chmod(join(noted, "hooks", hook), 0o755)
  }
  const settings: [string, string][] = [
    ["core.fsmonitor", `${command} fsmonitor`],
    ["core.hooksPath", join(noted, "hooks")],
    ["filter.evil.clean", `${command} clean`],
    ["filter.evil.process", `${command} process`],
    ["filter.evil.required", "true"],
    ["commit.gpgSign", "true"],
    ["gpg.program", `${command} gpg`],
  ]
  for (const [setting, value] of settings) await git(["--git-dir", join(evieFolder, ".git"), "config", setting, value], evieFolder)
  await writeFile(join(evieFolder, ".gitattributes"), "* filter=evil\n")
  await writeFile(join(evieFolder, "notes.md"), "notes\n")

  expect(await syncNow()).toBe(`Pushed the changes in ${evieFolder} to ${name} on GitHub.`)
  expect(await github.commits(name)).toHaveLength(2)
  expect((await readdir(noted)).filter((each) => each !== "noted" && each !== "hooks")).toEqual([])
})

test("the repository is named evie- and the project's name in lower case, with spaces and other characters made into dashes", async () => {
  const { run, github, commands } = await start()
  const folder = join(await tempFolder("evie-place-"), "money")
  await run.evie.send({ type: "new_project", folder, name: "Family Finances & Café" })
  const { session } = await run.evie.send({ type: "open_session", folder })

  await run.evie.send({ type: "run_command", session, name: "sync", args: "" })

  expect(commands.filter((command) => command[0] === "gh" && command[2] === "create").map((command) => command[3])).toEqual(["evie-family-finances-cafe"])
  expect(github.repositories.get("evie-family-finances-cafe")).toEqual({ isPrivate: true })
})

test("a repository of that name already on the account is never taken over: the next free name, with -2, -3 and so on, is made", async () => {
  const { github, evieFolder, syncNow, name } = await start()
  await github.own(name, { isPrivate: true })
  await github.own(`${name}-2`, { isPrivate: false })

  expect(await syncNow()).toBe(`Made the private GitHub repository ${name}-3 for ${evieFolder}, and pushed what it holds there.`)
  expect(github.repositories.get(`${name}-3`)).toEqual({ isPrivate: true })
  expect(await github.commits(`${name}-3`)).toHaveLength(1)
  expect(await github.commits(name)).toEqual([])
  expect(await github.commits(`${name}-2`)).toEqual([])
})

// ---- Syncing after Evie changes something --------------------------------------------------

const write = (path: string, content: string) => ({ name: "write", input: { path, content } })
// One turn in which the model makes these calls, one after another, and then answers.
const turn = (...calls: { name: string; input: Record<string, unknown> }[]): ScriptedReply[] => [...calls.map((call) => ({ text: [], calls: [call] })), { text: ["Done."] }]

// Settles once the repository has this many commits.
async function commitsReach(github: Awaited<ReturnType<typeof standInGitHub>>, name: string, count: number) {
  while ((await github.commits(name)).length < count) await Bun.sleep(10)
}

test("a while after Evie last changed something in .evie/, what changed is committed and pushed, a burst of changes as one commit", async () => {
  const { run, github, name } = await start({ after: 100, script: turn(write(".evie/notes.md", "one\n"), write(".evie/plan.md", "two\n")) })

  await say(run, "write it down")
  await commitsReach(github, name, 1)
  await Bun.sleep(300)

  expect(await github.commits(name)).toEqual([{ subject: "Sync from evie", files: ["evie.json", "notes.md", "plan.md"] }])
})

test("a change named by a path through a link to the project is known to be in .evie/", async () => {
  const link = join(await tempFolder("evie-links-"), "project")
  const { run, github, name } = await start({ after: 50 }, async (projectFolder) => {
    await symlink(projectFolder, link)
    return turn(write(join(link, ".evie", "linked.md"), "through the link\n"))
  })

  await say(run, "write it down")
  await commitsReach(github, name, 1)

  expect((await github.commits(name))[0]!.files).toEqual(["evie.json", "linked.md"])
})

test("a file in .evie/ whose name starts with two dots is in .evie/", async () => {
  const { run, github, name } = await start({ after: 50, script: turn(write(".evie/..notes.md", "dots\n")) })

  await say(run, "write it down")
  await commitsReach(github, name, 1)

  expect((await github.commits(name))[0]!.files).toEqual(["..notes.md", "evie.json"])
})

test("a change outside .evie/ is no reason to sync, and a command, which may have changed it, is", async () => {
  const { run, github, commands, name } = await start({ after: 50, script: [...turn(write("README.md", "# Hello\n")), ...turn({ name: "shell", input: { command: "echo hi > .evie/hello.md" } })] })

  await say(run, "write the readme")
  await Bun.sleep(250)
  expect(commands).toEqual([])

  await say(run, "say hello")
  await commitsReach(github, name, 1)
  expect((await github.commits(name))[0]!.files).toEqual(["evie.json", "hello.md"])
})

test("a file shown is kept in .evie/, which is a reason to sync", async () => {
  const { run, github, name } = await start({ after: 50, script: [...turn(write("chart.md", "# Chart\n")), ...turn({ name: "show", input: { paths: ["chart.md"] } })] })

  await say(run, "write the chart")
  await say(run, "show it")
  await commitsReach(github, name, 1)

  expect((await github.commits(name))[0]!.files).toEqual(["evie.json", expect.stringMatching(/^shown\/\d{4}-\d{2}-\d{2}\/chart\.md$/)])
})

test("once an hour, what changed in .evie/ is synced whatever Evie did, as when the owner edits a file there himself", async () => {
  const { github, evieFolder, syncNow, name } = await start({ every: 100 })
  await syncNow()

  await writeFile(join(evieFolder, "by-hand.md"), "mine\n")
  await commitsReach(github, name, 2)

  expect((await github.commits(name))[0]!.files).toEqual(["by-hand.md", "evie.json"])
})

// ---- When a try fails --------------------------------------------------------------------------

test("a try that fails never fails the turn: it is kept, /sync says why, and the next try tries again", async () => {
  const { run, github, commands, evieFolder, syncNow, name } = await start({ after: 50, script: turn(write(".evie/notes.md", "one\n")) })
  const notLoggedIn = "To get started with GitHub CLI, please run:  gh auth login"
  github.failWith(notLoggedIn)

  await say(run, "write it down")
  while (!commands.some((command) => command[0] === "gh" && command[2] === "create")) await Bun.sleep(10)

  expect(run.events.findLast((event) => event.type === "turn_end")).toMatchObject({ body: { reason: "completed" } })
  const failed = `\`gh repo create\` failed: ${notLoggedIn}.`
  expect(await syncNow()).toBe(`Could not sync ${evieFolder}: ${failed} It is tried again a minute after evie next changes something there, and every hour.`)

  github.failWith(undefined)
  expect(await syncNow()).toBe(`Made the private GitHub repository ${name} for ${evieFolder}, and pushed what it holds there. The try before this one had failed: ${failed}`)
  expect(await github.commits(name)).toEqual([{ subject: "Sync from evie", files: ["evie.json", "notes.md"] }])
})

// ---- Projects that are not synced -------------------------------------------------------------

test("a project set to none, or with no setting, is never pushed, and /sync says why", async () => {
  const script = turn(write(".evie/notes.md", "one\n"), { name: "shell", input: { command: "true" } })
  const none = await start({ setting: "none", after: 20, every: 20, script })
  const unset = await start({ setting: undefined, after: 20, every: 20, script })

  await say(none.run, "write it down")
  await say(unset.run, "write it down")
  await Bun.sleep(200)

  expect(await none.syncNow()).toBe('This project\'s .evie folder is kept on this computer only: its "sync" setting is "none".')
  expect(await unset.syncNow()).toBe(
    "This project's .evie folder is not synced. To back it up to a private GitHub repository, set \"sync\": \"private\" in its " +
      ".evie/evie.json; that file is used once the project is trusted.",
  )
  expect([...none.commands, ...unset.commands]).toEqual([])
  expect(await exists(join(none.evieFolder, ".git"))).toBe(false)
})

test("a .evie folder that is Evie's data folder, as for a project at the home folder, is never synced", async () => {
  const { evieFolder, commands, syncNow } = await start({ dataFolderInside: true })

  expect(await syncNow()).toStartWith(`Could not sync ${evieFolder}: It is evie's data folder, which holds its keys and state, and is never synced.`)
  expect(commands).toEqual([])
})

// ---- Pushing only to the private repository Evie made ---------------------------------------------

test("a remote Evie did not set up itself is never pushed to, and /sync says how to let Evie make its own", async () => {
  const { github, evieFolder, syncNow } = await start()
  await github.own("somewhere-else", { isPrivate: false })
  await git(["init", "--quiet", "--initial-branch=main"], evieFolder)
  await git(["remote", "add", "origin", join(github.folder, "somewhere-else.git")], evieFolder)

  expect(await syncNow()).toStartWith(
    `Could not sync ${evieFolder}: Its git repository has a remote, origin, that evie did not set up, ${join(github.folder, "somewhere-else.git")}, ` +
      `so nothing was pushed. To have evie make a private repository for it instead, remove that remote: git -C ${evieFolder} remote remove origin.`,
  )
  expect(await github.commits("somewhere-else")).toEqual([])
})

test("before every push the repository is checked to be private: one made public since, or that cannot be checked, is not pushed to", async () => {
  const { github, evieFolder, syncNow, name } = await start()
  await syncNow()

  github.repositories.set(name, { isPrivate: false })
  await writeFile(join(evieFolder, "secret.md"), "mine\n")
  expect(await syncNow()).toStartWith(
    `Could not sync ${evieFolder}: The repository ${name} on GitHub is public now, so nothing was pushed to it. Make it private again on GitHub, then /sync.`,
  )

  github.repositories.set(name, { isPrivate: true })
  github.failWith("error connecting to api.github.com")
  expect(await syncNow()).toStartWith(
    `Could not sync ${evieFolder}: Whether the repository ${name} on GitHub is private could not be checked, so nothing was pushed: ` +
      "`gh repo view` failed: error connecting to api.github.com.",
  )
  expect(await github.commits(name)).toHaveLength(1)
})

test("a remote removed by hand is set back to the repository Evie made, rather than a new one made", async () => {
  const { github, evieFolder, commands, syncNow, name } = await start()
  await syncNow()
  await git(["remote", "remove", "origin"], evieFolder)
  await writeFile(join(evieFolder, "later.md"), "later\n")

  expect(await syncNow()).toBe(`Pushed the changes in ${evieFolder} to ${name} on GitHub.`)
  expect(await github.commits(name)).toHaveLength(2)
  expect(commands.filter((command) => command[0] === "gh" && command[2] === "create")).toHaveLength(1)
})

test("a push refused because the repository has commits this one does not says that nothing was overwritten, and what to do", async () => {
  const { github, evieFolder, syncNow, name } = await start()
  await syncNow()
  // Another computer syncs to the same repository.
  const elsewhere = join(await tempFolder("evie-elsewhere-"), "copy")
  await git(["clone", "--quiet", join(github.folder, `${name}.git`), elsewhere], github.folder)
  await writeFile(join(elsewhere, "theirs.md"), "theirs\n")
  await git(["add", "theirs.md"], elsewhere)
  await git(["commit", "--quiet", "-m", "From elsewhere"], elsewhere)
  await git(["push", "--quiet", "origin", "main"], elsewhere)
  await writeFile(join(evieFolder, "mine.md"), "mine\n")

  expect(await syncNow()).toBe(
    `Could not sync ${evieFolder}: The repository ${name} on GitHub has commits that ${evieFolder} does not, as when another computer ` +
      "has synced to it, so the push was refused and nothing there was overwritten. To bring them in, run " +
      `git -C ${evieFolder} pull --no-rebase origin main, then /sync. It is tried again a minute after evie next changes something there, and every hour.`,
  )
  expect((await github.commits(name)).map((commit) => commit.subject)).toEqual(["From elsewhere", "Sync from evie"])
})

test("a .evie folder that holds what a data folder holds, as a copy of one does, is never synced", async () => {
  for (const marker of ["evie.db", ".env", "server-token"]) {
    const { evieFolder, commands, syncNow } = await start()
    await writeFile(join(evieFolder, marker), "stands for a data folder's own file\n")

    expect(await syncNow()).toStartWith(`Could not sync ${evieFolder}: It holds ${marker}, as an evie data folder does, and a data folder, with its keys and state, is never synced.`)
    expect(commands).toEqual([])
  }
})

// ---- Work across every project, and stopping ------------------------------------------------

test("once Evie is the only one running, every project is synced hourly, not only those opened since it started", async () => {
  const { run, github } = await start({ every: 100 })
  const folder = join(await tempFolder("evie-place-"), "ledger")
  await run.evie.send({ type: "new_project", folder })

  await run.evie.resumeWork()
  await commitsReach(github, "evie-ledger", 1)

  expect((await github.commits("evie-ledger"))[0]!.files).toEqual(["evie.json"])
})

test("a push under way is stopped when Evie stops", async () => {
  let started = false
  const { run, syncNow } = await start({
    pushes: (signal) =>
      new Promise((resolve) => {
        started = true
        signal?.addEventListener("abort", () => resolve({ code: 143, stdout: "", stderr: "stopped" }))
      }),
  })
  await run.evie.resumeWork()

  const syncing = syncNow()
  while (!started) await Bun.sleep(10)
  await run.evie.close()

  expect(await syncing).toStartWith("Could not sync")
})
