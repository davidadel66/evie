import { afterEach, expect, test } from "bun:test"
import { chmod, mkdtemp, readFile, realpath, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { git } from "./support/git.ts"
import { startEvie } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

// A project's pull request, as the web app's project card shows it, read
// with the GitHub CLI. These tests put a stand-in for `gh` first on the
// path, which answers as `gh pr view` would and notes how it was run, so
// that no real repository on GitHub is ever asked.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// Evie in a project that is a git repository on a branch of its own, with
// a stand-in `gh` that prints `answer` and stops with `code`, or with no
// `gh` at all.
async function start(gh: { answer?: unknown; said?: string; code?: number } | "missing", options: { git?: boolean } = {}) {
  const bin = await realpath(await mkdtemp(join(tmpdir(), "evie-bin-")))
  const projectFolder = await realpath(await mkdtemp(join(tmpdir(), "evie-project-")))
  cleanups.push(() => rm(bin, { recursive: true, force: true }), () => rm(projectFolder, { recursive: true, force: true }))
  if (options.git !== false) {
    await git(["init", "-q", "-b", "feature"], projectFolder)
    await writeFile(join(projectFolder, "a.md"), "a\n")
    await git(["add", "-A"], projectFolder)
    await git(["commit", "-q", "-m", "Start"], projectFolder)
  }
  if (gh !== "missing") {
    await writeFile(join(bin, "answer"), gh.answer === undefined ? "" : JSON.stringify(gh.answer))
    await writeFile(join(bin, "said"), gh.said ?? "")
    await writeFile(
      join(bin, "gh"),
      `#!/bin/sh\nprintf '%s\\n' "$PWD" "$@" > "${bin}/asked"\ncat "${bin}/answer"\ncat "${bin}/said" >&2\nexit ${gh.code ?? 0}\n`,
    )
    await chmod(join(bin, "gh"), 0o755)
  }
  // Only the stand-in is on the path, so a real `gh` is never found.
  const path = process.env.PATH
  process.env.PATH = `${bin}:/usr/bin:/bin`
  cleanups.push(async () => {
    process.env.PATH = path
  })
  const run = await startEvie([scriptedProvider([]).plugin], { projectFolder })
  cleanups.push(run.stop)
  const asked = () => readFile(join(bin, "asked"), "utf8").then((text) => text.trim().split("\n"), () => undefined)
  return { run, asked, projectFolder }
}

const PR = {
  number: 128,
  title: "Show diffs in the browser",
  state: "OPEN",
  isDraft: false,
  url: "https://github.com/someone/project/pull/128",
  baseRefName: "dev",
  statusCheckRollup: [
    { __typename: "CheckRun", name: "check", status: "COMPLETED", conclusion: "SUCCESS", detailsUrl: "https://github.com/someone/project/actions/runs/1" },
    { __typename: "CheckRun", name: "lint", status: "COMPLETED", conclusion: "FAILURE", detailsUrl: "https://github.com/someone/project/actions/runs/2" },
    { __typename: "CheckRun", name: "build", status: "IN_PROGRESS", conclusion: "", detailsUrl: "https://github.com/someone/project/actions/runs/3" },
    { __typename: "StatusContext", context: "deploy", state: "PENDING", targetUrl: "https://deploy.example/4" },
    { __typename: "CheckRun", name: "docs", status: "COMPLETED", conclusion: "SKIPPED", detailsUrl: "" },
  ],
}

test("a branch's pull request is read with gh in the project's folder: its title, number, state, base and checks", async () => {
  const { run, asked, projectFolder } = await start({ answer: PR })

  expect(await run.evie.send({ type: "read_pull_request", project: run.project!.id })).toEqual({
    number: 128,
    title: "Show diffs in the browser",
    state: "open",
    url: "https://github.com/someone/project/pull/128",
    base: "dev",
    checks: [
      { name: "check", state: "passed", url: "https://github.com/someone/project/actions/runs/1" },
      { name: "lint", state: "failed", url: "https://github.com/someone/project/actions/runs/2" },
      { name: "build", state: "running", url: "https://github.com/someone/project/actions/runs/3" },
      { name: "deploy", state: "running", url: "https://deploy.example/4" },
      { name: "docs", state: "skipped" },
    ],
  })
  // Only read: `gh pr view`, in the project's folder.
  expect(await asked()).toEqual([projectFolder, "pr", "view", "--json", "number,title,state,isDraft,url,baseRefName,statusCheckRollup"])
})

test("a draft, a merged and a closed pull request each say so", async () => {
  for (const [given, state] of [[{ state: "OPEN", isDraft: true }, "draft"], [{ state: "MERGED" }, "merged"], [{ state: "CLOSED" }, "closed"]] as const) {
    const { run } = await start({ answer: { ...PR, ...given } })
    expect(await run.evie.send({ type: "read_pull_request", project: run.project!.id })).toMatchObject({ state })
  }
})

test("there is nothing to show when the branch has no pull request, gh is not signed in, or not installed", async () => {
  const none = await start({ said: 'no pull requests found for branch "feature"', code: 1 })
  expect(await none.run.evie.send({ type: "read_pull_request", project: none.run.project!.id })).toBeNull()
  const signedOut = await start({ said: "To get started with GitHub CLI, please run:  gh auth login", code: 4 })
  expect(await signedOut.run.evie.send({ type: "read_pull_request", project: signedOut.run.project!.id })).toBeNull()
  const missing = await start("missing")
  expect(await missing.run.evie.send({ type: "read_pull_request", project: missing.run.project!.id })).toBeNull()
})

test("gh is not asked about a folder that is not in a git repository", async () => {
  const { run, asked } = await start({ answer: PR }, { git: false })
  expect(await run.evie.send({ type: "read_pull_request", project: run.project!.id })).toBeNull()
  expect(await asked()).toBeUndefined()
})

test("a pull request that could not be read for another reason, such as no network, says why", async () => {
  const { run } = await start({ said: "error connecting to api.github.com", code: 1 })
  await expect(run.evie.send({ type: "read_pull_request", project: run.project!.id })).rejects.toThrow("The pull request could not be read: error connecting to api.github.com")
})
