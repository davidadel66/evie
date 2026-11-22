import { expect, test } from "bun:test"
import type { EventBodies, Job, StoredEvent, StoredEventType } from "../src/core/types.ts"
import { transcriptOf, workedFor, type Item, type View } from "../web/src/transcript.ts"

// How a conversation reads in the web app, apart from its screen: the
// session's stored events, the reply being written and its jobs, turned
// into what the page draws, in the order it happened: the owner's messages,
// evie's replies, each run of tool calls as one line of steps, and notes in
// quiet words.

// A session's events, numbered from 1, each a second after the one before
// unless it says when.
type Made = [StoredEventType, unknown, number | undefined]
function session(...made: Made[]): StoredEvent[] {
  let time = 1_000_000
  return made.map(([type, body, at], index) => {
    time = at ?? time + 1000
    return { type, session: "s1", seq: index + 1, time, body } as StoredEvent
  })
}
const event = <Type extends StoredEventType>(type: Type, body: EventBodies[Type], at?: number): Made => [type, body, at]

const view = (events: StoredEvent[], more: Partial<View> = {}): View => ({ events, replySoFar: "", turnRunning: false, jobs: [], earlier: false, ...more })

// Each item as its kind and what it says, to compare in a line.
function read(items: Item[]): string[] {
  return items.map((item) => {
    if (item.kind === "owner" || item.kind === "reply") return `${item.kind} ${item.turn}: ${item.text}${item.kind === "reply" && item.streaming ? " (streaming)" : ""}`
    if (item.kind === "steps") return `steps ${item.turn}: ${item.label} [${item.calls.map((call) => call.name).join(", ")}]`
    if (item.kind === "question") return `question ${item.turn}: ${item.text}${item.waiting ? ` (waiting: ${item.choices.join(" / ")})` : ""}`
    if (item.kind === "failed") return `failed ${item.turn}: ${item.error}`
    if (item.kind === "fold") return `fold ${item.turn}: ${item.label} (${read(item.items).join(" | ")})`
    return `note ${item.turn}: ${item.text}`
  })
}
// Every item, with what a fold holds in its place.
const unfolded = (items: Item[]) => items.flatMap((item) => (item.kind === "fold" ? item.items : [item]))

test("the owner's messages and evie's replies are read in order, with the turn each belongs to", () => {
  const events = session(
    event("turn_start", {}),
    event("user_message", { text: "What is a simple way to keep a reading list?" }),
    event("assistant_message", { text: "A plain text file, newest first." }),
    event("turn_end", { reason: "completed" }),
    event("turn_start", {}),
    event("user_message", { text: "In one sentence?" }),
    event("assistant_message", { text: "Keep one file, newest first." }),
    event("turn_end", { reason: "completed" }),
  )

  expect(read(transcriptOf(view(events)))).toEqual([
    "owner 1: What is a simple way to keep a reading list?",
    "reply 1: A plain text file, newest first.",
    "owner 2: In one sentence?",
    "reply 2: Keep one file, newest first.",
  ])
})

// The seeded session that fixes a README, with the times it took, and two
// more calls in its second run.
const fixingTheReadme = () =>
  session(
    event("turn_start", {}, 1_000_000),
    event("user_message", { text: "The README gives the wrong command for running the tests. Can you fix it?" }, 1_000_000),
    event("assistant_message", { text: "I'll read the README first.", toolCalls: [{ id: "c1", name: "read", arguments: '{"path":"README.md"}' }] }, 1_002_000),
    event("tool_result", { callId: "c1", name: "read", text: "\n1: # Recipe box\n2: Run the tests with `npm test`." }, 1_003_000),
    event("assistant_message", { text: "", toolCalls: [{ id: "c2", name: "edit", arguments: '{"path":"README.md","oldString":"`npm test`","newString":"`bun test`"}' }] }, 1_010_000),
    event("tool_result", { callId: "c2", name: "edit", text: "Edited README.md (1 replacement)." }, 1_011_000),
    event(
      "assistant_message",
      {
        text: "The README says `npm test`, but package.json runs the tests with Bun.",
        toolCalls: [
          { id: "c3", name: "shell", arguments: '{"command":"git diff\\n  --stat"}' },
          { id: "c4", name: "read", arguments: '{"path":"package.json"}' },
        ],
      },
      1_020_000,
    ),
    event("tool_result", { callId: "c3", name: "shell", text: " README.md | 2 +-" }, 1_030_000),
    event("tool_result", { callId: "c4", name: "read", text: `${"x".repeat(120)}\nmore` }, 1_031_000),
    event("assistant_message", { text: "", toolCalls: [{ id: "c5", name: "shell", arguments: '{"command":"bun test"}' }] }, 1_040_000),
    event("tool_result", { callId: "c5", name: "shell", text: "" }, 1_046_000),
    event("assistant_message", { text: "Fixed: the README now says `bun test`." }, 1_047_000),
    event("turn_end", { reason: "completed" }, 1_048_000),
  )

