// The input line for a real terminal. What the owner types stays on a line
// of its own at the bottom, and Evie's output is printed above it, so typing
// while Evie is printing does not mix the two.
//
// It works by keeping a small "live" area at the bottom of the screen: the
// unfinished line of output, if there is one, and under it the line being
// typed. Whenever either changes, the live area is wiped and drawn again.
// Finished lines of output are printed above it and never touched again.

export type Screen = {
  write(text: string): void
  // How many characters fit across the window.
  columns(): number
}

export type LiveInput = {
  // The lines the owner sends, one for each Enter.
  lines: AsyncIterable<string>
  // Prints Evie's output above the input line.
  write(text: string): void
  // Takes what was typed, as the terminal delivers it a key at a time.
  key(data: string): void
  // What the owner has typed and not yet sent.
  typing(): string
  // Sets what Ctrl-C does.
  onInterrupt(handler: () => void): void
  // Ends the input and leaves the screen tidy.
  close(): void
}

const PROMPT = "› "
// Every new line is written with a carriage return, because a terminal that
// delivers keys one at a time may no longer add one itself.
const NEW_LINE = "\r\n"

export function createLiveInput(screen: Screen): LiveInput {
  // The output line that has not ended yet.
  let unfinished = ""
  // What the owner has typed and not yet sent.
  let typed = ""
  // How many rows of the screen the live area takes up now.
  let drawnRows = 0
  let interrupt = () => {}
  let closed = false

  // The lines sent and not yet read, and whoever is waiting for the next one.
  const sent: string[] = []
  let waiting: ((next: IteratorResult<string>) => void) | undefined

  const rows = (text: string) => Math.max(1, Math.ceil(width(text) / Math.max(1, screen.columns())))

  // Wipes the live area. The cursor is on its last row, so it moves up to
  // the first and clears from there down.
  function wipe() {
    if (drawnRows === 0) return
    screen.write(`${drawnRows > 1 ? `\x1b[${drawnRows - 1}A` : ""}\r\x1b[J`)
    drawnRows = 0
  }

  function draw() {
    const input = PROMPT + typed
    screen.write(unfinished === "" ? input : `${unfinished}${NEW_LINE}${input}`)
    drawnRows = (unfinished === "" ? 0 : rows(unfinished)) + rows(input)
  }

  function send(line: string) {
    if (waiting) {
      waiting({ value: line, done: false })
      waiting = undefined
    } else sent.push(line)
  }

  function end() {
    if (closed) return
    closed = true
    wipe()
    if (unfinished !== "") screen.write(`${unfinished}${NEW_LINE}`)
    unfinished = ""
    waiting?.({ value: undefined, done: true })
    waiting = undefined
  }

  return {
    lines: {
      [Symbol.asyncIterator]() {
        return {
          next: () =>
            new Promise<IteratorResult<string>>((resolve) => {
              if (sent.length > 0) resolve({ value: sent.shift()!, done: false })
              else if (closed) resolve({ value: undefined, done: true })
              else waiting = resolve
            }),
        }
      },
    },

    write(text) {
      if (text === "") return
      // Once the input has ended there is no live area to keep, but output
      // still arriving, such as the rest of a reply, is printed as it comes.
      if (closed) return screen.write(text.replaceAll("\n", NEW_LINE))
      wipe()
      const lines = (unfinished + text).split("\n")
      unfinished = lines.pop()!
      // Finished lines go above the live area for good.
      if (lines.length > 0) screen.write(lines.map((line) => `${line}${NEW_LINE}`).join(""))
      draw()
    },

    key(data) {
      if (closed) return
      const keys = [...data]
      for (let at = 0; at < keys.length; at++) {
        const key = keys[at]!
        if (key === "\r" || key === "\n") {
          const line = typed
          typed = ""
          // A blank line is not sent.
          if (line.trim() === "") continue
          wipe()
          // The output so far is closed off, and the sent line shown under it.
          if (unfinished !== "") screen.write(`${unfinished}${NEW_LINE}`)
          unfinished = ""
          screen.write(`${PROMPT}${line}${NEW_LINE}`)
          draw()
          send(line)
        } else if (key === "\x7f" || key === "\b") typed = [...typed].slice(0, -1).join("")
        else if (key === "\x15") typed = ""
        else if (key === "\x03") interrupt()
        else if (key === "\x04") {
          if (typed === "") return end()
        } else if (key === "\x1b") {
          // An escape code, such as an arrow key: skip all of it.
          if (keys[at + 1] === "[" || keys[at + 1] === "O") {
            at += 2
            while (at < keys.length && !/[@-~]/.test(keys[at]!)) at += 1
          } else at += 1
        } else if (key >= " ") typed += key
        if (closed) return
      }
      wipe()
      draw()
    },

    typing() {
      return typed
    },

    onInterrupt(handler) {
      interrupt = handler
    },

    close: end,
  }
}

// How many columns a piece of text takes. Most characters take one; emoji
// and East Asian characters take two.
function width(text: string): number {
  let total = 0
  for (const character of text) {
    const code = character.codePointAt(0)!
    const wide =
      (code >= 0x1100 && code <= 0x115f) ||
      (code >= 0x2e80 && code <= 0xa4cf) ||
      (code >= 0xac00 && code <= 0xd7a3) ||
      (code >= 0xf900 && code <= 0xfaff) ||
      (code >= 0xfe30 && code <= 0xfe4f) ||
      (code >= 0xff00 && code <= 0xff60) ||
      (code >= 0x1f300 && code <= 0x1faff) ||
      (code >= 0x20000 && code <= 0x3fffd)
    total += wide ? 2 : 1
  }
  return total
}
