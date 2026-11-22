import type { EventBodies, SessionSummary, StoredEvent } from "../../src/core/types.ts"

// Notifications' rules, apart from the page, as T3 Code's: which change in
// the sessions' summaries, which every page is sent, makes which notice,
// when one is skipped, and where it is shown. A session needs the owner
// when it comes to wait for an answer; it has finished, or failed, when
// its turn ends, as its log says.

export type Notice = "needs you" | "finished" | "failed"

// A session's latest turn end, as its log has it, with what went wrong when it failed.
export type TurnEnd = { seq: number; time: number; reason: EventBodies["turn_end"]["reason"]; error?: string }

// Follows the sessions' summaries. `see` is given the list each time it
// changes, and says which sessions now need the owner, and which were
// working, or waiting, and no longer are, whose turn may have ended: their
// log says how, and `ended` what that comes to. A session first seen makes
// no notice, whatever it is doing, nor does a turn that ended before the
// page first saw its session; and each turn's end is told once.
export function watchSessions() {
  const seen = new Map<string, { summary: SessionSummary; since: number; told?: number }>()
  return {
    see(sessions: readonly SessionSummary[]): { session: SessionSummary; change: "needs you" | "ended" }[] {
      const changes: { session: SessionSummary; change: "needs you" | "ended" }[] = []
      const now = new Set(sessions.map((each) => each.session))
      for (const session of seen.keys()) if (!now.has(session)) seen.delete(session)
      for (const summary of sessions) {
        const was = seen.get(summary.session)
        seen.set(summary.session, { since: summary.lastActive, ...was, summary })
        if (was === undefined) continue
        const before = was.summary.state
        if (summary.state === "needs_you" && before !== "needs_you") changes.push({ session: summary, change: "needs you" })
        else if ((before === "working" || before === "needs_you") && (summary.state === "active" || summary.state === "settled")) changes.push({ session: summary, change: "ended" })
      }
      return changes
    },
    ended(session: string, end: TurnEnd | undefined): "finished" | "failed" | undefined {
      const watched = seen.get(session)
      if (watched === undefined || end === undefined || end.time <= watched.since || end.seq === watched.told) return undefined
      watched.told = end.seq
      return end.reason === "completed" ? "finished" : end.reason === "error" ? "failed" : undefined
    },
  }
}

// The latest turn_end among a session's events, or nothing.
export function latestTurnEnd(events: readonly StoredEvent[]): TurnEnd | undefined {
  const end = events.findLast((event) => event.type === "turn_end")
  if (end === undefined) return undefined
  return { seq: end.seq, time: end.time, reason: end.body.reason, ...(end.body.error !== undefined && { error: end.body.error }) }
}

// How the owner wants to be told, as this browser keeps it: system
// notifications off, on, a sound only, or both, as T3 Code offers them, and
// separately whether a notice is shown in the page.
export type NoticeMode = "off" | "on" | "sound" | "both"
export type NoticeSettings = { mode: NoticeMode; inApp: boolean }

// Where a notice of `session` is shown: nowhere while the owner is looking
// at it, with the window in front; in the page, of any other session, while
// the window is in front; and otherwise as the system's notification, when
// they are on and the browser allows them. The sound, when it is on, goes
// with it either way.
export function shownAs(facts: { session: string; lookingAt: string | undefined; focused: boolean; settings: NoticeSettings; allowed: boolean }): { inPage: boolean; system: boolean; sound: boolean } {
  const { session, lookingAt, focused, settings, allowed } = facts
  if (focused && session === lookingAt) return { inPage: false, system: false, sound: false }
  return {
    inPage: focused && settings.inApp,
    system: !focused && allowed && (settings.mode === "on" || settings.mode === "both"),
    sound: settings.mode === "sound" || settings.mode === "both",
  }
}

// Where this browser keeps how the owner wants to be told.
const KEY = "evie.notifications"
const MODES: readonly NoticeMode[] = ["off", "on", "sound", "both"]

// How they start, which the settings' resets go back to: system
// notifications off, as in T3 Code, and notices in the page on.
export const NOTICE_DEFAULTS: NoticeSettings = { mode: "off", inApp: true }

// As this browser last kept them, or as they start. A browser can refuse
// to keep anything, as in a private window, and then they are as they start.
export function rememberedNotices(): NoticeSettings {
  try {
    const kept = JSON.parse(localStorage.getItem(KEY) ?? "{}") as Partial<NoticeSettings>
    return { mode: MODES.includes(kept.mode as NoticeMode) ? kept.mode! : NOTICE_DEFAULTS.mode, inApp: typeof kept.inApp === "boolean" ? kept.inApp : NOTICE_DEFAULTS.inApp }
  } catch {
    return { ...NOTICE_DEFAULTS }
  }
}

export function keepNotices(settings: NoticeSettings) {
  try {
    localStorage.setItem(KEY, JSON.stringify(settings))
  } catch {
    // Not kept: the next visit starts as the defaults are.
  }
}

// Whether a choice of how the owner is told is kept, as the browser stands
// once asked: one that shows the system's notifications only when it
// allows them, as T3 Code's, and otherwise not, with what to do instead.
export function noticeChoice(mode: NoticeMode, allowed: "default" | "denied" | "granted" | "none"): { keep: true } | { keep: false; why: string } {
  if ((mode !== "on" && mode !== "both") || allowed === "granted") return { keep: true }
  if (allowed === "none") return { keep: false, why: "This browser shows them only for a page at a secure address, such as this computer's own. Sound only is still there." }
  return { keep: false, why: "Allow notifications in your browser or system settings, then choose this again. Sound only is still there." }
}
