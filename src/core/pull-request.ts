import { inGit } from "./projects.ts"
import type { Project, PullRequest } from "./types.ts"

// The pull request of the branch a project's folder is on, read with the
// GitHub CLI, `gh`, as the owner has signed it in on the server's computer.
// It only reads: `gh pr view` changes nothing on GitHub. How a check's
// status and conclusion say how it stands is adapted from T3 Code's
// `toCheckStatus` (MIT licence, notice in LICENSES/t3code-MIT.txt).

const FIELDS = "number,title,state,isDraft,url,baseRefName,statusCheckRollup"
// How long gh may take, in milliseconds, since it asks GitHub over the network.
const LONGEST = 15_000

// What gh says when there is no pull request to read, or it cannot be
// asked: no pull request for the branch, or no remote on GitHub. A gh that
// is not signed in stops with code 4.
const NOTHING_TO_READ = /no pull requests found|no git remotes|none of the git remotes/i
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

export async function readPullRequest(project: Project): Promise<PullRequest | null> {
  // A folder that is not in a repository has no branch, nor pull request.
  if ((await inGit(project.folder)) === false) return null
  let run
  try {
    // The path is the server's own, as it is now, so that the gh found is the one the owner has.
    run = Bun.spawn(["gh", "pr", "view", "--json", FIELDS], {
      cwd: project.folder,
      env: { ...process.env, GH_PROMPT_DISABLED: "1", NO_COLOR: "1" },
      stdin: "ignore",
      stdout: "pipe",
      stderr: "pipe",
      timeout: LONGEST,
    })
  } catch {
    // No gh installed.
    return null
  }
  const [said, problem, code] = await Promise.all([new Response(run.stdout).text(), new Response(run.stderr).text(), run.exited])
  if (code === SIGNED_OUT || NOTHING_TO_READ.test(problem)) return null
  if (code !== 0) throw new Error(`The pull request could not be read: ${problem.trim().split("\n")[0] || (run.signalCode ? "gh took too long to answer." : `gh stopped with code ${code}.`)}`)
  const read = JSON.parse(said) as { number: number; title: string; state: string; isDraft?: boolean; url: string; baseRefName: string; statusCheckRollup?: Rolled[] }
  const state = read.state === "MERGED" ? "merged" : read.state === "CLOSED" ? "closed" : read.isDraft ? "draft" : "open"
  return {
    number: read.number,
    title: read.title,
    state,
    url: read.url,
    base: read.baseRefName,
    checks: (read.statusCheckRollup ?? []).map((check) => {
      const url = check.detailsUrl || check.targetUrl
      return { name: check.name || check.context || "Check", state: stateOf(check), ...(url && { url }) }
    }),
  }
}
