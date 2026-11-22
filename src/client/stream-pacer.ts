// Smooths a reply's streamed pieces into an even flow of characters, so
// fast bursts from a model glide onto the screen instead of landing in
// sudden jumps. Any client can share it by passing its own frame clock:
// requestAnimationFrame in a browser, a short timer in an interactive
// terminal, or a manual clock in tests.

// Waits for the client's next frame and calls `apply` then; gives back a
// way to stop waiting. A hidden browser tab draws no frames, so it waits
// until the tab is shown again.
export type NextFrame = (apply: () => void) => () => void

export type StreamPacerOptions = {
  // Waits for the next frame to reveal the next slice.
  frame: NextFrame
  // Called with each revealed slice of text, never empty.
  emit(text: string): void
  // Current time in milliseconds, for scaling slices to the real time
  // between frames (60Hz vs 120Hz displays, or a tab returning after a
  // pause). Defaults to `performance.now` when available, else `Date.now`.
  now?: () => number
  // Baseline reveal speed, in characters per second, when the buffer is
  // small. Keeps steady streams calm and readable.
  charsPerSecond?: number
  // Target time, in milliseconds, to drain a burst sitting in the buffer.
  // Larger backlogs speed up proportionally so the display never falls far
  // behind a fast model.
  catchUpMs?: number
}

export type StreamPacer = {
  // Adds newly received text to the buffer and schedules a frame if needed.
  push(text: string): void
  // Cancels any pending frame, clears the buffer, and returns whatever had
  // not been revealed yet (without calling `emit`). Callers that need the
  // rest applied ahead of another event can take it in one step.
  take(): string
  // Reveals whatever is still buffered right away, in one `emit` call, and
  // cancels any pending frame.
  flush(): void
  // Cancels any pending frame and discards buffered text without emitting.
  clear(): void
}

const DEFAULT_CHARS_PER_SECOND = 110
const DEFAULT_CATCH_UP_MS = 260
// Nominal frame duration (60Hz) used for the first frame after idle, and
// limits on elapsed time between frames so a brief hitch neither stalls
// nor dumps an entire backlog in one jump.
const NOMINAL_FRAME_MS = 16
const MIN_STEP_MS = 4
const MAX_STEP_MS = 80

const defaultNow = () => (typeof performance !== "undefined" ? performance.now() : Date.now())

export function createStreamPacer(options: StreamPacerOptions): StreamPacer {
  const { frame, emit } = options
  const now = options.now ?? defaultNow
  const charsPerSecond = options.charsPerSecond ?? DEFAULT_CHARS_PER_SECOND
  const catchUpMs = options.catchUpMs ?? DEFAULT_CATCH_UP_MS

  let buffer = ""
  let waiting: (() => void) | undefined
  let lastTick: number | undefined
  // Fractional characters carried from one frame to the next, so 60Hz and
  // 120Hz displays reveal at the same real-time speed.
  let carry = 0

  function cancel() {
    waiting?.()
    waiting = undefined
  }

  function resetClock() {
    lastTick = undefined
    carry = 0
  }

  function schedule() {
    if (buffer !== "" && waiting === undefined) waiting = frame(tick)
  }

  function tick() {
    waiting = undefined
    if (buffer === "") {
      resetClock()
      return
    }
    const at = now()
    const first = lastTick === undefined
    const elapsed = first ? NOMINAL_FRAME_MS : Math.min(MAX_STEP_MS, Math.max(MIN_STEP_MS, at - lastTick!))
    lastTick = at

    const baseline = (charsPerSecond * elapsed) / 1000
    const adaptive = (buffer.length * elapsed) / catchUpMs
    carry += Math.max(baseline, adaptive)
    const wanted = first ? Math.max(1, Math.floor(carry)) : Math.floor(carry)
    if (wanted < 1) {
      schedule()
      return
    }

    const count = safeSliceLength(buffer, Math.min(buffer.length, wanted))
    carry = count >= buffer.length ? 0 : Math.max(0, carry - count)
    const piece = buffer.slice(0, count)
    buffer = buffer.slice(count)
    if (buffer === "") resetClock()
    emit(piece)
    if (buffer !== "") schedule()
  }

  return {
    push(text) {
      if (text === "") return
      buffer += text
      schedule()
    },
    take() {
      cancel()
      const rest = buffer
      buffer = ""
      resetClock()
      return rest
    },
    flush() {
      const rest = this.take()
      if (rest !== "") emit(rest)
    },
    clear() {
      cancel()
      buffer = ""
      resetClock()
    },
  }
}

// Adjusts `count` so a slice never splits a UTF-16 surrogate pair (such as
// an emoji or mathematical symbol).
function safeSliceLength(buffer: string, count: number): number {
  if (count < buffer.length) {
    const code = buffer.charCodeAt(count - 1)
    if (code >= 0xd800 && code <= 0xdbff) return count + 1
  }
  return count
}
