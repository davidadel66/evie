import type { SessionSummary, TurnChanges } from "../../src/core/types.ts"
import { addressOf } from "./address.ts"
import type { ConversationState } from "./conversation.ts"
import { asAsked } from "./questions.ts"
import { titleOf } from "./session-list.ts"
import { transcriptOf, type Call, type Item } from "./transcript.ts"
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
// run of work, a note or the files a turn changed; or why a turn failed.
export type ChatLine = { key: string } & (
  | { kind: "owner"; text: string; files: string[] }
  | { kind: "text"; text: string }
  | { kind: "step"; text: string }
  | { kind: "failed"; text: string }
)

// A conversation's items as the side chat shows them, in short: a finished
// turn's work before its final reply is one line, of the tools it used,
// and so is each run of calls; evie's words between them are left out, as
// are a question still waiting, which the chat asks at its end, and the
// line that says evie is working, which the chat says at its end too.
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
        return item.calls.length === 0 ? [] : [{ key, kind: "step", text: used(item.calls) }]
      case "fold": {
        const calls = item.items.flatMap((inside) => (inside.kind === "steps" ? inside.calls : []))
        return [{ key, kind: "step", text: calls.length === 0 ? item.label : used(calls) }]
      }
      case "note":
        return [{ key, kind: "step", text: item.text }]
      case "changes":
        return [{ key, kind: "step", text: `Changed ${item.files.length} ${item.files.length === 1 ? "file" : "files"}` }]
      case "failed":
        return [{ key, kind: "failed", text: item.error }]
    }
  })
}

// The tools a run of calls used, each named once: "Used read, edit and shell".
function used(calls: readonly Call[]): string {
  const names = [...new Set(calls.map((call) => call.name))]
  return `Used ${names.length <= 1 ? (names[0] ?? "") : `${names.slice(0, -1).join(", ")} and ${names.at(-1)}`}`
}

// What the side chat shows of a conversation as it stands, with the turns
// that changed files: its lines, a question still waiting left out, since
// the chat asks it at its end.
export function sideChatOf(state: ConversationState, changes: readonly TurnChanges[] = []): ChatLine[] {
  return chatLines(asAsked(transcriptOf(state, changes), state.waiting))
}
