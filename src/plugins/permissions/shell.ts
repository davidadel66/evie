// What the permissions plugin can tell about a shell command from its text.
//
// This is matching on text, the same as the guard: it recognises commands
// written plainly and can be fooled by one hidden in a script or built from
// a variable. It makes the common case quiet and the obvious risk visible.
// It is not a sandbox. Where it cannot tell, it leans towards asking.

// Sets quoted text aside while a command is taken apart, so that `rg "a|b"`
// is one command and not two. Only text the shell would not run is set
// aside: single quotes, and double quotes with no `$` or backtick in them.
function setQuotesAside(command: string): { text: string; putBack(text: string): string } {
  const quoted: string[] = []
  const text = command.replace(/\\\n/g, " ").replace(/'[^']*'|"[^"$`\\]*"/g, (found) => {
    quoted.push(found)
    return `\u0000${quoted.length - 1}\u0000`
  })
  return { text, putBack: (piece) => piece.replace(/\u0000(\d+)\u0000/g, (_all, index: string) => quoted[Number(index)]!) }
}

// The commands in a command line: the pieces between `&&`, `||`, `;`, `|`,
// `&` and new lines, and whatever is run inside `$(…)` or backticks. Each is
// judged on its own, so `git status && git push` is not let through as
// `git status`. An `&` that is part of a redirect, as in `2>&1`, does not split.
export function partsOf(command: string): string[] {
  const { text, putBack } = setQuotesAside(command)
  const inner = [...text.matchAll(/\$\(([^()]*)\)|`([^`]*)`/g)].map((found) => found[1] ?? found[2] ?? "")
  const pieces = [text, ...inner].flatMap((piece) => piece.split(/&&|\|\||;|\|&?|\n|(?<![>&])&(?![>&])/))
  const parts = pieces.map((piece) => putBack(piece.trim())).filter((piece) => piece !== "")
  return parts.length > 0 ? parts : [command.trim()]
}

// Words that run the command after them, and brackets that group commands.
// They are looked past, with their options, to find the command itself.
const LOOKED_PAST = ["then", "do", "else", "elif", "if", "while", "until", "!", "time", "nohup", "exec", "command", "builtin", "env", "nice", "(", "{"]
const isAssignment = (word: string) => /^\w+=/.test(word)
// git's own options that take a value as the next word.
const GIT_TAKES_VALUE = ["-C", "-c", "--git-dir", "--work-tree", "--namespace", "--exec-path", "--super-prefix", "--config-env"]

// A command as its plain words: with the words that only introduce it taken
// off the front, along with any `NAME=value` settings, and git's own options
// that come before its subcommand.
//   `altered`  settings or options were taken off that can change what the
//              command does, so it is not known to be harmless
//   `byPath`   the program was named by a path, such as ./ls, so it is not
//              known to be the usual program of that name
export function plainWords(part: string): { words: string[]; altered: boolean; byPath: boolean } {
  let words = part.replace(/^[({\s]+/, "").split(/\s+/).filter((word) => word !== "")
  let altered = false
  while (words.length > 0) {
    if (isAssignment(words[0]!)) {
      altered = true
      words = words.slice(1)
    } else if (LOOKED_PAST.includes(words[0]!)) {
      words = words.slice(1)
      while (words.length > 0 && (words[0]!.startsWith("-") || /^\d+$/.test(words[0]!))) words = words.slice(1)
    } else break
  }
  const byPath = words[0]?.includes("/") ?? false
  if (byPath) words = [words[0]!.slice(words[0]!.lastIndexOf("/") + 1), ...words.slice(1)]
  if (words[0] === "git") {
    // `git -C folder push` and `git -c name=value push` are still a push.
    let at = 1
    while (words[at]?.startsWith("-")) {
      if (!["--no-pager", "-P"].includes(words[at]!)) altered = true
      at += GIT_TAKES_VALUE.includes(words[at]!) ? 2 : 1
    }
    words = ["git", ...words.slice(at)]
  }
  return { words: words.map((word) => word.replace(/[)}]+$/, "")).filter((word) => word !== ""), altered, byPath }
}

// Tools whose first argument says what they do, so a rule names both words.
const TWO_WORDS = ["git", "gh", "npm", "pnpm", "yarn", "bun", "cargo", "go", "docker", "kubectl", "make", "pip", "uv", "brew", "terraform"]
const RUNS_SCRIPTS = ["npm", "pnpm", "yarn", "bun"]
const plainWord = (word: string | undefined): word is string => word !== undefined && /^[\w:.-]+$/.test(word) && !word.startsWith("-")

// The start of a command that an "always" answer is remembered by: `git push`
// for `git push origin dev`, `ls` for `ls -la`, and three words where two
// would cover too much: `gh pr create`, `npm run build`. Undefined when the
// command is not written plainly enough to name a start for, in which case
// it is remembered whole.
export function startOf(part: string): string | undefined {
  const { words, altered, byPath } = plainWords(part)
  const [first, second, third] = words
  if (altered || byPath || first === undefined || !/^[\w.-]+$/.test(first) || !part.trimStart().startsWith(first)) return undefined
  if (!TWO_WORDS.includes(first)) return first
  if (!plainWord(second)) return undefined
  if (first === "gh" || (second === "run" && RUNS_SCRIPTS.includes(first))) return plainWord(third) ? `${first} ${second} ${third}` : undefined
  return `${first} ${second}`
}

// Commands that only look, and for some, the options that make them do more:
// write a file, change a setting, or run another program.
const READ_ONLY: Record<string, RegExp | undefined> = {
  ls: undefined, cat: undefined, head: undefined, tail: undefined, wc: undefined, pwd: undefined, echo: undefined,
  cd: undefined, which: undefined, whoami: undefined, tree: /^-[a-zA-Z]*o/, file: undefined, stat: undefined,
  du: undefined, df: undefined, date: /^(-[a-zA-Z]*s|--set)/, rg: /^--pre/, grep: undefined,
  sort: /^(-[a-zA-Z]*o|--output)/, uniq: undefined, cut: undefined, diff: undefined, basename: undefined,
  dirname: undefined, realpath: undefined, true: undefined,
  "git status": /^--output/, "git diff": /^(--output|--ext-diff)/, "git log": /^(--output|--ext-diff)/,
  "git show": /^(--output|--ext-diff)/, "git rev-parse": undefined, "git ls-files": undefined, "git blame": undefined,
  "git describe": undefined,
}

// True for a command that only looks and changes nothing. Writing its output
// to a file, running another command inside it, or setting a variable for it
// makes it more than that. `find` and `git branch` are left out, because
// they can also delete.
export function isReadOnly(part: string): boolean {
  // Sending output nowhere, or errors to where the output goes, writes no file.
  const bare = setQuotesAside(part).text.replace(/\d*>&\d+/g, " ").replace(/(\d*|&)>\s*\/dev\/null(?=\s|$)/g, " ")
  if (/[<>]|\$\(|`/.test(bare)) return false
  const { words, altered, byPath } = plainWords(bare)
  // Nothing but `NAME=value`: a variable is set and no command is run.
  if (words.length === 0) return part.trim().split(/\s+/).every(isAssignment)
  if (altered || byPath) return false
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