test("a turn that ended as completed is folded before its final reply into one line saying how long it worked, holding evie's words and each run of calls in the order they came", () => {
  const items = transcriptOf(view(fixingTheReadme()))

  expect(read(items)).toEqual([
    "owner 1: The README gives the wrong command for running the tests. Can you fix it?",
    "fold 1: Worked for 48s (" +
      "reply 1: I'll read the README first. | " +
      "steps 1: Used read and edit [read, edit] | " +
      "reply 1: The README says `npm test`, but package.json runs the tests with Bun. | " +
      "steps 1: Used shell and read [shell, read, shell])",
    "reply 1: Fixed: the README now says `bun test`.",
  ])
})

test("a turn that was stopped, failed, or had a reply cut short is not folded, and its first run says how long it worked", () => {
  const ending = (how: [StoredEventType, unknown, number | undefined][]) =>
    session(
      event("turn_start", {}, 1_000_000),
      event("user_message", { text: "Check the tests" }, 1_000_000),
      event("assistant_message", { text: "Running them.", toolCalls: [{ id: "c1", name: "shell", arguments: '{"command":"bun test"}' }] }, 1_001_000),
      event("tool_result", { callId: "c1", name: "shell", text: "12 pass" }, 1_005_000),
      ...how,
    )
  const lines = (events: StoredEvent[]) => read(transcriptOf(view(events)))

  expect(lines(ending([event("turn_end", { reason: "cancelled" }, 1_010_000)]))).toEqual([
    "owner 1: Check the tests",
    "reply 1: Running them.",
    "steps 1: Worked for 10s [shell]",
    "note 1: Stopped.",
  ])
  expect(lines(ending([event("turn_end", { reason: "error", error: "OpenRouter answered 502." }, 1_010_000)])).slice(2)).toEqual(["steps 1: Worked for 10s [shell]", "failed 1: OpenRouter answered 502."])
  expect(
    lines(
      ending([
        event("assistant_message", { text: "All 12", interrupted: true }, 1_006_000),
        event("user_message", { text: "Shorter" }, 1_007_000),
        event("assistant_message", { text: "They pass." }, 1_008_000),
        event("turn_end", { reason: "completed" }, 1_010_000),
      ]),
    ).slice(2),
  ).toEqual(["steps 1: Worked for 10s [shell]", "reply 1: All 12", "note 1: The reply was cut short.", "owner 1: Shorter", "reply 1: They pass."])
})

test("each call in the steps is its name, a short form of its input on one line, its result as stored, and that result in short: its first line, cut short", () => {
  const calls = unfolded(transcriptOf(view(fixingTheReadme()))).flatMap((item) => (item.kind === "steps" ? item.calls : []))

  expect(calls).toEqual([
    { id: "c1", name: "read", input: "README.md", result: "\n1: # Recipe box\n2: Run the tests with `npm test`.", short: "1: # Recipe box", running: false },
    { id: "c2", name: "edit", input: "README.md", result: "Edited README.md (1 replacement).", short: "Edited README.md (1 replacement).", running: false },
    { id: "c3", name: "shell", input: "git diff --stat", result: " README.md | 2 +-", short: "README.md | 2 +-", running: false },
    { id: "c4", name: "read", input: "package.json", result: `${"x".repeat(120)}\nmore`, short: `${"x".repeat(80)}…`, running: false },
    { id: "c5", name: "shell", input: "bun test", result: "", short: "", running: false },
  ])
})

