import { useEffect, useState, useSyncExternalStore } from "react"
import type { SessionSummary } from "../../src/core/types.ts"
import { selectedIn } from "./address.ts"
import { Button } from "./components/ui/button.tsx"
import type { Connection } from "./connection.ts"
import { Check, X } from "./icons.tsx"
import { latestTurnEnd, rememberedNotices, shownAs, watchSessions, type Notice, type TurnEnd } from "./notices.ts"
import { select } from "./selection.ts"
import { titleOf } from "./session-list.ts"
import type { Sessions } from "./sessions.ts"
import { NEW_CHAT, workspaceIn } from "./workspace.ts"

// Notifications, as T3 Code's: when a session comes to need the owner, or
// its turn finishes or fails, the owner is told, with the session's name.
// While the window is in front, a notice of another session floats at the
// top of the page, with Open; while it is behind others, the system's
// notification says it, and opens the session when clicked, if the owner
// has turned them on. Nothing is said of the session the owner is looking
// at. A sound goes with it, if the owner chose one. It is all read from the
// summaries every page is sent, and for a turn's end, the session's latest
// events, so evie's server does nothing new for it.

// A notice in the page: of which session, saying what, and for a turn
// that failed, what went wrong.
type Shown = { id: number; session: string; notice: Notice; name: string; error?: string }

// How long a notice that a turn has ended stays. One that the session
// needs the owner stays while it does.
const ENDED_STAYS = 8_000
// How many notices are on the page at once, the latest last.
const AT_ONCE = 3
// How many of a session's latest events are read for its turn's end.
const READ = 20

