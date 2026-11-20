// What the permissions plugin can tell about a shell command from its text.
//
// This is matching on text, the same as the guard: it recognises commands
// written plainly and can be fooled by one hidden in a script or built from
// a variable. It makes the common case quiet and the obvious risk visible.
// It is not a sandbox. Where it cannot tell, it leans towards asking.

// The commands in a command line: the pieces between `&&`, `||`, `;`, `|`,
// `&` and new lines, and whatever is run inside `$(…)` or backticks. Each is
// judged on its own, so `git status && git push` is not let through as
// `git status`. An `&` that is part of a redirect, as in `2>&1`, does not split.
export function partsOf(command: string): string[] {
  const inner = [...command.matchAll(/\$\(([^()]*)\)|`([^`]*)`/g)].map((found) => found[1] ?? found[2] ?? "")
  const pieces = [command, ...inner].flatMap((text) => text.split(/&&|\|\||;|\|&?|\n|(?<![>&])&(?![>&])/))
  const parts = pieces.map((piece) => piece.trim()).filter((piece) => piece !== "")
  return parts.length > 0 ? parts : [command.trim()]
}

// Words that run the command after them, and brackets that group commands.
// They are looked past to find the command itself.
const LOOKED_PAST = ["then", "do", "else", "elif", "if", "while", "until", "!", "time", "nohup", "exec", "command", "builtin", "env", "nice", "(", "{"]
const isAssignment = (word: string) => /^\w+=/.test(word)

// A command as its plain words: with the words that only introduce it taken
// off the front, along with any `NAME=value` settings, and git's own options
// that come before its subcommand. `changed` says whether settings or a
// change of folder were taken off, which can alter what the command does.
export function plainWords(part: string): { words: string[]; changed: boolean } {
  let words = part.replace(/^[({\s]+/, "").split(/\s+/).filter((word) => word !== "")
  let changed = false
  while (words.length > 0 && (LOOKED_PAST.includes(words[0]!) || isAssignment(words[0]!))) {
    if (isAssignment(words[0]!)) changed = true
    words = words.slice(1)
  }
  if (words[0] === "git") {
    // `git -C folder push` and `git -c name=value push` are still a push.
    let at = 1
    while (words[at] === "-C" || words[at] === "-c") {
      at += 2
      changed = true
    }
    while (words[at]?.startsWith("-")) at += 1
    if (at > 1) words = ["git", ...words.slice(at)]
  }
  // A tool run through a package runner is that tool.
  if (["npx", "bunx"].includes(words[0] ?? "") && words.length > 1) words = words.slice(1)
  return { words: words.map((word) => word.replace(/[)}]+$/, "")).filter((word) => word !== ""), changed }
}

// Tools whose first argument says what they do, so a rule names both words.
const TWO_WORDS = ["git", "gh", "npm", "pnpm", "yarn", "bun", "cargo", "go", "docker", "kubectl", "make", "pip", "uv", "brew", "terraform"]

// The start of a command that an "always" answer is remembered by: `git push`
// for `git push origin dev`, and `ls` for `ls -la`. Undefined when the
// command is not written plainly enough to name a start for, in which case
// it is remembered whole.
export function startOf(part: string): string | undefined {
  const { words, changed } = plainWords(part)
  const [first, second] = words
  if (changed || first === undefined || !/^[\w./-]+$/.test(first) || !part.trimStart().startsWith(first)) return undefined
  if (!TWO_WORDS.includes(first)) return first
  return second !== undefined && /^[\w:.-]+$/.test(second) && !second.startsWith("-") ? `${first} ${second}` : undefined
}

// Commands that only look, and for some, the options that make them do more:
// write a file, change a setting, or run another program.
const READ_ONLY: Record<string, RegExp | undefined> = {
  ls: undefined, cat: undefined, head: undefined, tail: undefined, wc: undefined, pwd: undefined, echo: undefined,
  which: undefined, whoami: undefined, tree: /^-o/, file: undefined, stat: undefined, du: undefined, df: undefined,
  date: /^(-s|--set)/, rg: /^--pre/, grep: undefined, sort: /^(-o|--output)/, uniq: undefined, cut: undefined,
  diff: undefined, basename: undefined, dirname: undefined, realpath: undefined, true: undefined,
  "git status": /^--output/, "git diff": /^(--output|--ext-diff)/, "git log": /^(--output|--ext-diff)/,
  "git show": /^(--output|--ext-diff)/, "git rev-parse": undefined, "git ls-files": undefined, "git blame": undefined,
  "git describe": undefined,
}

// True for a command that only looks and changes nothing. Writing its output
// to a file, running another command inside it, or setting a variable for it
// makes it more than that. `find` and `git branch` are left out, because
// they can also delete.
export function isReadOnly(part: string): boolean {
  if (/[<>]|\$\(|`/.test(part)) return false
  const { words, changed } = plainWords(part)
  // Nothing but `NAME=value`: a variable is set and no command is run.
  if (words.length === 0) return part.trim().split(/\s+/).every(isAssignment)
  if (changed) return false
  const [first = "", second = ""] = words
  // `git remote` and `git remote -v` list; any other form changes things.
  if (first === "git" && second === "remote") return words.length === 2 || (words.length === 3 && words[2] === "-v")
  const name = first === "git" ? `git ${second}` : first
  if (!Object.hasOwn(READ_ONLY, name)) return false
  const doesMore = READ_ONLY[name]
  const rest = words.slice(first === "git" ? 2 : 1)
  if (doesMore && rest.some((word) => doesMore.test(word))) return false
  // `uniq in out` writes its second file.
  if (name === "uniq" && rest.filter((word) => !word.startsWith("-")).length > 1) return false
  return true
}