// True for a command that changes folder to nowhere in particular: `cd`
// alone goes to the home folder, and `cd -` to wherever was last.
export function leavesForHome(part: string): boolean {
  const { words } = plainWords(part)
  return (words[0] === "cd" || words[0] === "pushd") && (words.length === 1 || words[1] === "-" || words[1] === "~")
}

// Commands that reach outside the machine: they send, publish or deploy. In
// auto mode these are asked about.
const OUTWARD: RegExp[] = [
  /^git (push|send-email)\b/,
  /^(npm|pnpm|yarn|bun|cargo) publish\b/,
  /^(twine upload|gem push)\b/,
  // gh and glab change things on the forge unless they are only looking.
  /^(gh|glab) (?!(pr|mr|issue|repo|run|release|workflow) (view|list|status|diff|checks)\b)(?!(auth status|status)\b)/,
  /^curl\b.* (-[a-zA-Z]*[dFTX]\S*|--(request|data\S*|form\S*|upload-file|json)\b)/,
  /^wget\b.* --(post-data|post-file|method|body-data|body-file)\b/,
  /^(http|https|xh)\b.* (POST|PUT|PATCH|DELETE)\b/,
  /^(ssh|scp|sftp|rsync|ftp|nc|ncat|netcat|telnet)\b/,
  /^docker\b.*( push\b| login\b| --push\b)/,
  /^kubectl (apply|delete|create|replace|patch|scale|rollout|exec)\b/,
  /^helm (install|upgrade|uninstall|rollback|push)\b/,
  /^(terraform|tofu) (apply|destroy)\b/,
  /^pulumi (up|destroy)\b/,
  /^(vercel|netlify|fly|flyctl|heroku|wrangler|firebase|aws|gcloud|az|supabase)\b/,
  /^(mail|sendmail|mutt)\b/,
]
// A tool run through a package runner is that tool.
const RUNNER = /^(npx|bunx|pnpx|(pnpm|yarn) dlx|bun x|(npm|pnpm) exec)( -\S+)* /

export function isOutwardFacing(part: string): boolean {
  const plain = plainWords(part).words.join(" ").replace(RUNNER, "")
  return OUTWARD.some((pattern) => pattern.test(plain))
}

// Every piece of a command that could name a place on disk: its words, the
// values of `--flag=value` and `NAME=value`, a value written straight after
// a short flag as in `-C../..`, the targets of redirects, and the entries of
// `{a,b}`. Addresses such as https://… are left out.
export function placesIn(part: string): string[] {
  const places: string[] = []
  for (const piece of part.replace(/["']/g, "").split(/[\s<>={},;()|&]+/)) {
    if (piece === "" || /^[a-z][a-z0-9+.-]*:\/\//i.test(piece)) continue
    if (!piece.startsWith("-")) places.push(piece)
    else if (!piece.startsWith("--") && piece.length > 2) places.push(piece.slice(2))
  }
  return places
}
