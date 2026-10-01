// The composer. Enter sends, Shift+Enter newlines. It stays live while Evie
// responds so a message can join the queue, but a Context Scope transition
// disables it until the server and displayed scope agree. While a turn
// streams it also offers Stop, and composer command results report below it.

import { useEffect, useRef, type ReactNode } from "react";
import type { ComposerNotice } from "../api/turnControls";
import { ArrowUp, StopSquare } from "../ui/Icon";

type Props = {
  value: string;
  onChange: (v: string) => void;
  onSend: () => void;
  streaming: boolean;
  disabled?: boolean;
  modelSelector?: ReactNode;
  /** Stops the running turn; offered only while streaming. */
  onStop?: () => void;
  /** A stop request is in flight; the turn's stream reports when it ended. */
  stopping?: boolean;
  /** The latest composer command's result, such as a compaction outcome. */
  notice?: ComposerNotice | null;
};

export function Composer({ value, onChange, onSend, streaming, disabled = false, modelSelector, onStop, stopping = false, notice }: Props) {
  const ref = useRef<HTMLTextAreaElement>(null);

  // Grow with the content instead of scrolling inside one row. Reset to auto
  // first so deleting a line shrinks it back.
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
  }, [value]);

  return (
    <div className="flex-none px-7 pt-[14px] pb-[18px]">
      <div
        className={`flex items-end gap-[10px] rounded-[10px] border py-[11px] pr-3 pl-4 ${
          streaming
            ? "border-hair-strong bg-[#101416]"
            : "border-hair-input bg-[#12171a]"
        }`}
      >
        <textarea
          ref={ref}
          rows={1}
          value={value}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => {
            if (disabled) return;
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              onSend();
            }
          }}
          placeholder={disabled ? "Updating chat…" : streaming ? "Queue a message…" : "Message Evie…"}
          className="text-ink placeholder:text-fainter flex-1 resize-none border-none bg-transparent py-1 font-sans text-[length:var(--chat-text-size)] leading-[1.5]"
        />
        {streaming && onStop && (
          <button
            type="button"
            onClick={onStop}
            disabled={stopping}
            aria-label={stopping ? "Stopping turn" : "Stop turn"}
            title="Stop this turn. Work already saved stays in the conversation."
            className="border-hair-input text-body hover:text-ink focus-visible:outline-teal flex h-[30px] flex-none cursor-pointer items-center gap-[6px] rounded-[7px] border px-[10px] text-xs focus-visible:outline-2 focus-visible:outline-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <StopSquare size={11} />
            {stopping ? "Stopping…" : "Stop"}
          </button>
        )}
        <div
          onClick={disabled ? undefined : onSend}
          aria-disabled={disabled}
          className={`bg-teal flex h-[30px] w-[30px] flex-none items-center justify-center rounded-[7px] ${
            disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer"
          }`}
        >
          <ArrowUp size={14} stroke="#0a0c0d" />
        </div>
      </div>
      {notice && (
        <p role="status" className={`mt-2 text-xs ${notice.tone === "warning" ? "text-amber-ink" : "text-muted-text"}`}>
          {notice.text}
        </p>
      )}
      {modelSelector}
    </div>
  );
}