test("a fold, and a line of steps in it, keep their keys when the page before is read, even when the turn began on that page; without its start, the fold names the tools", () => {
  const key = (items: Item[], name: string) => unfolded(items).findLast((item) => item.kind === "steps" && item.calls.some((call) => call.name === name))?.key
  const fold = (items: Item[]) => items.find((item) => item.kind === "fold")

  const whole = transcriptOf(view(fixingTheReadme()))
  const fromTheMiddle = transcriptOf(view(fixingTheReadme().slice(8), { earlier: true }))

  expect(key(fromTheMiddle, "shell")).toBeDefined()
  expect(key(fromTheMiddle, "shell")).toBe(key(whole, "shell"))
  expect(fold(fromTheMiddle)?.key).toBeDefined()
  expect(fold(fromTheMiddle)?.key).toBe(fold(whole)?.key)
  expect(read(fromTheMiddle)[0]).toBe("fold 0: Used read and shell (steps 0: Used read and shell [read, shell])")
})

test("while a turn runs, the run being worked on says Working, the call with no result yet is the one running, and the reply being written comes last", () => {
  const callsOf = (items: Item[]) => items.flatMap((item) => (item.kind === "steps" ? item.calls.map((call) => [call.name, call.running]) : []))

  const calling = transcriptOf(view(fixingTheReadme().slice(0, 8), { turnRunning: true }))
  expect(read(calling).slice(1)).toEqual([
    "reply 1: I'll read the README first.",
    "steps 1: Used read and edit [read, edit]",
    "reply 1: The README says `npm test`, but package.json runs the tests with Bun.",
    "steps 1: Working [shell, read]",
  ])
  expect(callsOf(calling)).toEqual([
    ["read", false],
    ["edit", false],
    ["shell", false],
    ["read", true],
  ])

  const writing = transcriptOf(view(fixingTheReadme().slice(0, 11), { turnRunning: true, replySoFar: "Fixed: the REA" }))
  expect(read(writing).slice(-2)).toEqual(["steps 1: Used shell and read [shell, read, shell]", "reply 1: Fixed: the REA (streaming)"])
  expect(callsOf(writing).every(([, running]) => running === false)).toBe(true)
})

test("a running turn that is neither writing nor carrying out a call says it is working at the end, and one that is writing shows only what it writes", () => {
  const asked = session(event("turn_start", {}), event("user_message", { text: "Tell me a joke" }))

  expect(read(transcriptOf(view(asked, { turnRunning: true })))).toEqual(["owner 1: Tell me a joke", "steps 1: Working []"])
  expect(read(transcriptOf(view(asked, { turnRunning: true, replySoFar: "Why did" })))).toEqual(["owner 1: Tell me a joke", "reply 1: Why did (streaming)"])
  expect(read(transcriptOf(view(asked, { turnRunning: false })))).toEqual(["owner 1: Tell me a joke"])
  // Between a call's result and evie's next words, it is still working.
  expect(read(transcriptOf(view(fixingTheReadme().slice(0, 4), { turnRunning: true }))).slice(-2)).toEqual(["steps 1: Used read [read]", "steps 1: Working []"])
})

