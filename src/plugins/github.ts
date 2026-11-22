import { gitSettingsOnly } from "../core/git.ts"
import { inGit } from "../core/projects.ts"
import type { Plugin, PullRequest } from "../core/types.ts"

// The built-in github plugin. It reads the pull request of the branch a
// project's folder is on, with the GitHub CLI, `gh`, as the owner has
// signed it in on the server's computer, for the web app's project card,
// and for the base a branch's changes are compared with. It only reads:
// `gh pr view` changes nothing on GitHub. How a check's status and
// conclusion say how it stands is adapted from T3 Code's `toCheckStatus`
// (MIT licence, notice in LICENSES/t3code-MIT.txt).

export function github(): Plugin {
  return {
    name: "github",
    setup(host) {
      host.pullRequests.add({ name: "github", read: readPullRequest })
    },
  }
}

const FIELDS = "number,title,state,isDraft,url,baseRefName,statusCheckRollup"
// How long gh may take, in milliseconds, since it asks GitHub over the network.
const LONGEST = 15_000

// What gh says when the branch has no pull request; when the repository
// has no remote on GitHub, so that none can be read; and a gh that is not
// signed in stops with code 4.
const NONE = /no pull requests found/i
const NO_REMOTE = /no git remotes|none of the git remotes/i
const SIGNED_OUT = 4

// A check as `statusCheckRollup` lists it: a check run, with a status and a
// conclusion, or a commit status, with a state.
type Rolled = { name?: string; context?: string; status?: string; conclusion?: string; state?: string; detailsUrl?: string; targetUrl?: string }

function stateOf(check: Rolled): PullRequest["checks"][number]["state"] {
  const status = check.status?.trim().toUpperCase()
  if (status !== undefined && status !== "" && status !== "COMPLETED") return "running"
  switch ((check.conclusion || check.state || "").trim().toUpperCase()) {
    case "SUCCESS":
      return "passed"
    case "ACTION_REQUIRED":
      return "waiting"
    case "FAILURE":
    case "ERROR":
    case "TIMED_OUT":
    case "STARTUP_FAILURE":
      return "failed"
    case "CANCELLED":
      return "cancelled"
    case "SKIPPED":
      return "skipped"
    case "PENDING":
    case "EXPECTED":
      return "running"
    default:
      return "neutral"
  }
}

// What gh is given of the server's environment: where to find programs,
// the home, settings and temporary folders it keeps its sign-in and work
// in, the language, and its own settings, such as GH_TOKEN. Not git's
// settings from the server's environment, which would point the git it
// runs elsewhere, nor any key.
const GIVEN = /^(PATH|HOME|LANG|LANGUAGE|LC_[A-Z_]+|TMPDIR|XDG_CONFIG_HOME|GH_[A-Z0-9_]+|GITHUB_[A-Z0-9_]+)$/

// Only an https link is given back: anything else, which a page would open
// as it is, is left out.
const https = (link: string | undefined) => {
  try {
    return link !== undefined && new URL(link).protocol === "https:" ? link : undefined
  } catch {
    return undefined
  }
}

async function readPullRequest(folder: string): Promise<PullRequest | null> {
  // A folder that is not in a repository has no branch, nor pull request.
  if ((await inGit(folder)) === false) return null
  let run
  try {
    const env = Object.fromEntries(Object.entries(process.env).filter(([name]) => GIVEN.test(name)))
    run = Bun.spawn(["gh", "pr", "view", "--json", FIELDS], {
      cwd: folder,
      // The git gh runs to find the branch and its remote runs none of the
      // commands a repository's settings can name, and reaches no other
      // computer, as the core runs it (core/git.ts); gh asks GitHub itself.
      env: { ...env, ...gitSettingsOnly(), GH_PROMPT_DISABLED: "1", NO_COLOR: "1" },
      stdin: "ignore",
      stdout: "pipe",
      stderr: "pipe",
      timeout: LONGEST,
    })
  } catch {
    throw new Error("The GitHub CLI, gh, is not installed on the computer evie runs on.")
  }
  const [said, problem, code] = await Promise.all([new Response(run.stdout).text(), new Response(run.stderr).text(), run.exited])
  // Where pull requests cannot be read, the answer says why, so that it is not taken for a branch with none.
  if (code === SIGNED_OUT) throw new Error("gh is not signed in on the computer evie runs on: run gh auth login there.")
  if (NO_REMOTE.test(problem)) throw new Error("This repository has no remote on GitHub.")
  if (NONE.test(problem)) return null
  if (code !== 0) throw new Error(`The pull request could not be read: ${problem.trim().split("\n")[0] || (run.signalCode ? "gh took too long to answer." : `gh stopped with code ${code}.`)}`)
  const read = JSON.parse(said) as { number: number; title: string; state: string; isDraft?: boolean; url: string; baseRefName: string; statusCheckRollup?: Rolled[] }
  const state = read.state === "MERGED" ? "merged" : read.state === "CLOSED" ? "closed" : read.isDraft ? "draft" : "open"
  const url = https(read.url)
  return {
    number: read.number,
    title: read.title,
    state,
    ...(url !== undefined && { url }),
    base: read.baseRefName,
    checks: (read.statusCheckRollup ?? []).map((check) => {
      const url = https(check.detailsUrl || check.targetUrl)
      return { name: check.name || check.context || "Check", state: stateOf(check), ...(url !== undefined && { url }) }
    }),
  }
}