export function Notices({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const [shown, setShown] = useState<Shown[]>([])
  const looking = lookedAt(useSyncExternalStore(onAddressChange, () => location.hash))

  useEffect(() => {
    let next = 0
    const watch = watchSessions()
    // The system's notifications showing, closed once the window is in front again.
    const system = new Set<Notification>()
    const dismiss = (id: number) => setShown((was) => was.filter((each) => each.id !== id))

    function tell(summary: SessionSummary, notice: Notice, end?: TurnEnd) {
      const settings = rememberedNotices()
      const where = shownAs({
        session: summary.session,
        lookingAt: lookedAt(location.hash),
        focused: document.visibilityState === "visible" && document.hasFocus(),
        settings,
        allowed: typeof Notification === "function" && Notification.permission === "granted",
      })
      if (where.sound) chime(notice)
      const name = titleOf(summary)
      if (where.inPage) {
        const id = next++
        setShown((was) => [...was.filter((each) => !(each.session === summary.session && each.notice === notice)), { id, session: summary.session, notice, name, ...(end?.error !== undefined && { error: end.error }) }].slice(-AT_ONCE))
        if (notice !== "needs you") setTimeout(() => dismiss(id), ENDED_STAYS)
      }
      if (where.system) {
        try {
          const shownNow = new Notification(WORDS[notice], { body: name, tag: summary.session })
          system.add(shownNow)
          shownNow.addEventListener("click", () => {
            shownNow.close()
            focus()
            select({ session: summary.session })
          })
          shownNow.addEventListener("close", () => system.delete(shownNow))
        } catch {
          // A browser can have notifications and refuse to show one from a page.
        }
      }
    }

    // A turn's end is read from the session's latest events, once the line is up.
    async function endOf(session: string): Promise<TurnEnd | undefined> {
      const now = connection.state()
      if (now.status !== "connected") return undefined
      try {
        return latestTurnEnd((await now.evie.send({ type: "read_history", session, limit: READ })).events)
      } catch {
        return undefined
      }
    }

    function look() {
      const listed = sessions.state()
      if (!listed.listed) return
      for (const { session, change } of watch.see(listed.sessions)) {
        if (change === "needs you") tell(session, "needs you")
        else
          void endOf(session.session).then((end) => {
            const notice = watch.ended(session.session, end)
            const latest = sessions.state().sessions.find((each) => each.session === session.session) ?? session
            if (notice !== undefined) tell(latest, notice, end)
          })
      }
      // A notice that a session needs the owner goes once it no longer does.
      const waiting = new Set(listed.sessions.filter((each) => each.state === "needs_you").map((each) => each.session))
      setShown((was) => (was.some((each) => each.notice === "needs you" && !waiting.has(each.session)) ? was.filter((each) => each.notice !== "needs you" || waiting.has(each.session)) : was))
    }
    const unsubscribe = sessions.subscribe(look)
    look()

    // Back in front, the system's notifications have been seen, as T3 Code takes them.
    const seen = () => {
      for (const each of system) each.close()
      system.clear()
    }
    addEventListener("focus", seen)
    // A sound can be played only once the owner has used the page.
    addEventListener("pointerdown", unlockSound)
    addEventListener("keydown", unlockSound)
    return () => {
      unsubscribe()
      removeEventListener("focus", seen)
      removeEventListener("pointerdown", unlockSound)
      removeEventListener("keydown", unlockSound)
      seen()
    }
  }, [connection, sessions])

  // A session opened takes its notices away.
  useEffect(() => {
    if (looking !== undefined) setShown((was) => (was.some((each) => each.session === looking) ? was.filter((each) => each.session !== looking) : was))
  }, [looking])

  // The region is always on the page, and only the notices in it come and
  // go, so that a screen reader, which listens to a region already there, says them.
  return (
    <div role="region" aria-label="Notices" aria-live="polite" className="pointer-events-none fixed top-header-height right-4 z-50 flex w-notice flex-col gap-2">
      {shown.map((each) => (
        <section key={each.id} aria-label={`${WORDS[each.notice]}: ${each.name}`} className="pointer-events-auto flex items-center gap-2 rounded-md border border-line bg-surface py-2 pr-2 pl-4 shadow-float">
          <div className="flex min-w-0 flex-1 flex-col gap-1 py-1">
            <State notice={each.notice} />
            <p className="truncate text-label text-text">{each.name}</p>
            {each.error !== undefined && <p className="truncate text-caption text-text-muted">{each.error}</p>}
          </div>
          <Button variant="secondary" onClick={() => select({ session: each.session })}>
            Open
          </Button>
          <Button variant="icon" aria-label="Dismiss" onClick={() => setShown((was) => was.filter((other) => other.id !== each.id))}>
            <X />
          </Button>
        </section>
      ))}
    </div>
  )
}

// The session the owner is looking at, as the address has it: the one
// open in the session view, or the one in a workspace's side chat.
function lookedAt(hash: string): string | undefined {
  const selected = selectedIn(hash)
  if (selected !== undefined) return "session" in selected ? selected.session : undefined
  const chat = workspaceIn(hash)?.chat
  return chat === NEW_CHAT ? undefined : chat
}
function onAddressChange(listener: () => void) {
  addEventListener("hashchange", listener)
  return () => removeEventListener("hashchange", listener)
}

// What each notice says, as its title.
const WORDS: Record<Notice, string> = { "needs you": "Needs you", finished: "Finished", failed: "Failed" }

// The notice's state in words, as the board's Status says one: Needs you
// with its dot, Finished with a check, and Failed, each in its colour.
function State({ notice }: { notice: Notice }) {
  if (notice === "needs you") {
    return (
      <span className="flex items-center gap-2 text-caption text-status-needs">
        <span aria-hidden="true" className="size-2 rounded-full bg-status-needs" />
        Needs you
      </span>
    )
  }
  if (notice === "finished") {
    return (
      <span className="flex items-center gap-1 text-caption text-status-done">
        <Check className="size-icon-sm" />
        Finished
      </span>
    )
  }
  return <span className="text-caption text-status-failed">Failed</span>
}

// The sound, made in the page, so that there is no file to fetch: two soft
// notes, rising when a session needs the owner and falling when a turn has
// ended. A browser plays sound only once the page has been used, so the
// player is made then, when the owner wants a sound.
let audio: AudioContext | undefined
function unlockSound() {
  const { mode } = rememberedNotices()
  if (mode !== "sound" && mode !== "both") return
  try {
    audio ??= new AudioContext()
    void audio.resume().catch(() => {})
  } catch {
    // No sound in this browser.
  }
}
function chime(notice: Notice) {
  if (audio === undefined || audio.state !== "running") return
  const notes = notice === "needs you" ? [660, 880] : [880, 660]
  notes.forEach((frequency, at) => {
    const start = audio!.currentTime + at * 0.14
    const tone = audio!.createOscillator()
    const loudness = audio!.createGain()
    tone.frequency.value = frequency
    loudness.gain.setValueAtTime(0.0001, start)
    loudness.gain.exponentialRampToValueAtTime(0.08, start + 0.02)
    loudness.gain.exponentialRampToValueAtTime(0.0001, start + 0.3)
    tone.connect(loudness).connect(audio!.destination)
    tone.start(start)
    tone.stop(start + 0.32)
  })
}
