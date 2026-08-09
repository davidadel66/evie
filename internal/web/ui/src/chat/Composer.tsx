// The composer. Enter sends, Shift+Enter newlines. Never disabled: a message
// sent mid-turn joins the queue (useSession) and fires when the turn ends,
// so the bar stays live while Evie responds.

import { useEffect, useRef } from "react";
import { ArrowUp } from "../ui/Icon";

type Props = {
  value: string;
  onChange: (v: string) => void;
  onSend: () => void;
  streaming: boolean;
};

export function Composer({ value, onChange, onSend, streaming }: Props) {
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
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              onSend();
            }
          }}
          placeholder={streaming ? "Queue a message…" : "Message Evie…"}
          className="text-ink placeholder:text-fainter flex-1 resize-none border-none bg-transparent py-1 font-sans text-[length:var(--chat-text-size)] leading-[1.5]"
        />
        <div
          onClick={onSend}
          className="bg-teal flex h-[30px] w-[30px] flex-none cursor-pointer items-center justify-center rounded-[7px]"
        >
          <ArrowUp size={14} stroke="#0a0c0d" />
        </div>
      </div>
    </div>
  );
}
