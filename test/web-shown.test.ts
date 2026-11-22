import { expect, test } from "bun:test"
import type { EventBodies, StoredEvent, StoredEventType, TurnChanges } from "../src/core/types.ts"
import { shownText, type ShownFile } from "../src/plugins/show/record.ts"
import { galleryOf, viewerOf } from "../web/src/shown.ts"
import { transcriptOf, type Item, type View } from "../web/src/transcript.ts"

// What evie showed, as the page draws it, apart from its screen: each file
// under evie's reply for its turn, in the order shown, outside the folded
// work; where the page fetches it from; and the images full screen moves
// through, every one shown in the session, in order.

type Made = [StoredEventType, unknown]
function session(...made: Made[]): StoredEvent[] {
  return made.map(([type, body], index) => ({ type, session: "s1", seq: index + 1, time: 1_000_000 + index * 1000, body }) as StoredEvent)
}
const event = <Type extends StoredEventType>(type: Type, body: EventBodies[Type]): Made => [type, body]
const view = (events: StoredEvent[], more: Partial<View> = {}): View => ({ events, replySoFar: "", turnRunning: false, jobs: [], earlier: false, project: "p1", ...more })

// A show call, and its result: its text for the model, and its details
// naming the files shown, which the page reads.
let calls = 0
function showing(...files: ShownFile[]): Made[] {
  const id = `show${++calls}`
  return [
    event("assistant_message", { text: "", toolCalls: [{ id, name: "show", arguments: JSON.stringify({ paths: files.map((file) => file.from ?? file.path) }) }] }),
    event("tool_result", { callId: id, name: "show", text: shownText(files), details: { files } }),
  ]
}
const chart: ShownFile = { storage: "project", path: "shown/2026-11-22/chart.png", type: "image/png", size: 33, from: "out/chart.png" }
const logo: ShownFile = { storage: "project", path: "shown/2026-11-22/logo.svg", type: "image/svg+xml", size: 82, from: "design/logo.svg" }
const card: ShownFile = { storage: "project", path: "shown/2026-11-22/card.pdf", type: "application/pdf", size: 57, from: "out/card.pdf" }
const guide: ShownFile = { storage: "project", path: "files/guide.md", type: "text/markdown", size: 29 }

// Each item as its kind and, for a file shown, its name.
const read = (items: Item[]): string[] => items.map((item) => (item.kind === "shown" ? `shown ${item.turn}: ${item.file.name}` : item.kind === "fold" ? `fold ${item.turn} (${read(item.items).join(", ")})` : `${item.kind} ${item.turn}`))

test("each file shown is drawn under evie's reply for its turn, in the order shown, outside the folded work, before the files the turn changed", () => {
  const events = session(
    event("turn_start", {}),
    event("user_message", { text: "Design a recipe card, with a logo and a guide" }),
    event("assistant_message", { text: "Drawing the card first." }),
    ...showing(chart),
    ...showing(logo, card),
    event("assistant_message", { text: "Here are all three." }),
    ...[event("turn_end", { reason: "completed" })],
  )
  const ended = events.at(-1)!.seq
  const changes: TurnChanges[] = [{ turn: 1, start: 1, end: ended, time: 0, files: [{ path: "design/card.html", status: "added", added: 40, removed: 0 }] }]

  expect(read(transcriptOf(view(events), changes))).toEqual(["owner 1", "fold 1 (reply 1, steps 1)", "reply 1", "shown 1: chart.png", "shown 1: logo.svg", "shown 1: card.pdf", "changes 1"])
})

test("while the turn runs, what it has shown so far is at its end; a turn stopped or failed keeps what it showed, before saying so", () => {
  const running = session(event("turn_start", {}), event("user_message", { text: "Draw it" }), ...showing(chart), event("assistant_message", { text: "", toolCalls: [{ id: "r1", name: "read", arguments: '{"path":"a.md"}' }] }))
  expect(read(transcriptOf(view(running, { turnRunning: true })))).toEqual(["owner 1", "steps 1", "shown 1: chart.png"])
  expect(read(transcriptOf(view(running, { turnRunning: true, replySoFar: "Here it" })))).toEqual(["owner 1", "steps 1", "reply 1", "shown 1: chart.png"])

  const stopped = session(event("turn_start", {}), event("user_message", { text: "Draw it" }), ...showing(chart), event("turn_end", { reason: "cancelled" }))
  expect(read(transcriptOf(view(stopped)))).toEqual(["owner 1", "steps 1", "shown 1: chart.png", "note 1"])
  const failed = session(event("turn_start", {}), event("user_message", { text: "Draw it" }), ...showing(chart), event("turn_end", { reason: "error", error: "The model could not be reached." }))
  expect(read(transcriptOf(view(failed)))).toEqual(["owner 1", "steps 1", "shown 1: chart.png", "failed 1"])
})

