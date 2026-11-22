import { expect, test } from "bun:test"
import { createStreamPacer, type NextFrame } from "../src/client/stream-pacer.ts"

// A manual frame clock with a controllable timestamp, so tests can step
// frame by frame at 60Hz, 120Hz, or after a long pause.
function clock(start = 0) {
  let time = start
  let pending: (() => void) | undefined
  const frame: NextFrame = (apply) => {
    pending = apply
    return () => {
      if (pending === apply) pending = undefined
    }
  }
  return {
    frame,
    now: () => time,
    waiting: () => pending !== undefined,
    step(ms = 16) {
      time += ms
      const apply = pending
      pending = undefined
      apply?.()
    },
    drain(ms = 16, limit = 500) {
      let frames = 0
      while (pending !== undefined && frames < limit) {
        this.step(ms)
        frames += 1
      }
      return frames
    },
  }
}

test("spreads a burst across multiple frames in order instead of dumping it at once", () => {
  const c = clock()
  const slices: string[] = []
  const pacer = createStreamPacer({ frame: c.frame, now: c.now, emit: (piece) => slices.push(piece) })

  const burst = "Thinking about the beds by the fence."
  pacer.push(burst)
  expect(slices).toEqual([])
  expect(c.waiting()).toBe(true)

  c.step(16)
  expect(slices.length).toBe(1)
  expect(slices[0]!.length).toBeGreaterThan(0)
  expect(slices[0]!.length).toBeLessThan(burst.length)
  expect(c.waiting()).toBe(true)

  const rest = c.drain(16)
  expect(rest).toBeGreaterThan(2)
  expect(slices.join("")).toBe(burst)
  expect(c.waiting()).toBe(false)
})

test("scales up smoothly when a large backlog builds up so it catches up quickly", () => {
  const c = clock()
  const slices: string[] = []
  const pacer = createStreamPacer({ frame: c.frame, now: c.now, emit: (piece) => slices.push(piece) })

  const big = "x".repeat(2000)
  pacer.push(big)
  c.step(16)
  // First frame on a 2000-char burst drains much faster than the ~2-char baseline.
  expect(slices[0]!.length).toBeGreaterThan(50)
  expect(slices[0]!.length).toBeLessThan(big.length)

  const frames = 1 + c.drain(16)
  expect(slices.join("")).toBe(big)
  // Drains within roughly a couple of seconds, not 18+ seconds at baseline speed.
  expect(frames).toBeLessThan(120)
})

test("reveals at the same real-time speed on 60Hz and 120Hz displays", () => {
  const sample = "The quick brown fox jumps over the lazy dog."
  const runAt = (stepMs: number) => {
    const c = clock()
    let shown = ""
    const pacer = createStreamPacer({ frame: c.frame, now: c.now, emit: (piece) => (shown += piece) })
    pacer.push(sample)
    // Run for ~200ms of simulated time.
    const steps = Math.round(200 / stepMs)
    for (let i = 0; i < steps; i++) c.step(stepMs)
    return shown.length
  }

  const at60 = runAt(16)
  const at120 = runAt(8)
  expect(Math.abs(at60 - at120)).toBeLessThanOrEqual(4)
})

test("never splits a UTF-16 surrogate pair across frames", () => {
  const c = clock()
  const slices: string[] = []
  const pacer = createStreamPacer({ frame: c.frame, now: c.now, charsPerSecond: 60, emit: (piece) => slices.push(piece) })

  // Mix of 1-code-unit ASCII and 2-code-unit emoji so 1-char slices land on high surrogates.
  const text = "a😀b🚀c🎉d"
  pacer.push(text)
  c.drain(16)

  expect(slices.join("")).toBe(text)
  for (const slice of slices) {
    // No unpaired high or low surrogate at either end of any emitted slice.
    const first = slice.charCodeAt(0)
    const last = slice.charCodeAt(slice.length - 1)
    expect(first < 0xdc00 || first > 0xdfff).toBe(true)
    expect(last < 0xd800 || last > 0xdbff).toBe(true)
  }
})

test("flush emits all remaining buffered text at once and cancels the pending frame", () => {
  const c = clock()
  const slices: string[] = []
  const pacer = createStreamPacer({ frame: c.frame, now: c.now, emit: (piece) => slices.push(piece) })

  pacer.push("Hello world from evie")
  c.step(16)
  expect(c.waiting()).toBe(true)

  pacer.flush()
  expect(c.waiting()).toBe(false)
  expect(slices.join("")).toBe("Hello world from evie")

  // A later frame or flush emits nothing more.
  c.step(16)
  pacer.flush()
  expect(slices.join("")).toBe("Hello world from evie")
})

test("take returns the remaining buffer without calling emit, and clear drops it silently", () => {
  const c = clock()
  const slices: string[] = []
  const pacer = createStreamPacer({ frame: c.frame, now: c.now, emit: (piece) => slices.push(piece) })

  pacer.push("First half, second half")
  c.step(16)
  const revealed = slices.join("")
  const rest = pacer.take()
  expect(revealed + rest).toBe("First half, second half")
  expect(c.waiting()).toBe(false)
  expect(slices.join("")).toBe(revealed)

  pacer.push("Discarded text")
  expect(c.waiting()).toBe(true)
  pacer.clear()
  expect(c.waiting()).toBe(false)
  c.step(16)
  expect(slices.join("")).toBe(revealed)
})

test("ignores empty pushes", () => {
  const c = clock()
  const slices: string[] = []
  const pacer = createStreamPacer({ frame: c.frame, now: c.now, emit: (piece) => slices.push(piece) })

  pacer.push("")
  expect(c.waiting()).toBe(false)
  c.step(16)
  expect(slices).toEqual([])
})
