// The two prose shapes: David's message is a bubble pinned right with the
// design's asymmetric corner; Evie's is bare markdown pinned left — no bubble,
// so long answers read as a document rather than a chat balloon.

import { Markdown } from "./Markdown";
import type { DiscardReason } from "../store/events";
import { clockTime } from "../ui/time";

/** A user turn: the bubble, with the time it was sent beneath when known.
 *  The bubble's text is body color, not brighter than Evie's answer, so the
 *  question does not outrank the reply. A turn after the first sits well
 *  clear of the answer above it, which is where a long session lost its
 *  landmarks (ui-chat.spec.md readability amendment). A message sent with a
 *  skill shows its /name above the text (skills.decisions.md, 2026-11-03). */
export function UserMessage({ text, sentAt, first = true, skill }: { text: string; sentAt?: number; first?: boolean; skill?: string }) {
  return (
    <div className={`${first ? "" : "mt-9 "}flex flex-none flex-col items-end gap-[5px]`}>
      <div className="evie-user-message bg-bubble border-hair-bubble text-body max-w-[76%] rounded-[12px_12px_4px_12px] border px-[14px] py-[10px] text-[length:var(--chat-text-size)] leading-[1.55] whitespace-pre-wrap">
        {skill && <div className="text-teal mb-1 font-mono text-[12px]">{`/${skill}`}</div>}
        {text}
      </div>
      {sentAt !== undefined && <span className="text-ghost pr-1 text-[11px] tabular-nums">{clockTime(sentAt)}</span>}
    </div>
  );
}

export function AssistantMessage({
  text,
  streaming,
  discarded,
  quiet,
}: {
  text: string;
  streaming: boolean;
  discarded?: { reason: DiscardReason; message: string };
  /** Settled progress text inside the activity trace: muted log size, not answer size. */
  quiet?: boolean;
}) {
  return (
    <div className={`${quiet ? "evie-quiet " : ""}min-w-0 max-w-[min(720px,100%)] flex-none self-start`}>
      <Markdown text={text} streaming={streaming} />
      {streaming && <Caret />}
      {discarded && <DiscardWarning message={discarded.message} />}
    </div>
  );
}

export function DiscardWarning({ message }: { message: string }) {
  return (
    <div className="mt-2 rounded border border-[#4a2a2e] bg-[#161113] px-3 py-2 text-xs text-[#d9a0a0]" role="status">
      {message}
    </div>
  );
}

/** The design's blinking block caret, marking where Evie is still writing. */
function Caret() {
  return (
    <span
      className="bg-teal ml-[2px] inline-block h-[15px] w-[8px] align-[-2px]"
      style={{ animation: "blink 1s step-end infinite" }}
    />
  );
}
