import type { EventBodies } from "../../src/core/types.ts"
import type { Outgoing } from "./conversation.ts"

// The messages the owner queues while evie works, apart from the screen, as
// T3 Code queues a follow-up: a message sent from the box steers the turn
// running, or with mod held, waits here instead, to be sent as a turn of
// its own once the turn before it has finished. The page holds the queue,
// for as long as it has the session open, and evie's server knows nothing
// of it until a message is sent from it.

// A message waiting, by a key of its own, with the files attached to it.
export type Queued = { key: number; text: string; files: readonly Outgoing[] }

// The messages waiting, in order; the one being edited in the box, which
// keeps its place and is not sent meanwhile; whether the queue is held, as
// after a turn that was stopped; and the key the next one takes.
export type Queue = { items: readonly Queued[]; editing: number | undefined; held: boolean; nextKey: number }

export const NO_QUEUE: Queue = { items: [], editing: undefined, held: false, nextKey: 1 }

// The queue with a message added at its end.
export const queueMessage = (queue: Queue, text: string, files: readonly Outgoing[]): Queue => ({
  ...queue,
  items: [...queue.items, { key: queue.nextKey, text, files }],
  nextKey: queue.nextKey + 1,
})

// The queue without the message, which is no longer being edited.
export const removeQueued = (queue: Queue, key: number): Queue => ({
  ...queue,
  items: queue.items.filter((item) => item.key !== key),
  editing: queue.editing === key ? undefined : queue.editing,
})

// A message taken off the queue to be sent at once, by hand: into the turn
// running, which it steers, or as a turn of its own. Sent by hand, the
// queue is no longer held.
export function sendQueuedNow(queue: Queue, key: number): { queue: Queue; send: Queued } | undefined {
  const send = queue.items.find((item) => item.key === key)
  return send === undefined ? undefined : { queue: { ...removeQueued(queue, key), held: false }, send }
}

// The message put in the box to be edited, in its place in the queue.
export const editQueued = (queue: Queue, key: number): Queue => (queue.items.some((item) => item.key === key) ? { ...queue, editing: key } : queue)

// The message edited, with its new words and files, in its place; one left
// with neither goes.
export function saveQueued(queue: Queue, key: number, text: string, files: readonly Outgoing[]): Queue {
  if (text.trim() === "" && files.length === 0) return removeQueued(queue, key)
  return { ...queue, items: queue.items.map((item) => (item.key === key ? { ...item, text, files } : item)), editing: undefined }
}

// The edit given up: the message stays as it was.
export const stopEditing = (queue: Queue): Queue => ({ ...queue, editing: undefined })

// The first message, taken off to be sent, when the queue may go on: it is
// not held, and the first is not being edited.
export function releaseQueue(queue: Queue): { queue: Queue; send?: Queued } {
  const first = queue.items[0]
  if (queue.held || first === undefined || first.key === queue.editing) return { queue }
  return { queue: removeQueued(queue, first.key), send: first }
}

// What the queue does as a turn ends: one that finished lets the first
// message go; one that was stopped, or failed, holds the queue, as T3 Code
// holds its queue after an interruption, until a message is sent from it
// by hand and that turn finishes.
export function afterTurn(queue: Queue, reason: EventBodies["turn_end"]["reason"]): { queue: Queue; send?: Queued } {
  if (reason !== "completed") return { queue: { ...queue, held: true } }
  return releaseQueue({ ...queue, held: false })
}

// How a message sent from the box goes, as T3 Code's: with no turn running
// it is sent, and starts one; while one runs it steers it, or, with mod
// held, as mod and Enter or a mod-click on Send, it is queued.
export const dispatchOf = ({ working, alternate }: { working: boolean; alternate: boolean }): "send" | "steer" | "queue" => (!working ? "send" : alternate ? "queue" : "steer")
