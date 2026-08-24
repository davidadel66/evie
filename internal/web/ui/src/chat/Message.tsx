// The two prose shapes: David's message is a bubble pinned right with the
// design's asymmetric corner; Evie's is bare markdown pinned left — no bubble,
// so long answers read as a document rather than a chat balloon.

import { Markdown } from "./Markdown";
import type { DiscardReason } from "../store/events";

export function UserMessage({ text }: { text: string }) {
  return (
    <div className="bg-bubble border-hair-bubble text-[#d7dcd9] max-w-[62%] flex-none self-end rounded-[10px_10px_3px_10px] border px-[14px] py-[10px] text-[length:var(--chat-text-size)] leading-[1.55] whitespace-pre-wrap">
      {text}
    </div>
  );
}

export function AssistantMessage({
  text,
  streaming,
  discarded,
}: {
  text: string;
  streaming: boolean;
  discarded?: { reason: DiscardReason; message: string };
}) {
  return (
    <div className="min-w-0 max-w-[min(720px,100%)] flex-none self-start">
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
