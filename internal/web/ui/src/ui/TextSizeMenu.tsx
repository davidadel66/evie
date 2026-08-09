import { useEffect, useRef, useState } from "react";
import type { ChatTextSize } from "./textSize";

const options: { value: ChatTextSize; label: string; pixels: number }[] = [
  { value: "compact", label: "Compact", pixels: 13 },
  { value: "default", label: "Default", pixels: 15 },
  { value: "large", label: "Large", pixels: 17 },
];

type Props = {
  value: ChatTextSize;
  onChange: (value: ChatTextSize) => void;
};

export function TextSizeMenu({ value, onChange }: Props) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const closeOutside = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
        trigger.current?.focus();
      }
    };
    window.addEventListener("pointerdown", closeOutside);
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      window.removeEventListener("pointerdown", closeOutside);
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]);

  return (
    <div ref={root} className="relative">
      <button
        ref={trigger}
        type="button"
        aria-expanded={open}
        aria-controls="chat-text-size-options"
        aria-label="Chat text size"
        onClick={() => setOpen(!open)}
        className={`border-hair-strong flex h-7 cursor-pointer items-baseline gap-px rounded-[6px] border bg-transparent px-2 font-sans transition-colors ${
          open ? "text-teal border-teal-hair" : "text-muted-text hover:text-body"
        }`}
      >
        <span className="text-[11px]">A</span>
        <span className="text-[15px]">a</span>
      </button>

      {open && (
        <div
          id="chat-text-size-options"
          role="group"
          aria-label="Chat text size"
          className="border-hair-strong bg-card absolute top-[34px] right-0 z-30 w-[174px] rounded-[8px] border p-[5px] shadow-[0_12px_32px_rgba(0,0,0,.35)]"
        >
          <div className="text-fainter px-2 pt-1 pb-[5px] font-mono text-[10px] uppercase tracking-[.08em]">
            Chat text
          </div>
          {options.map((option) => {
            const selected = option.value === value;
            return (
              <button
                key={option.value}
                type="button"
                aria-pressed={selected}
                onClick={() => {
                  onChange(option.value);
                  setOpen(false);
                  trigger.current?.focus();
                }}
                className={`flex w-full cursor-pointer items-center justify-between rounded-[5px] border-none px-2 py-[7px] font-sans ${
                  selected
                    ? "bg-teal-deep text-ink"
                    : "text-body bg-transparent hover:bg-muted"
                }`}
              >
                <span className="text-[12px] font-medium">{option.label}</span>
                <span className={selected ? "text-teal font-mono text-[10.5px]" : "text-fainter font-mono text-[10.5px]"}>
                  {option.pixels}px
                </span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