test("a show call that was refused shows nothing; a file is named by its name, its Storage folder, and fetched from Storage or the session's own files, all read from the call's details", () => {
  const events = session(
    event("turn_start", {}),
    event("user_message", { text: "Show me" }),
    event("assistant_message", { text: "", toolCalls: [{ id: "x", name: "show", arguments: '{"paths":["notes.txt"]}' }] }),
    event("tool_result", { callId: "x", name: "show", text: "Not shown: notes.txt is not a kind of file evie can show." }),
    // The text written for the model is never read for files: only the details are.
    event("assistant_message", { text: "", toolCalls: [{ id: "y", name: "show", arguments: '{"paths":["a.png"]}' }] }),
    event("tool_result", { callId: "y", name: "show", text: shownText([chart]) }),
    ...showing(guide, { storage: "session", path: "/data/shown/s1/2026-11-22/plot.png", type: "image/png", size: 40, from: "/tmp/plot.png" }),
    event("assistant_message", { text: "Here." }),
    event("turn_end", { reason: "completed" }),
  )
  const shown = transcriptOf(view(events)).flatMap((item) => (item.kind === "shown" ? [item.file] : []))

  expect(shown).toEqual([
    {
      key: expect.stringMatching(/^shown-show\d+-0$/),
      path: ".evie/files/guide.md",
      type: "text/markdown",
      name: "guide.md",
      folder: ".evie/files/",
      address: "/storage?project=p1&path=files%2Fguide.md",
      storage: { project: "p1", path: "files/guide.md" },
    },
    {
      key: expect.stringMatching(/^shown-show\d+-1$/),
      path: "/data/shown/s1/2026-11-22/plot.png",
      type: "image/png",
      name: "plot.png",
      folder: "/data/shown/s1/2026-11-22/",
      address: "/storage?session=s1&path=%2Fdata%2Fshown%2Fs1%2F2026-11-22%2Fplot.png",
    },
  ])
})

test("a file shown in Storage before the session moved is fetched from the project it was in then", () => {
  const events = session(
    event("turn_start", {}),
    event("user_message", { text: "Show me" }),
    ...showing(chart),
    event("turn_end", { reason: "completed" }),
    event("moved", { from: "p1", to: "p2" }),
    event("note", { text: "You now work in p2." }),
    event("turn_start", {}),
    event("user_message", { text: "And again" }),
    ...showing(chart),
    event("turn_end", { reason: "completed" }),
  )
  const addresses = transcriptOf(view(events, { project: "p2" })).flatMap((item) => (item.kind === "shown" ? [item.file.address] : []))

  expect(addresses).toEqual(["/storage?project=p1&path=shown%2F2026-11-22%2Fchart.png", "/storage?project=p2&path=shown%2F2026-11-22%2Fchart.png"])
})

test("full screen moves through every image shown in the session, pictures and SVG, in the order shown; a PDF and a document are not among them", () => {
  const events = session(
    event("turn_start", {}),
    event("user_message", { text: "Draw it" }),
    ...showing(chart),
    event("assistant_message", { text: "Here." }),
    event("turn_end", { reason: "completed" }),
    event("turn_start", {}),
    event("user_message", { text: "And the rest" }),
    ...showing(logo, card, guide),
    event("assistant_message", { text: "Here are the rest." }),
    event("turn_end", { reason: "completed" }),
  )

  expect(galleryOf(transcriptOf(view(events))).map((file) => file.name)).toEqual(["chart.png", "logo.svg"])
  expect(["image/png", "image/jpeg", "image/gif", "image/webp", "image/svg+xml", "application/pdf", "text/markdown", "text/html"].map(viewerOf)).toEqual(["image", "image", "image", "image", "svg", "pdf", "markdown", undefined])
})
