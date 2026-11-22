import type { EventBodies } from "../../src/core/types.ts"
import type { Outgoing } from "./conversation.ts"

// The messages the owner queues while evie works, apart from the screen, as
// T3 Code queues a follow-up: a message sent from the box steers the turn
// running, or with mod held, waits here instead, to be sent as a turn of
// its own once the turn before it has finished. The page holds the queue,
// and evie's server knows nothing of it until a message is sent from it.
//
// One message goes at a time. One let go is in flight until it reaches the
// session, as its message there shows, or cannot be sent; the next is let
// go only when nothing is in flight, no turn runs, the session is open,
// the queue is not held, and the first is not being edited. The queue is
// held by a turn that was stopped or failed, and by the owner leaving the
// session, and only a message sent from it by hand lets it go on.

// A message waiting, by a key of its own, with the files attached to it.
export type Queued = { key: number; text: string; files: readonly Outgoing[] }

// Why the queue is held: a turn stopped, a turn or a send that failed, or
// the owner left the session, while which nothing is sent.
export type Held = "stopped" | "failed" | "left"

// The messages waiting, in order; the one being edited in the box, which
// keeps its place and is not sent meanwhile; why the queue is held, if it
// is; the message let go and not yet in the session; and the key the next
// one takes.
export type Queue = { items: readonly Queued[]; editing: number | undefined; held: Held | undefined; inFlight: Queued | undefined; nextKey: number }

export const NO_QUEUE: Queue = { items: [], editing: undefined, held: undefined, inFlight: undefined, nextKey: 1 }

// The queue with a message added at its end.
export const queueMessage = (queue: Queue, text: string, files: readonly Outgoing[]): Queue => ({
  ...queue,
  items: [...queue.items, { key: queue.nextKey, text, files }],
  nextKey: queue.nextKey + 1,
})

// The queue without the message, which is no longer being edited. Emptied,
// it is held no longer: what held it was about the messages in it.
export function removeQueued(queue: Queue, key: number): Queue {
  const items = queue.items.filter((item) => item.key !== key)
  return { ...queue, items, editing: queue.editing === key ? undefined : queue.editing, held: items.length === 0 ? undefined : queue.held }
}

// A message taken off the queue to be sent at once, by hand: into the turn
// running, which it steers, or as a turn of its own. Sent by hand, the
// queue is no longer held.
export function sendQueuedNow(queue: Queue, key: number): { queue: Queue; send: Queued } | undefined {
  const send = queue.items.find((item) => item.key === key)
  return send === undefined ? undefined : { queue: { ...removeQueued(queue, key), held: undefined, inFlight: send }, send }
}

// The message put in the box to be edited, in its place in the queue,
// unless another is being edited.
export const editQueued = (queue: Queue, key: number): Queue => (queue.editing === undefined && queue.items.some((item) => item.key === key) ? { ...queue, editing: key } : queue)

// The message edited, with its new words and files, in its place; one left
// with neither goes.
export function saveQueued(queue: Queue, key: number, text: string, files: readonly Outgoing[]): Queue {
  if (text.trim() === "" && files.length === 0) return removeQueued(queue, key)
  return { ...queue, items: queue.items.map((item) => (item.key === key ? { ...item, text, files } : item)), editing: undefined }
}

// The edit given up: the message stays as it was.
export const stopEditing = (queue: Queue): Queue => ({ ...queue, editing: undefined })

// The first message, taken off to be sent and in flight, when the queue
// may go on.
export function releaseQueue(queue: Queue, session: { turnRunning: boolean; opened: boolean }): { queue: Queue; send?: Queued } {
  const first = queue.items[0]
  if (first === undefined || queue.inFlight !== undefined || session.turnRunning || !session.opened || queue.held !== undefined || first.key === queue.editing) return { queue }
  return { queue: { ...removeQueued(queue, first.key), inFlight: first }, send: first }
}

// What the queue does as a turn ends: one that finished lets the first
// message go, when it may; one that was stopped, or failed, holds the
// queue, as T3 Code holds its queue after an interruption.
export function afterTurn(queue: Queue, reason: EventBodies["turn_end"]["reason"]): { queue: Queue; send?: Queued } {
  if (reason === "completed") return releaseQueue(queue, { turnRunning: false, opened: true })
  if (queue.items.length === 0 || queue.held !== undefined) return { queue }
  return { queue: { ...queue, held: reason === "cancelled" ? "stopped" : "failed" } }
}

// A message of the owner's reached the session: the one in flight, if it
// says the same, is no longer.
export const reachedSession = (queue: Queue, text: string): Queue => (queue.inFlight?.text === text ? { ...queue, inFlight: undefined } : queue)

// The message in flight has done what it does without reaching the
// session as a message, as a command run.
export const doneInFlight = (queue: Queue): Queue => ({ ...queue, inFlight: undefined })

// The message in flight could not be sent: it goes back to the front of
// the queue, which is held.
export const sendFailed = (queue: Queue, item: Queued): Queue => ({ ...queue, items: [item, ...queue.items.filter((each) => each.key !== item.key)], held: "failed", inFlight: undefined })

// The owner left the session: any edit is given up, and the queue is held,
// so that nothing is sent while the owner cannot see it. A queue held
// already keeps why.
export function leaveQueue(queue: Queue): Queue {
  if (queue.items.length === 0) return queue.editing === undefined ? queue : { ...queue, editing: undefined }
  return { ...queue, editing: undefined, held: queue.held ?? "left" }
}

// How a message sent from the box goes, as T3 Code's: with no turn running
// it is sent, and starts one; while one runs it steers it, or, with mod
// held, as mod and Enter or a mod-click on Send, it is queued. While a
// question waits it is never queued: it answers or declines it, as Enter
// does.
export const dispatchOf = ({ working, alternate, asked }: { working: boolean; alternate: boolean; asked: boolean }): "send" | "steer" | "queue" =>
  !working ? "send" : alternate && !asked ? "queue" : "steer"
