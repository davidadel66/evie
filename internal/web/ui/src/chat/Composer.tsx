// The composer. Enter sends, Shift+Enter newlines. It stays live while Evie
// responds so a message can join the queue, but a Context Scope transition
// disables it until the server and displayed scope agree. While a turn
// streams it also offers Stop, and composer command results report below it.
// It shares the transcript's centered column. A bare "/" prefix opens the
// command menu; popovers such as the model picker float above the box.

import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import type { ComposerNotice } from "../api/turnControls";
import { ArrowUp, StopSquare } from "../ui/Icon";
import { slashMatches } from "./composerCommand";

type Props = {
  value: string;
  onChange: (v: string) => void;
  onSend: () => void;
  /** Runs a command chosen from the "/" menu, as if it were sent exactly. */
  onCommand?: (command: string) => void;
  streaming: boolean;
  disabled?: boolean;
  /** A popover anchored above the box, such as the model picker. */
  overlay?: ReactNode;
  /** Status and recovery lines below the box. */
  after?: ReactNode;
  /** Stops the running turn; offered only while streaming. */
  onStop?: () => void;
  /** A stop request is in flight; the turn's stream reports when it ended. */
  stopping?: boolean;
  /** The latest composer command's result, such as a compaction outcome. */
  notice?: ComposerNotice | null;
};

export function Composer({ value, onChange, onSend, onCommand, streaming, disabled = false, overlay, after, onStop, stopping = false, notice }: Props) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const menuID = useId();
  // Highlight and dismissal belong to the draft they were set for, so any edit
  // reopens the menu at its first match.
  const [menu, setMenu] = useState({ draft: value, index: 0, dismissed: false });
  const matches = disabled || overlay ? [] : slashMatches(value);
  const current = menu.draft === value ? menu : { draft: value, index: 0, dismissed: false };
  const menuOpen = matches.length > 0 && !current.dismissed;
  const active = Math.min(current.index, matches.length - 1);
  const run = (command: string) => (onCommand ? onCommand(command) : onChange(command));

  // Grow with the content instead of scrolling inside one row. Reset to auto
  // first so deleting a line shrinks it back.
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
  }, [value]);

  return (
    <div className="flex-none px-4 pt-3 pb-4 sm:px-7">
      <div className="mx-auto w-full max-w-[768px]">
        <div className="relative">
          {overlay}
          {menuOpen && (
            <div id={menuID} role="listbox" aria-label="Commands" className="bg-card border-hair-input absolute bottom-full left-0 z-30 mb-2 w-[min(360px,100%)] overflow-hidden rounded-xl border p-1.5 shadow-[0_12px_32px_rgba(0,0,0,0.45)]">
              {matches.map((command, i) => (
                <div
                  key={command.name}
                  id={`${menuID}-${i}`}
                  role="option"
                  aria-selected={i === active}
                  onMouseDown={(e) => e.preventDefault()}
                  onMouseEnter={() => setMenu({ draft: value, index: i, dismissed: false })}
                  onClick={() => run(command.name)}
                  className={`${i === active ? "bg-hover" : ""} flex cursor-pointer items-baseline gap-3 rounded-md px-2 py-1.5 text-[13px]`}
                >
                  <span className="text-ink font-mono">{command.name}</span>
                  <span className="text-muted-text min-w-0 truncate">{command.description}</span>
                </div>
              ))}
            </div>
          )}
          <div
            className={`flex items-end gap-[10px] rounded-[18px] border py-[11px] pr-3 pl-4 ${
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
              aria-autocomplete="list"
              aria-controls={menuOpen ? menuID : undefined}
              aria-activedescendant={menuOpen ? `${menuID}-${active}` : undefined}
              onChange={(e) => onChange(e.target.value)}
              onKeyDown={(e) => {
                if (disabled) return;
                if (menuOpen) {
                  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                    e.preventDefault();
                    const step = e.key === "ArrowDown" ? 1 : -1;
                    setMenu({ draft: value, index: (active + step + matches.length) % matches.length, dismissed: false });
                    return;
                  }
                  if (e.key === "Escape") {
                    e.preventDefault();
                    setMenu({ draft: value, index: active, dismissed: true });
                    return;
                  }
                  if (e.key === "Tab") {
                    e.preventDefault();
                    onChange(matches[active].name);
                    return;
                  }
                  if (e.key === "Enter" && !e.shiftKey) {
                    e.preventDefault();
                    run(matches[active].name);
                    return;
                  }
                }
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
              className={`bg-teal flex h-[30px] w-[30px] flex-none items-center justify-center rounded-full ${
                disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer"
              }`}
            >
              <ArrowUp size={14} stroke="#0a0c0d" />
            </div>
          </div>
        </div>
        {notice && (
          <p role="status" className={`mt-2 text-xs ${notice.tone === "warning" ? "text-amber-ink" : "text-muted-text"}`}>
            {notice.text}
          </p>
        )}
        {after}
      </div>
    </div>
  );
}
