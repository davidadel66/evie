// What the permissions plugin can tell about a shell command from its text.
//
// This is matching on text, the same as the guard: it recognises commands
// written plainly and can be fooled by one hidden in a script or built from
// a variable. It makes the common case quiet and the obvious risk visible.
// It is not a sandbox.

// Sets quoted text aside while a command is taken apart, so that `rg "a|b"`
// is one command and not two. Only text the shell would not run is set
// aside: single quotes, and double quotes with no `$` or backtick in them.
//
// `unquote` puts it back with the quotes taken off, as the shell would read
// it, and takes off double quotes that hold a `$` too. A quote left open
// stays, so the word cannot be read as a place.
function setQuotesAside(command: string): { text: string; putBack(text: string): string; unquote(text: string): string } {
  const quoted: string[] = []
  const text = command.replace(/\\\n/g, " ").replace(/'[^']*'|"[^"$`\\]*"/g, (found) => {
    quoted.push(found)
    return `\u0000${quoted.length - 1}\u0000`
  })
  const back = (piece: string, bare: boolean) => piece.replace(/\u0000(\d+)\u0000/g, (_all, index: string) => (bare ? quoted[Number(index)]!.slice(1, -1) : quoted[Number(index)]!))
  return { text, putBack: (piece) => back(piece, false), unquote: (piece) => back(piece.replace(/"([^"'`\\]*)"/g, "$1"), true) }
}

// The commands in a command line: the pieces between `&&`, `||`, `;`, `|`,
// `&` and new lines, and whatever is run inside `$(…)` or backticks. Each is
// judged on its own, so `git status && git push` is not let through as
// `git status`. An `&` that is part of a redirect, as in `2>&1`, does not split.
export function partsOf(command: string): string[] {
  const { text, putBack } = setQuotesAside(withoutHeredocs(command))
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
//   `gitIn`    the folders git was told to work in, by `-C` or `--work-tree`
export function plainWords(part: string): { words: string[]; altered: boolean; byPath: boolean; gitIn: string[] } {
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
  const gitIn: string[] = []
  if (words[0] === "git") {
    // `git -C folder push` and `git -c name=value push` are still a push.
    let at = 1
    while (words[at]?.startsWith("-")) {
      const option = words[at]!
      if (!["--no-pager", "-P"].includes(option)) altered = true
      if ((option === "-C" || option === "--work-tree") && words[at + 1] !== undefined) gitIn.push(words[at + 1]!)
      else if (option.startsWith("--work-tree=")) gitIn.push(option.slice("--work-tree=".length))
      at += GIT_TAKES_VALUE.includes(option) ? 2 : 1
    }
    words = ["git", ...words.slice(at)]
  }
  return { words: words.map((word) => word.replace(/[)}]+$/, "")).filter((word) => word !== ""), altered, byPath, gitIn }
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

// Where a `cd` or `pushd` goes, as written with its quotes taken off: "~"
// for the home folder and "-" for wherever it was last. Undefined for any
// other command.
export function cdTarget(part: string): string | undefined {
  const { text, unquote } = setQuotesAside(part)
  const { words } = plainWords(text)
  if (words[0] !== "cd" && words[0] !== "pushd") return undefined
  const to = words.slice(1).find((word) => word === "-" || !word.startsWith("-"))
  return to === undefined ? "~" : unquote(to)
}

// Every piece of a command that could name a place on disk: its words, the
// values of `--flag=value` and `NAME=value`, a value written straight after
// a short flag as in `-C../..`, the targets of redirects, and the entries of
// `{a,b}`. Quoted text is one word, as the shell reads it, so a message such
// as `-m "fix /etc/hosts"` names no place. Addresses such as https://… are
// left out, and so is the program run, unless `withProgram` is set: running
// /usr/bin/env is not working in /usr/bin.
export function placesIn(part: string, withProgram = true): string[] {
  const { text, unquote } = setQuotesAside(part)
  const program = withProgram ? undefined : text.split(/\s+/).find((word) => word !== "" && !isAssignment(word) && !/^[({!]+$/.test(word))
  const places: string[] = []
  for (const piece of text.split(/[\s<>={},;()|&]+/)) {
    if (piece === "" || piece === program) continue
    const word = unquote(piece).replace(/["']/g, "")
    if (word === "" || /^[a-z][a-z0-9+.-]*:\/\//i.test(word)) continue
    if (!word.startsWith("-")) places.push(word)
    else if (!word.startsWith("--") && word.length > 2) places.push(word.slice(2))
  }
  return places
}

// ---- Heredocs -----------------------------------------------------------------------------
//
// Reading a heredoc's end line is adapted from OpenCode's shell scanner,
// packages/core/src/shell/scan.ts (bashHeredocDelimiter and bashHeredoc).
//
// MIT License
//
// Copyright (c) 2025 opencode
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// The end word of a heredoc, written from `start`, just after `<<` or
// `<<-`: `EOF`, `'EOF'`, `"EOF"` or `\EOF`, with its quotes taken off, and
// where it ends in the text. Undefined when there is none.
function heredocEnd(text: string, start: number): { word: string; end: number } | undefined {
  let word = ""
  let quote: "'" | '"' | undefined
  let at = start
  while (at < text.length && /[ \t]/.test(text[at]!)) at += 1
  for (; at < text.length; at += 1) {
    const char = text[at]!
    if (quote !== undefined) {
      if (char === quote) quote = undefined
      else if (char === "\\" && quote === '"' && '$`"\\'.includes(text[at + 1] ?? "")) word += text[++at]
      else word += char
    } else if (/[\s;&|()<>]/.test(char)) break
    else if (char === "'" || char === '"') quote = char
    else if (char === "\\" && text[at + 1] !== undefined && text[at + 1] !== "\n") word += text[++at]
    else word += char
  }
  return word === "" || quote !== undefined ? undefined : { word, end: at }
}

// A command line with the text of each heredoc taken out: the lines after
// the one that starts it, up to and with its end line. That text is data
// given to a command, never commands, so nothing in it is judged as one. A
// heredoc with no end line runs to the end, as the shell reads it. A heredoc
// inside `"$(…)"`, as a commit message is often written, counts too.
//
// `keep` is given each line that starts heredocs, and keeps their text when
// it says so, as the guard does for a heredoc a shell reads as commands.
export function withoutHeredocs(command: string, keep: (line: string) => boolean = () => false): string {
  if (!command.includes("<<")) return command
  let out = ""
  const waiting: { word: string; tabs: boolean }[] = []
  // What is open where the text has got to: quotes, and `$(` inside double quotes or out of them.
  const open: ("'" | '"' | "$(")[] = []
  for (let at = 0; at < command.length; at += 1) {
    const char = command[at]!
    const inside = open.at(-1)
    if (inside === "'") {
      if (char === "'") open.pop()
    } else if (char === "\\") {
      out += command.slice(at, at + 2)
      at += 1
      continue
    } else if (command.startsWith("$(", at)) {
      open.push("$(")
      out += "$("
      at += 1
      continue
    } else if (inside === '"') {
      if (char === '"') open.pop()
    } else if (char === "'" || char === '"') {
      open.push(char)
    } else if (char === ")" && inside === "$(") {
      open.pop()
    } else if (char === "#" && (at === 0 || /[\s;&|()]/.test(command[at - 1]!))) {
      // A comment runs to the end of its line.
      const end = command.indexOf("\n", at)
      out += command.slice(at, end === -1 ? command.length : end)
      at = (end === -1 ? command.length : end) - 1
      continue
    } else if (command.startsWith("<<<", at)) {
      // A here-string: its text is on the same line, and is a word like any other.
      out += "<<<"
      at += 2
      continue
    } else if (command.startsWith("<<", at)) {
      const tabs = command[at + 2] === "-"
      const found = heredocEnd(command, at + (tabs ? 3 : 2))
      if (found !== undefined) {
        waiting.push({ word: found.word, tabs })
        out += command.slice(at, found.end)
        at = found.end - 1
        continue
      }
    } else if (char === "\n" && waiting.length > 0) {
      const heredocs = waiting.splice(0)
      if (!keep(out.slice(out.lastIndexOf("\n") + 1))) {
        // The bodies start on the next line, one after the other.
        let from = at + 1
        for (const heredoc of heredocs) {
          while (from < command.length) {
            const end = command.indexOf("\n", from)
            const line = command.slice(from, end === -1 ? command.length : end)
            from = end === -1 ? command.length : end + 1
            if ((heredoc.tabs ? line.replace(/^\t+/, "") : line) === heredoc.word) break
          }
        }
        out += "\n"
        at = from - 1
        continue
      }
    }
    out += char
  }
  return out
}

// Shells, and the programs that run what they are given as a shell does.
const SHELLS = ["bash", "sh", "zsh", "dash", "ksh", "fish", "eval", "source", "."]
const INTERPRETERS = /^(python[\d.]*|node|perl|ruby)$/
// Words that give an interpreter its program, so that it reads none from standard input.
const GIVES_PROGRAM = /^(-[a-zA-Z]*[ceEm]|--(eval|print|command)(=.*)?)$/

// True for a line of commands in which a shell, or an interpreter with no
// program of its own but standard input, runs what it is given: there, a
// heredoc is commands, not data.
export function runsInput(line: string): boolean {
  const { text } = setQuotesAside(line)
  return text.split(/&&|\|\||;|\|&?|&|\$\(|`|\(/).some((piece) => {
    const { words } = plainWords(piece.replace(/\d*<<-?\s*\S+/g, " ").replace(/\d*[<>]+&?\s*\S+/g, " "))
    const [name = "", ...rest] = words
    if (SHELLS.includes(name)) return true
    return INTERPRETERS.test(name) && !rest.some((word) => word !== "-" && (!word.startsWith("-") || GIVES_PROGRAM.test(word)))
  })
}
