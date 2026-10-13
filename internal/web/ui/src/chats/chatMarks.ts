import type { ChatStatus } from "../api/chatStatus";

// Sidebar markers (multi-chat.spec.md Stage 4): a spinner while a chat runs a
// turn, amber while it waits on an approval, and a bold title when its latest
// turn finished after it was last viewed. Read state lives in this browser
// only; turns that finished before it was first recorded never count.

export type ChatMark = { running: boolean; waiting: boolean; unread: boolean };
export type ReadState = { baseline: number; read: Record<string, number> };

const readKey = "evie.chatRead";

export function loadReadState(now: number): ReadState {
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(readKey) ?? "null");
    if (stored && typeof stored === "object" && typeof (stored as ReadState).baseline === "number") {
      const read = (stored as ReadState).read;
      return { baseline: (stored as ReadState).baseline, read: read && typeof read === "object" ? read : {} };
    }
  } catch {
    // Unavailable storage starts fresh; nothing is unread yet.
  }
  const fresh = { baseline: now, read: {} };
  saveReadState(fresh);
  return fresh;
}

export function markRead(state: ReadState, id: string, at: number): ReadState {
  if ((state.read[id] ?? 0) >= at) return state;
  const next = { ...state, read: { ...state.read, [id]: at } };
  saveReadState(next);
  return next;
}

function saveReadState(state: ReadState) {
  try {
    localStorage.setItem(readKey, JSON.stringify(state));
  } catch {
    // Read state is a convenience; without storage it lasts for the page.
  }
}

/** chatMark combines a chat's live status, its last recorded activity and
 *  when it was last viewed. */
export function chatMark(status: ChatStatus | undefined, activityAt: string | undefined, read: ReadState, id: string): ChatMark {
  const running = !!status?.running;
  const activity = Math.max(status?.finishedAt ?? 0, activityAt ? Date.parse(activityAt) || 0 : 0);
  return {
    running,
    waiting: (status?.waiting ?? 0) > 0,
    unread: !running && activity > (read.read[id] ?? read.baseline),
  };
}

/** combineMarks summarizes chats a collapsed or shortened list hides. */
export function combineMarks(marks: ChatMark[]): ChatMark {
  return {
    running: marks.some((mark) => mark.running),
    waiting: marks.some((mark) => mark.waiting),
    unread: marks.some((mark) => mark.unread),
  };
}

/** The words a screen reader hears after a chat's title. */
export function markText(mark: ChatMark): string {
  return [mark.running && "working", mark.waiting && "waiting for your approval", mark.unread && "new reply"].filter(Boolean).join(", ");
}
