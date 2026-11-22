import { expect, test } from "bun:test"
import {
  afterTurn,
  dispatchOf,
  editQueued,
  leaveQueue,
  NO_QUEUE,
  queueMessage,
  reachedSession,
  releaseQueue,
  removeQueued,
  saveQueued,
  sendFailed,
  sendQueuedNow,
  stopEditing,
  type Queue,
} from "../web/src/queue.ts"

// The messages the owner queues while evie works, as T3 Code queues them,
// apart from the screen: one at a time, each waits for the turn before it
// to end, and is then sent as a turn of its own.

const words = (queue: Queue) => queue.items.map((item) => item.text)
const queueOf = (...texts: string[]) => texts.reduce((queue, text) => queueMessage(queue, text, []), NO_QUEUE)
const IDLE = { turnRunning: false, opened: true }

test("messages queued wait in the order they were queued; once a turn finishes, the first is let go, as a turn of its own", () => {
  const queue = queueOf("Then water them", "Then weed the paths")
  expect(words(queue)).toEqual(["Then water them", "Then weed the paths"])

  const first = afterTurn(queue, "completed")
  expect(first.send?.text).toBe("Then water them")
  expect(words(first.queue)).toEqual(["Then weed the paths"])
  expect(first.queue.inFlight?.text).toBe("Then water them")
})

test("one message at a time: the next is let go only once the one in flight has reached the session and its turn has ended", () => {
  const first = afterTurn(queueOf("one", "two"), "completed")
  // In flight, nothing more goes, whatever asks: an edit saved, an edit given up, a turn's end seen twice, a reconnect.
  expect(releaseQueue(first.queue, IDLE).send).toBeUndefined()
  expect(afterTurn(first.queue, "completed").send).toBeUndefined()
  expect(releaseQueue(stopEditing(first.queue), IDLE).send).toBeUndefined()

  // It reaches the session, whose turn runs: still nothing.
  const reached = reachedSession(first.queue, "one")
  expect(reached.inFlight).toBeUndefined()
  expect(releaseQueue(reached, { turnRunning: true, opened: true }).send).toBeUndefined()
  // Another message reaching it, as from a second tab, is not this one.
  expect(reachedSession(first.queue, "something else").inFlight?.text).toBe("one")

  // Its turn ends: the next goes.
  expect(afterTurn(reached, "completed").send?.text).toBe("two")
})

test("nothing is let go while a turn runs, before the session is open, while the queue is held, or while the first is being edited", () => {
  const queue = queueOf("one", "two")
  expect(releaseQueue(queue, IDLE).send?.text).toBe("one")
  expect(releaseQueue(queue, { turnRunning: true, opened: true }).send).toBeUndefined()
  expect(releaseQueue(queue, { turnRunning: false, opened: false }).send).toBeUndefined()
  expect(releaseQueue(afterTurn(queue, "cancelled").queue, IDLE).send).toBeUndefined()
  expect(releaseQueue(editQueued(queue, queue.items[0]!.key), IDLE).send).toBeUndefined()
  // Another being edited does not stop the first going.
  expect(releaseQueue(editQueued(queue, queue.items[1]!.key), IDLE).send?.text).toBe("one")
})

test("a turn stopped, or one that failed, holds the queue, saying why; only a message sent from it by hand lets it go on, not a turn finished another way", () => {
  const queue = queueOf("one", "two", "three")
  const stopped = afterTurn(queue, "cancelled")
  expect(stopped.send).toBeUndefined()
  expect(stopped.queue.held).toBe("stopped")
  expect(afterTurn(queue, "error").queue.held).toBe("failed")

  // A turn the box or the terminal started, which finishes, leaves it held.
  const later = afterTurn(stopped.queue, "completed")
  expect(later.send).toBeUndefined()
  expect(later.queue.held).toBe("stopped")

  // Sent by hand, it goes, and the queue is no longer held.
  const byHand = sendQueuedNow(later.queue, later.queue.items[0]!.key)!
  expect(byHand.send.text).toBe("one")
  expect(byHand.queue.held).toBeUndefined()
  expect(byHand.queue.inFlight?.text).toBe("one")
  expect(afterTurn(reachedSession(byHand.queue, "one"), "completed").send?.text).toBe("two")
})

test("a turn ending with nothing queued holds nothing, and a queue emptied is held no longer", () => {
  expect(afterTurn(NO_QUEUE, "cancelled").queue.held).toBeUndefined()
  const held = afterTurn(queueOf("one"), "cancelled").queue
  expect(removeQueued(held, held.items[0]!.key).held).toBeUndefined()
})

test("a message that could not be sent goes back to the front of the queue, which is held", () => {
  const first = afterTurn(queueOf("one", "two"), "completed")
  const failed = sendFailed(first.queue, first.send!)
  expect(words(failed)).toEqual(["one", "two"])
  expect(failed.held).toBe("failed")
  expect(failed.inFlight).toBeUndefined()
})

test("each queued message has a key of its own, kept as others come and go; one can be taken off the queue", () => {
  const queue = queueOf("one", "two", "three")
  expect(words(removeQueued(queue, queue.items[1]!.key))).toEqual(["one", "three"])
  const more = queueMessage(removeQueued(queue, queue.items[0]!.key), "four", [])
  expect(new Set(more.items.map((item) => item.key)).size).toBe(3)
})

test("a queued message being edited keeps its place; saved, it has its new words, and emptied, it goes; only one is edited at a time", () => {
  const queue = queueOf("one", "two")
  const [one, two] = queue.items
  const editing = editQueued(queue, one!.key)
  expect(editing.editing).toBe(one!.key)
  // While one is being edited, another is not.
  expect(editQueued(editing, two!.key).editing).toBe(one!.key)

  const saved = saveQueued(editing, one!.key, "one, said better", [])
  expect(saved.editing).toBeUndefined()
  expect(words(saved)).toEqual(["one, said better", "two"])
  expect(words(saveQueued(editQueued(saved, two!.key), two!.key, "  ", []))).toEqual(["one, said better"])
  expect(words(stopEditing(editQueued(saved, two!.key)))).toEqual(["one, said better", "two"])
})

test("leaving the session holds the queue, so nothing is sent while the owner cannot see it, and gives up an edit; a queue held already keeps why", () => {
  const queue = editQueued(queueOf("one", "two"), 1)
  const left = leaveQueue(queue)
  expect(left.held).toBe("left")
  expect(left.editing).toBeUndefined()
  expect(words(left)).toEqual(["one", "two"])
  expect(leaveQueue(afterTurn(queueOf("one"), "cancelled").queue).held).toBe("stopped")
  expect(leaveQueue(NO_QUEUE)).toEqual(NO_QUEUE)
})

test("a message from the box starts a turn when none runs; while one runs it steers it, as by default, or with mod held is queued, as T3 Code's are; while a question waits it is never queued", () => {
  expect(dispatchOf({ working: false, alternate: false, asked: false })).toBe("send")
  expect(dispatchOf({ working: false, alternate: true, asked: false })).toBe("send")
  expect(dispatchOf({ working: true, alternate: false, asked: false })).toBe("steer")
  expect(dispatchOf({ working: true, alternate: true, asked: false })).toBe("queue")
  // As a permission prompt waits, mod and Enter says no and what to do instead, as Enter does.
  expect(dispatchOf({ working: true, alternate: true, asked: true })).toBe("steer")
})
