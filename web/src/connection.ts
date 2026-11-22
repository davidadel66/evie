import type { Evie } from "../../src/core/types.ts"
import { connect, ConnectError } from "../../src/server/connect.ts"
import { PATHS } from "../../src/server/wire.ts"

// The web app's connection to evie's server, apart from its screens, which
// are told its state and redrawn as it changes. It is the same network
// client the terminal uses, so it catches up by itself after a drop.

// What the screens are told, with reasons in words for the owner.
// Connecting, until the first connection is made, the reason says why the
// last try failed. Reconnecting, after a drop, it says why the line is down.
// Signed out, the screens offer to sign in, and the reason, when there is
// one, says why.
export type ConnectionState =
  | { status: "connecting"; reason?: string }
  | { status: "connected"; evie: Evie }
  | { status: "reconnecting"; reason: string }
  | { status: "signed out"; reason?: string }

// What the one quiet line under the header says while the page is not
// connected: the word, as Working is said, and why, when there is a why.
// While connected, and when signed out, which has a page of its own, there
// is no line.
export function connectionLine(state: ConnectionState): { word: string; reason?: string } | undefined {
  if (state.status === "connecting") return { word: "Connecting", ...(state.reason !== undefined && { reason: state.reason }) }
  if (state.status === "reconnecting") return { word: "Reconnecting", reason: state.reason }
  return undefined
}

export type ConnectionOptions = {
  // The server's address: the page's own, in a browser.
  url: string
  // A device's token. A browser has none: it connects with its cookie.
  token?: string
  // The code of a sign-in link the page was opened with, traded for the
  // cookie before connecting.
  code?: string
  // For tests: the first wait, in milliseconds, before trying again, and
  // how long a sign-in may wait for the server's answer, ten seconds when
  // left out.
  firstWait?: number
  answerWithin?: number
}

export type Connection = {
  // The state now. It is the same object until the state changes.
  state(): ConnectionState
  // Calls the listener each time the state changes, until the function
  // returned is called.
  subscribe(listener: () => void): () => void
  // Signs in with a token pasted in, as on another device: trades it for the
  // browser's cookie, and connects with that. Gives back what went wrong,
  // in words for the owner, or nothing once it is connecting. A sign-in
  // asked for while one is under way, or while it is connecting, is let be.
  signIn(proof: { token: string }): Promise<string | undefined>
  // Closes the connection, for good.
  close(): Promise<void>
}

export function openConnection(options: ConnectionOptions): Connection {
  let state: ConnectionState = { status: "connecting" }
  const listeners = new Set<() => void>()
  let evie: Evie | undefined
  // Fires when the connection is closed, and stops it trying again.
  const closing = new AbortController()
  // True while it is connecting, or signing in on the way to it, so that a
  // second sign-in, as from a double press, opens no second connection.
  let busy = false

  function set(next: ConnectionState) {
    state = next
    for (const listener of [...listeners]) listener()
  }

  // Connects, and tries again while the server cannot be reached, as when
  // it is being started again, waiting as the network client does after a
  // drop: one second, then twice as long each time, up to ten seconds.
  // Once connected, the network client connects again by itself when the
  // line drops, and catches up on what was missed meanwhile.
  // `signInFailed` says why a sign-in code just failed, for when the browser
  // is signed out.
  async function start(signInFailed?: string) {
    busy = true
    try {
      await tryUntilAnswered(signInFailed)
    } finally {
      busy = false
    }
  }
  async function tryUntilAnswered(signInFailed?: string) {
    for (let wait = options.firstWait ?? FIRST_WAIT; ; wait = Math.min(wait * 2, LONGEST_WAIT)) {
      try {
        const client: Evie = await connect({
          url: options.url,
          ...(options.token !== undefined && { token: options.token }),
          ...(options.firstWait !== undefined && { firstWait: options.firstWait }),
          // So that an address that takes the connection and never answers is tried again.
          signal: AbortSignal.any([closing.signal, AbortSignal.timeout(CONNECT_TIMEOUT)]),
          onDrop: (reason) => set({ status: "reconnecting", reason: `The connection to evie's server dropped: ${reason}.` }),
          onBack: () => set({ status: "connected", evie: client }),
          // The server took the token back, or refuses it or this page as
          // the client connects again, which trying again will not change.
          onEnd: (cause) => {
            evie = undefined
            const reason = cause.reason === "token" ? "This browser was signed out: its token was taken back." : cause.message
            set({ status: "signed out", reason })
          },
        })
        if (closing.signal.aborted) return void client.close()
        evie = client
        return set({ status: "connected", evie })
      } catch (cause) {
        if (!(cause instanceof ConnectError)) throw cause
        if (closing.signal.aborted) return
        // No cookie, or one whose token the server does not know: a browser
        // that has not signed in, or whose token was taken back while it
        // was away. Either way it only needs to sign in.
        if (cause.reason === "token") return set({ status: "signed out", ...(signInFailed !== undefined && { reason: signInFailed }) })
        // Reached at an address the server does not let in, which signing in
        // will not change: the reason says what to change.
        if (cause.reason === "page") return set({ status: "signed out", reason: cause.message })
        set({ status: "connecting", reason: cause.message })
        await pause(wait, closing.signal)
      }
    }
  }
  // A sign-in code is traded first. When it fails, the browser may be
  // signed in already, from before, so it connects all the same.
  const answerWithin = options.answerWithin ?? ANSWER_WITHIN
  busy = true
  void (options.code === undefined ? start() : trade(options.url, { code: options.code }, closing.signal, answerWithin).then(start))

  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    async signIn(proof) {
      if (busy) return undefined
      busy = true
      const failed = await trade(options.url, proof, closing.signal, answerWithin)
      if (failed !== undefined) {
        busy = false
        return failed
      }
      set({ status: "connecting" })
      void start()
      return undefined
    },
    async close() {
      closing.abort()
      await evie?.close()
    },
  }
}

// Trades a sign-in code or a token for the browser's cookie, which the
// browser keeps from the answer. Gives back what went wrong, in words for
// the owner, or nothing. It waits for the answer no longer than `within`
// milliseconds, or until the connection is closed.
async function trade(url: string, proof: { code: string } | { token: string }, closing: AbortSignal, within: number): Promise<string | undefined> {
  const timeout = AbortSignal.timeout(within)
  let answer: Response
  try {
    answer = await fetch(new URL(PATHS.login, url), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(proof),
      signal: AbortSignal.any([closing, timeout]),
    })
  } catch {
    if (timeout.aborted) return "evie's server did not answer in time. Check that `evie serve` is running, and try again."
    return "evie's server could not be reached. Check that `evie serve` is running."
  }
  if (answer.ok) return undefined
  if (answer.status === 401 && "code" in proof) return "This sign-in link has been used already or has expired. Run `evie web` for a new one."
  if (answer.status === 401) return "evie did not accept that token. Check that it was copied whole."
  if (answer.status === 403) return 'evie\'s server does not let this address sign in. Add it to "origins" under "server" in evie.json.'
  if (answer.status === 503) return "evie's server could not check that just now. Try again."
  return `Signing in failed: evie's server answered ${answer.status}.`
}

const FIRST_WAIT = 1000
const LONGEST_WAIT = 10_000
const CONNECT_TIMEOUT = 10_000
const ANSWER_WITHIN = 10_000

// Waits, or until the signal fires.
function pause(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, ms)
    signal.addEventListener("abort", () => {
      clearTimeout(timer)
      resolve()
    }, { once: true })
  })
}