test("a reply cut short, a turn stopped or failed, a compaction and a note to evie are each said in quiet words, as the terminal says them, with more to open", () => {
  const events = session(
    event("turn_start", {}),
    event("user_message", { text: "Walk me through src/readings.ts." }),
    event("assistant_message", { text: "It adds up every rea", interrupted: true }),
    event("turn_end", { reason: "cancelled" }),
    event("turn_start", {}),
    event("user_message", { text: "Go on." }),
    event("assistant_message", { text: "It divides by", interrupted: true }),
    event("user_message", { text: "Shorter, please." }),
    event("assistant_message", { text: "It is a plain mean." }),
    event("turn_end", { reason: "completed" }),
    event("compaction", { summary: "## Goal\n- Understand `average`.", keepFrom: 9, files: ["src/readings.ts"] }),
    event("turn_start", {}),
    event("note", { text: "Background job 1 (sh scripts/slow-tests.sh) has finished.\n\n12 tests passed" }),
    event("assistant_message", { text: "All 12 tests passed." }),
    event("turn_end", { reason: "completed" }),
    event("turn_start", {}),
    event("user_message", { text: "Again?" }),
    event("turn_end", { reason: "error", error: "OpenRouter answered 502: the model is overloaded." }),
    event("turn_start", {}),
    event("user_message", { text: "And once more?" }),
    event("turn_end", { reason: "cancelled" }),
  )

  const items = transcriptOf(view(events))

  expect(read(items)).toEqual([
    "owner 1: Walk me through src/readings.ts.",
    "reply 1: It adds up every rea",
    "note 1: Stopped. The reply was cut short.",
    "owner 2: Go on.",
    "reply 2: It divides by",
    "note 2: The reply was cut short.",
    "owner 2: Shorter, please.",
    "reply 2: It is a plain mean.",
    "note 2: The conversation was getting long, so its earlier part is now a summary.",
    "note 3: Note to evie: Background job 1 (sh scripts/slow-tests.sh) has finished.",
    "reply 3: All 12 tests passed.",
    "owner 4: Again?",
    "failed 4: OpenRouter answered 502: the model is overloaded.",
    "owner 5: And once more?",
    "note 5: Stopped.",
  ])
  const more = items.flatMap((item) => (item.kind === "note" && item.more !== undefined ? [item.more] : []))
  expect(more).toEqual(["## Goal\n- Understand `average`.", "Background job 1 (sh scripts/slow-tests.sh) has finished.\n\n12 tests passed"])
})

// The seeded session that asks the owner a question, answered a minute later.
const askingFirst = (choice: string | null) =>
  session(
    event("turn_start", {}, 1_000_000),
    event("user_message", { text: "Add a way to mark a plant as watered." }, 1_000_000),
    event("assistant_message", { text: "One thing to settle first.", toolCalls: [{ id: "c1", name: "ask", arguments: '{"question":"Record the time too?"}' }] }, 1_001_000),
    event("question", { id: "q1", text: "Should watering record the time as well as the day?", choices: ["Just the day", "The day and the time"] }, 1_002_000),
    event("answer", { id: "q1", choice }, 1_062_000),
    event("tool_result", { callId: "c1", name: "ask", text: `The owner chose: ${choice}` }, 1_062_000),
    event("assistant_message", { text: "", toolCalls: [{ id: "c2", name: "edit", arguments: '{"path":"src/plants.ts"}' }] }, 1_063_000),
    event("tool_result", { callId: "c2", name: "edit", text: "Edited src/plants.ts (1 replacement)." }, 1_064_000),
    event("assistant_message", { text: "Done: only the day is kept." }, 1_065_000),
    event("turn_end", { reason: "completed" }, 1_066_000),
  )

test("a question shows its choices while it waits; answered, the answer is the owner's message, and the minute spent waiting is not counted as work", () => {
  expect(read(transcriptOf(view(askingFirst(null).slice(0, 4), { turnRunning: true })))).toEqual([
    "owner 1: Add a way to mark a plant as watered.",
    "reply 1: One thing to settle first.",
    "steps 1: Working [ask]",
    "question 1: Should watering record the time as well as the day? (waiting: Just the day / The day and the time)",
  ])

  // Folded once the turn has ended, the question and its answer stay outside, after the fold's line.
  expect(read(transcriptOf(view(askingFirst("Just the day"))))).toEqual([
    "owner 1: Add a way to mark a plant as watered.",
    "fold 1: Worked for 6s (reply 1: One thing to settle first. | steps 1: Used ask [ask] | steps 1: Used edit [edit])",
    "question 1: Should watering record the time as well as the day?",
    "owner 1: Just the day",
    "reply 1: Done: only the day is kept.",
  ])

  expect(read(transcriptOf(view(askingFirst(null)))).slice(2, 4)).toEqual(["question 1: Should watering record the time as well as the day?", "note 1: Not answered."])
})

