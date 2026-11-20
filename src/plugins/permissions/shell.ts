// What the permissions plugin can tell about a shell command from its text.
//
// This is matching on text, the same as the guard: it recognises commands
// written plainly and can be fooled by one hidden in a script or built from
// a variable. It makes the common case quiet and the obvious risk visible.
// It is not a sandbox.

// The commands in a command line: the pieces between `&&`, `||`, `;`, `|`
// and new lines, and whatever is run inside `$(…)` or backticks. Each is
// judged on its own, so `git status && git push` is not let through as
// `git status`.
export function partsOf(command: string): string[] {
  const inner = [...command.matchAll(/\$\(([^()]*)\)|`([^`]*)`/g)].map((found) => found[1] ?? found[2] ?? "")
  const pieces = [command, ...inner].flatMap((text) => text.split(/&&|\|\||[;|&\n]/))
  const parts = pieces.map((piece) => piece.trim().replace(/^[({]\s*/, "")).filter((piece) => piece !== "")
  return parts.length > 0 ? parts : [command.trim()]
}

const words = (part: string) => part.split(/\s+/).filter((word) => !/^\w+=/.test(word))

// Tools whose first argument says what they do, so a rule names both words.
const TWO_WORDS = ["git", "gh", "npm", "pnpm", "yarn", "bun", "bunx", "npx", "cargo", "go", "docker", "kubectl", "make", "pip", "uv", "brew", "terraform"]

// The start of a command that an "always" answer is remembered by: `git push`
// for `git push origin dev`, and `ls` for `ls -la`.
export function startOf(part: string): string {
  const [first = "", second] = words(part)
  return TWO_WORDS.includes(first) && second !== undefined && !second.startsWith("-") ? `${first} ${second}` : first
}

// Commands that only look. `find` and `git branch` are left out, because
// they can also delete.
const READ_ONLY = [
  "ls", "cat", "head", "tail", "wc", "pwd", "echo", "which", "whoami", "date", "tree", "file", "stat", "du", "df", "rg", "grep", "sort", "uniq", "cut", "diff", "basename", "dirname", "realpath", "true",
  "git status", "git diff", "git log", "git show", "git rev-parse", "git ls-files", "git blame", "git remote", "git describe",
]

// True for a command that only looks and changes nothing. Writing its output
// to a file, or running another command inside it, makes it more than that.
export function isReadOnly(part: string): boolean {
  if (/[<>]|\$\(|`/.test(part)) return false
  const [first = "", second = ""] = words(part)
  if (first === "git") {
    // `git remote` only lists; `git remote add` and the like change things.
    if (second === "remote") return words(part).length === 2 || words(part)[2] === "-v"
    return READ_ONLY.includes(`git ${second}`)
  }
  return READ_ONLY.includes(first)
}

// Commands that reach outside the machine: they send, publish or deploy. In
// auto mode these are asked about.
const OUTWARD: RegExp[] = [
  /^git\s+push\b/,
  /^(npm|pnpm|yarn|bun|cargo|twine|gem)\s+publish\b/,
  // gh changes things on GitHub unless it is only looking.
  /^gh\s+(?!(pr|issue|repo|run|release|workflow)\s+(view|list|status|diff|checks)\b)(?!(auth\s+status|status)\b)/,
  /^(curl|wget)\b.*(\s-X\s*(POST|PUT|PATCH|DELETE)\b|\s(-d|--data\S*|-F|--form\S*|-T|--upload-file)\b)/i,
  /^(ssh|scp|sftp|rsync|ftp)\b/,
  /^docker\s+(push|login)\b/,
  /^kubectl\s+(apply|delete|create|replace|patch|scale|rollout|exec)\b/,
  /^terraform\s+(apply|destroy)\b/,
  /^(vercel|netlify|fly|flyctl|heroku|wrangler|firebase|aws|gcloud|az)\b/,
  /^(mail|sendmail|mutt)\b/,
]

export function isOutwardFacing(part: string): boolean {
  const plain = words(part).join(" ")
  return OUTWARD.some((pattern) => pattern.test(plain))
}
