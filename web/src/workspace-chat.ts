import type { SessionSummary } from "../../src/core/types.ts"
import { addressOf } from "./address.ts"
import type { ConversationState, GivenBack } from "./conversation.ts"
import { asAsked } from "./questions.ts"
import { titleOf } from "./session-list.ts"
import { runWords } from "./step-words.ts"
import { transcriptOf, type Item } from "./transcript.ts"
import { NEW_CHAT } from "./workspace.ts"

// The workspace's side chat, apart from its screen: a small chat about the
// workspace, beside it. Its header names the chat and opens it full size in
// the session view; it shows the conversation in short, each run of evie's
// work as one quiet line, with a small message box under it.

// What the header says under "evie": the session's title, or for a chat
// not started yet, the workspace it will start in.
export function chatTitle(session: SessionSummary | undefined, workspace: string): string {
  return session === undefined ? `New chat in ${workspace}` : titleOf(session)
}

// Where the header's Open this chat full size goes, for the side chat's
// `chat` as the address keeps it: its session in the session view, or for
// a chat not started yet, a new session there in the workspace's project.
export const fullSize = (chat: string, project: string) => (chat === NEW_CHAT ? addressOf({ draft: project }) : addressOf({ session: chat }))

// One line of the side chat: the owner's message, with the names of the
// files attached to it; evie's words, in Markdown; one quiet line for a
// run of work or a note; or why a turn failed.
export type ChatLine = { key: string } & (
  | { kind: "owner"; text: string; files: string[] }
  | { kind: "text"; text: string }
  | { kind: "step"; text: string }
  | { kind: "failed"; text: string }
)

// A conversation's items as the side chat shows them, in short: a finished
// turn's work before its final reply is one line, in plain words, and so is
// each run of calls; evie's words between them are left out, as are a
// question still waiting, which the chat asks at its end, and the line that
// says evie is working, which the chat says at its end too. Of the notes,
// those written for the model are left out, and so is background work
// starting, which the call that started it says; its end says whether it
// finished.
export function chatLines(items: readonly Item[]): ChatLine[] {
  return items.flatMap((item): ChatLine[] => {
    const { key } = item
    switch (item.kind) {
      case "owner":
        return [{ key, kind: "owner", text: item.text, files: [...(item.attachments ?? []).map((each) => each.name), ...(item.sending ?? []).map((each) => each.name)] }]
      case "reply":
        return [{ key, kind: "text", text: item.text }]
      case "question":
        return item.waiting ? [] : [{ key, kind: "text", text: item.text }]
      case "steps":
        return item.calls.length === 0 ? [] : [{ key, kind: "step", text: runWords(item.calls) }]
      case "fold": {
        const calls = item.items.flatMap((inside) => (inside.kind === "steps" ? inside.calls : []))
        return [{ key, kind: "step", text: calls.length === 0 ? item.label : runWords(calls) }]
      }
      case "note":
        if (item.text.startsWith(NOTE_TO_EVIE)) return []
        if (key.startsWith("job")) return key.endsWith("-end") ? [{ key, kind: "step", text: / failed after /.test(item.text) ? "Background work failed" : "Background work finished" }] : []
        return [{ key, kind: "step", text: item.text }]
      // The run's own line says what it changed, and the chat says at its end that evie is working.
      case "changes":
      case "thinking":
        return []
      case "failed":
        return [{ key, kind: "failed", text: item.error }]
    }
  })
}

// How a note written for the model starts, as the conversation shows it.
const NOTE_TO_EVIE = "Note to evie: "

// What the side chat shows of a conversation as it stands: its lines, a
// question still waiting left out, since the chat asks it at its end, and
// then each command running, such as /compact, until it has run.
export function sideChatOf(state: ConversationState): ChatLine[] {
  const running = state.running.map((name, at): ChatLine => ({ key: `running-${at}`, kind: "step", text: `Running /${name}` }))
  return [...chatLines(asAsked(transcriptOf(state), state.waiting)), ...running]
}

// Why a message sent on the box's behalf, as a new chat's first, came back
// to the box. The small box holds no files, so one that had files says
// they were not kept, and where to attach them again.
export function givenBackWords(back: GivenBack): string {
  if (back.files === undefined || back.files.length === 0) return back.problem
  return `${back.problem} Its attached files were not kept here: open this chat full size to attach them again.`
}
