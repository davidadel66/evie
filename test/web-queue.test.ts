import { expect, test } from "bun:test"
import { afterTurn, dispatchOf, editQueued, NO_QUEUE, queueMessage, releaseQueue, removeQueued, saveQueued, sendQueuedNow, stopEditing, type Queue } from "../web/src/queue.ts"

// The messages the owner queues while evie works, as T3 Code queues them,
// apart from the screen: each waits for the turn before it to end, and is
// then sent as a turn of its own.

const words = (queue: Queue) => queue.items.map((item) => item.text)
const queueOf = (...texts: string[]) => texts.reduce((queue, text) => queueMessage(queue, text, []), NO_QUEUE)

test("messages queued wait in the order they were queued; once a turn finishes, the first is sent as a turn of its own, and the next when that one finishes", () => {
  const queue = queueOf("Then water them", "Then weed the paths")
  expect(words(queue)).toEqual(["Then water them", "Then weed the paths"])

  const first = afterTurn(queue, "completed")
  expect(first.send?.text).toBe("Then water them")
  expect(words(first.queue)).toEqual(["Then weed the paths"])

  const second = afterTurn(first.queue, "completed")
  expect(second.send?.text).toBe("Then weed the paths")
  expect(words(second.queue)).toEqual([])
  expect(afterTurn(second.queue, "completed").send).toBeUndefined()
})

test("each queued message has a key of its own, kept as others come and go", () => {
  const queue = removeQueued(queueOf("one", "two"), queueOf("one", "two").items[0]!.key)
  const more = queueMessage(queue, "three", [])
  expect(new Set(more.items.map((item) => item.key)).size).toBe(2)
})

test("a queued message can be taken off the queue, or sent at once, into the turn that is running or as one of its own", () => {
  const queue = queueOf("one", "two", "three")
  const [one, two] = queue.items
  expect(words(removeQueued(queue, two!.key))).toEqual(["one", "three"])

  const now = sendQueuedNow(queue, two!.key)
  expect(now?.send.text).toBe("two")
  expect(words(now!.queue)).toEqual(["one", "three"])
  expect(sendQueuedNow(removeQueued(queue, one!.key), one!.key)).toBeUndefined()
})

test("a queued message being edited in the box keeps its place and is not sent; saved, it has its new words, and emptied, it goes", () => {
  const queue = queueOf("one", "two")
  const [one, two] = queue.items
  const editing = editQueued(queue, one!.key)
  expect(editing.editing).toBe(one!.key)

  // The turn ends while the first is being edited: the queue waits for it.
  const waited = afterTurn(editing, "completed")
  expect(waited.send).toBeUndefined()
  expect(words(waited.queue)).toEqual(["one", "two"])

  const saved = saveQueued(waited.queue, one!.key, "one, said better", [])
  expect(saved.editing).toBeUndefined()
  expect(words(saved)).toEqual(["one, said better", "two"])
  // With no turn running, the queue goes on once the edit is done.
  expect(releaseQueue(saved).send?.text).toBe("one, said better")

  expect(words(saveQueued(editQueued(saved, two!.key), two!.key, "  ", []))).toEqual(["one, said better"])
  expect(stopEditing(editQueued(saved, two!.key)).editing).toBeUndefined()
  expect(words(stopEditing(editQueued(saved, two!.key)))).toEqual(["one, said better", "two"])
})

test("a turn stopped, or one that failed, holds the queue: nothing is sent until one is sent by hand, and the next turn that finishes lets it go on", () => {
  const queue = queueOf("one", "two", "three")
  const stopped = afterTurn(queue, "cancelled")
  expect(stopped.send).toBeUndefined()
  expect(stopped.queue.held).toBe(true)
  expect(afterTurn(queue, "error").queue.held).toBe(true)
  expect(releaseQueue(stopped.queue).send).toBeUndefined()

  const byHand = sendQueuedNow(stopped.queue, stopped.queue.items[0]!.key)!
  expect(byHand.queue.held).toBe(false)
  expect(afterTurn(byHand.queue, "completed").send?.text).toBe("two")
})

test("a message from the box starts a turn when none runs; while one runs it steers it, as by default, or with mod held is queued, as T3 Code's are", () => {
  expect(dispatchOf({ working: false, alternate: false })).toBe("send")
  expect(dispatchOf({ working: false, alternate: true })).toBe("send")
  expect(dispatchOf({ working: true, alternate: false })).toBe("steer")
  expect(dispatchOf({ working: true, alternate: true })).toBe("queue")
})
