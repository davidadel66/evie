import { expect, test } from "bun:test"
import { createLiveInput } from "../src/index.ts"
import { fakeScreen } from "./support/fake-screen.ts"

function open(columns = 60) {
  const screen = fakeScreen(columns)
  const input = createLiveInput(screen)
  const sent: string[] = []
  const reading = (async () => {
    for await (const line of input.lines) sent.push(line)
  })()
  return { screen, input, sent, reading }
}

test("typing while output is streaming does not mix the two", async () => {
  const { screen, input } = open()

  input.write("Hello wor")
  input.key("h")
  input.key("i")
  input.write("ld\nand a second line")
  input.key("!")

  expect(screen.shown()).toEqual(["Hello world", "and a second line", "› hi!"])
})

test("Enter sends the line, shows it above, and leaves a fresh input line", async () => {
  const { screen, input, sent } = open()

  input.write("An answer in pro")
  input.key("stop there\r")
  await Promise.resolve()
  input.write("New answer")

  expect(sent).toEqual(["stop there"])
  expect(screen.shown()).toEqual(["An answer in pro", "› stop there", "New answer", "›"])
})

test("Backspace removes the last character, and Ctrl-U clears the line", async () => {
  const { screen, input } = open()

  input.key("helo")
  input.key("\x7f")
  input.key("lo")
  expect(screen.shown()).toEqual(["› hello"])

  input.key("\x15")
  input.key("new")
  expect(screen.shown()).toEqual(["› new"])
})

test("an output line longer than the window wraps without leaving anything behind", async () => {
  const { screen, input } = open(20)

  input.write("a".repeat(45))
  input.key("typed")
  input.write("b".repeat(10))
  input.write("\nshort")

  expect(screen.shown()).toEqual(["a".repeat(20), "a".repeat(20), "aaaaa" + "b".repeat(10), "short", "› typed"])
})

test("a typed line longer than the window wraps, and output still lands above it", async () => {
  const { screen, input } = open(20)

  input.key("x".repeat(30))
  input.write("first\n")
  input.write("second\n")

  expect(screen.shown()).toEqual(["first", "second", "› " + "x".repeat(18), "x".repeat(12)])
})

test("arrow keys and other escape codes are ignored", async () => {
  const { screen, input } = open()

  input.key("ab")
  input.key("\x1b[A")
  input.key("\x1b[D")
  input.key("c")

  expect(screen.shown()).toEqual(["› abc"])
})

test("a blank line is not sent", async () => {
  const { input, sent } = open()

  input.key("   \r")
  input.key("\r")
  await Promise.resolve()

  expect(sent).toEqual([])
})

test("Ctrl-C calls the interrupt handler, and Ctrl-D on an empty line ends the input", async () => {
  const { input, sent, reading } = open()
  let interrupts = 0
  input.onInterrupt(() => (interrupts += 1))

  input.key("\x03")
  input.key("last\r")
  input.key("\x04")
  await reading

  expect(interrupts).toBe(1)
  expect(sent).toEqual(["last"])
})

test("several lines pasted at once are sent one after another", async () => {
  const { input, sent } = open()

  input.key("one\rtwo\r")
  await new Promise((resolve) => setTimeout(resolve, 1))

  expect(sent).toEqual(["one", "two"])
})

test("output that arrives after the input has ended is still printed", async () => {
  const { screen, input, reading } = open()

  input.write("A reply in pro")
  input.key("\x04")
  await reading
  input.write("gress\nand its last line\n")

  expect(screen.shown().filter((row) => row !== "")).toEqual(["A reply in pro", "gress", "and its last line"])
})
