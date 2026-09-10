// Provider reasoning activity can arrive without public text. Keep its timer
// visible, and offer expansion only when text is available (Astra supplies a
// public summary). Plain text avoids layout jumps from partial markdown.

import { useState } from "react";
import { ChevronDown } from "../ui/Icon";
import type { Item } from "../store/reducer";

type Props = { item: Extract<Item, { kind: "reasoning" }> };

export function Reasoning({ item }: Props) {
  // null means David hasn't touched it: follow the stream (open live, closed
  // when done). An explicit click sticks and is never overridden.
  const [manual, setManual] = useState<boolean | null>(null);
  const summary = item.text.replace(/\s+/g, " ").trim();
  const hasText = summary.length > 0;
  const open = hasText && (manual ?? item.streaming);
  const label = item.streaming ? "Thinking…" : `Thought for ${formatDuration(item.ms)}`;
  const durationHint = "Elapsed wait for response text, measured in this browser. Includes provider and network time.";

  const header = hasText ? (
    <button
      type="button"
      aria-expanded={open}
      title={durationHint}
      onClick={() => setManual(!open)}
      className="flex w-full min-w-0 cursor-pointer items-center gap-2 px-[10px] py-[5px] text-left"
    >
      <span
        className="text-ghost shrink-0 transition-transform duration-150"
        style={{ transform: open ? "rotate(180deg)" : undefined }}
      >
        <ChevronDown size={12} />
      </span>
      <span className="text-muted-text min-w-0 truncate font-sans text-[11.5px]">
        {item.streaming ? label : summary}
      </span>
      {!item.streaming && (
        <span className="text-muted-text shrink-0 font-sans text-[11.5px]">
          {`- ${formatDuration(item.ms)}`}
        </span>
      )}
    </button>
  ) : (
    <div title={durationHint} className="text-muted-text px-[10px] py-[5px] font-sans text-[11.5px]">
      {label}
    </div>
  );

  const body = open && (
    <div className="text-faint px-[14px] pt-[2px] pb-[8px] font-mono text-[11.5px] leading-[1.75] whitespace-normal">
      {item.text}
    </div>
  );

  // Live: the card. Done: just the line (and the text, if he opened it).
  // self-stretch, not self-start: shrink-to-fit lets the browser pick a
  // fit-content width from the pre-wrap text's soft-wrap points, which can
  // collapse the block to a sliver. Stretch to the column, capped at 720px,
  // the same pattern as ToolCard.
  if (!item.streaming) {
    return (
      <div className="min-w-0 max-w-[min(720px,100%)] flex-none self-stretch">
        {header}
        {body}
      </div>
    );
  }

  return (
    <div className="border-hair-strong bg-card min-w-0 max-w-[min(720px,100%)] flex-none self-stretch overflow-hidden rounded-[7px] border">
      {header}
      {body}
    </div>
  );
}

function formatDuration(ms?: number): string {
  if (ms === undefined) return "0s";
  const s = Math.round(ms / 1000);
  return s < 1 ? "<1s" : `${s}s`;
}