// Commands that reach outside the machine: they send, publish or deploy. In
// auto mode these are asked about.
const OUTWARD: RegExp[] = [
  /^git (push|send-email)\b/,
  /^(npm|pnpm|yarn|bun|cargo|twine|gem) publish\b/,
  // gh changes things on GitHub unless it is only looking.
  /^gh (?!(pr|issue|repo|run|release|workflow) (view|list|status|diff|checks)\b)(?!(auth status|status)\b)/,
  /^curl\b.* (-[a-zA-Z]*[dFTX]\S*|--(request|data\S*|form\S*|upload-file|json)\b)/,
  /^wget\b.* --(post-data|post-file|method|body-data|body-file)\b/,
  /^(ssh|scp|sftp|rsync|ftp|nc|ncat|netcat|telnet)\b/,
  /^docker (push|login)\b/,
  /^kubectl (apply|delete|create|replace|patch|scale|rollout|exec)\b/,
  /^terraform (apply|destroy)\b/,
  /^(vercel|netlify|fly|flyctl|heroku|wrangler|firebase|aws|gcloud|az|supabase)\b/,
  /^(mail|sendmail|mutt)\b/,
]

export function isOutwardFacing(part: string): boolean {
  const plain = plainWords(part).words.join(" ")
  return OUTWARD.some((pattern) => pattern.test(plain))
}

// The words of a command that name a place on disk, as the shell would read
// them: a full path, one starting from the home folder, or one that climbs
// out with `..`. Used to tell a command that reaches outside the project.
// Addresses such as https://… are not places on disk.
export function placesIn(part: string): string[] {
  const places: string[] = []
  for (const raw of part.split(/\s+/)) {
    // The value of `--flag=value` and `NAME=value` counts too.
    for (const word of [raw, raw.includes("=") ? raw.slice(raw.indexOf("=") + 1) : ""]) {
      const tidy = word.replace(/^["'(]+|["');,]+$/g, "").replace(/^\d*[<>]+&?/, "")
      if (tidy === "" || tidy.startsWith("-") || /^[a-z][a-z0-9+.-]*:\/\//i.test(tidy)) continue
      if (/^(\/|~|\$HOME\b|\$\{HOME\})/.test(tidy) || /(^|\/)\.\.(\/|$)/.test(tidy)) places.push(tidy)
    }
  }
  return places
}

// Every word of a command that could be a file, for holding a command to the
// rules that deny reading or changing a file.
export function wordsIn(part: string): string[] {
  return part.split(/\s+/).map((word) => word.replace(/^["'(]+|["');,]+$/g, "").replace(/^\d*[<>]+&?/, "")).filter((word) => word !== "" && !word.startsWith("-"))
}
