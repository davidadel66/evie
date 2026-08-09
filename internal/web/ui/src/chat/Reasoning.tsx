// The thinking block. While reasoning streams it's a card that stays out of
// the way of nothing; once done it sheds the chrome and becomes a bare
// one-liner — "Thought for 4s" — that unfolds the raw text on click. The
// body is never markdown: thinking is a scratchpad, and half-finished lists
// would jump around through a renderer. Provider whitespace is collapsed at
// render time: some reasoning streams separate token fragments with newlines,
// which pre-wrap turns into a one-word-per-line column.

import { useState } from "react";
import { ChevronDown } from "../ui/Icon";
import type { Item } from "../store/reducer";

type Props = { item: Extract<Item, { kind: "reasoning" }> };

export function Reasoning({ item }: Props) {
  // null means David hasn't touched it: follow the stream (open live, closed
  // when done). An explicit click sticks and is never overridden.
  const [manual, setManual] = useState<boolean | null>(null);
  const open = manual ?? item.streaming;

  const header = (
    <div
      onClick={() => setManual(!open)}
      className="flex cursor-pointer items-center gap-2 px-[10px] py-[5px]"
    >
      <span
        className="text-ghost transition-transform duration-150"
        style={{ transform: open ? "rotate(180deg)" : undefined }}
      >
        <ChevronDown size={12} />
      </span>
      <span className="text-muted-text font-sans text-[11.5px]">
        {item.streaming ? "Thinking…" : `Thought for ${formatDuration(item.ms)}`}
      </span>
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