// The seeded session that runs the slow tests in the background, and is
// woken when they finish.
const inTheBackground = () =>
  session(
    event("turn_start", {}, 1_000_000),
    event("user_message", { text: "Run the slow tests in the background." }, 1_000_000),
    event("assistant_message", { text: "", toolCalls: [{ id: "c1", name: "shell", arguments: '{"command":"sh scripts/slow-tests.sh","background":true}' }] }, 1_001_000),
    event("tool_result", { callId: "c1", name: "shell", text: "The command was started in the background. It is background job 1." }, 1_004_000),
    event("assistant_message", { text: "They are running in the background." }, 1_005_000),
    event("turn_end", { reason: "completed" }, 1_006_000),
    event("turn_start", {}, 1_020_000),
    event("note", { text: "Background job 1 (sh scripts/slow-tests.sh) has finished." }, 1_020_000),
    event("assistant_message", { text: "All 12 passed." }, 1_022_000),
    event("turn_end", { reason: "completed" }, 1_023_000),
  )
const job = (status: Job["status"], ended?: number): Job => ({
  id: "1",
  session: "s1",
  kind: "shell",
  title: "sh scripts/slow-tests.sh",
  status,
  started: 1_003_000,
  ...(ended !== undefined && { ended }),
  outputFile: "/tmp/1.out",
})

test("a background job's start and end are said where they happened, as the terminal says them", () => {
  expect(read(transcriptOf(view(inTheBackground(), { jobs: [job("finished", 1_019_000)] })))).toEqual([
    "owner 1: Run the slow tests in the background.",
    "fold 1: Worked for 6s (steps 1: Used shell [shell])",
    "note 1: Background job 1 started: sh scripts/slow-tests.sh",
    "reply 1: They are running in the background.",
    "note 1: Background job 1 finished after 16s: sh scripts/slow-tests.sh",
    "note 2: Note to evie: Background job 1 (sh scripts/slow-tests.sh) has finished.",
    "reply 2: All 12 passed.",
  ])

  const said = (status: Job["status"], ended?: number) => read(transcriptOf(view(inTheBackground().slice(0, 6), { jobs: [job(status, ended)] }))).slice(2).filter((line) => line.includes("job"))
  expect(said("running")).toEqual(["note 1: Background job 1 started: sh scripts/slow-tests.sh"])
  expect(said("failed", 1_008_000).at(-1)).toBe("note 1: Background job 1 failed after 5s: sh scripts/slow-tests.sh")
  expect(said("stopped", 1_008_000).at(-1)).toBe("note 1: Background job 1 stopped after 5s: sh scripts/slow-tests.sh")
  expect(said("unknown", 1_008_000).at(-1)).toBe("note 1: Background job 1 ended (how is not known) after 5s: sh scripts/slow-tests.sh")
})

test("what a command run from the page said, which is not stored, is a quiet note where it was said", () => {
  const notes = [{ key: "said1", time: 1_010_000, text: "Removed, because the turn created it: notes.md." }]
  expect(read(transcriptOf(view(inTheBackground(), { jobs: [job("finished", 1_019_000)], notes }))).slice(2, 5)).toEqual([
    "note 1: Background job 1 started: sh scripts/slow-tests.sh",
    "reply 1: They are running in the background.",
    "note 1: Removed, because the turn created it: notes.md.",
  ])
})

test("a page that begins part way through a turn shows the calls whose results it has, but not how long the turn worked, and a job from before the page is left for the page before", () => {
  const events = inTheBackground().slice(3)

  expect(read(transcriptOf(view(events, { earlier: true, jobs: [job("finished", 1_019_000)] })))).toEqual([
    "fold 0: Used shell (steps 0: Used shell [shell])",
    "reply 0: They are running in the background.",
    "note 0: Background job 1 finished after 16s: sh scripts/slow-tests.sh",
    "note 1: Note to evie: Background job 1 (sh scripts/slow-tests.sh) has finished.",
    "reply 1: All 12 passed.",
  ])
})

test("how long a turn worked is said in seconds, then minutes and seconds, then hours and minutes", () => {
  expect([workedFor(1), workedFor(48), workedFor(60), workedFor(125), workedFor(3599)]).toEqual(["Worked for 1s", "Worked for 48s", "Worked for 1m", "Worked for 2m 5s", "Worked for 59m 59s"])
  expect([workedFor(3600), workedFor(3780), workedFor(7265)]).toEqual(["Worked for 1h", "Worked for 1h 3m", "Worked for 2h 1m"])
})
