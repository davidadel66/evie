// How long a turn has worked, as the work log says it, apart from its
// screen, beside each call's own words (step-words.ts): counted on the
// server's clock, as T3 Code counts it, and said as T3 Code says a
// duration (adapted from T3 Code, MIT licence, notice in
// LICENSES/t3code-MIT.txt).

// What the line under a running turn says: how long it has worked, from
// its start to now, in milliseconds, in whole seconds as T3 Code's timer
// counts them. A clock that puts now before the start counts nothing.
export function workingFor(elapsed: number): string {
  const seconds = Math.max(0, Math.floor(elapsed / 1000))
  return `Working for ${seconds < 60 ? `${seconds}s` : durationWords(seconds * 1000)}`
}

// A duration, in milliseconds, as T3 Code says one: milliseconds below a
// second, tenths of a second below ten, then whole seconds, then minutes
// and seconds, then hours, minutes and seconds, leaving out what is none.
export function durationWords(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return "0ms"
  if (ms < 1000) return `${Math.round(ms)}ms`
  if (ms < 10_000) return `${Math.round(ms / 100) / 10}s`
  // The unit is chosen after rounding, so that 59.5 seconds is never "60s".
  const total = Math.round(ms / 1000)
  if (total < 60) return `${total}s`
  const [hours, minutes, seconds] = [Math.floor(total / 3600), Math.floor((total % 3600) / 60), total % 60]
  if (hours === 0) return `${minutes}m ${seconds}s`
  if (minutes > 0) return `${hours}h ${minutes}m ${seconds}s`
  return seconds > 0 ? `${hours}h ${seconds}s` : `${hours}h`
}

// How far the server's clock is ahead of the browser's, as an event that
// has just arrived shows it: its time on the server's clock, less when it
// arrived on the browser's. The line only ever makes an event later, so
// the largest of these is nearest the truth. `known` is the largest yet.
export const clockOffset = (known: number | undefined, eventTime: number, arrived: number) => Math.max(known ?? -Infinity, eventTime - arrived)

// The offset this page has worked out, from the events it has heard as
// they came, and the time now on the server's clock, which a running
// turn's time is counted on, as T3 Code counts it.
let offset: number | undefined
export function heard(eventTime: number, arrived = Date.now()) {
  offset = clockOffset(offset, eventTime, arrived)
}
export const serverNow = () => Date.now() + (offset ?? 0)
