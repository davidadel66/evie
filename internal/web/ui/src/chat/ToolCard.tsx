// A tool call as the design draws it: one dense mono row that expands to the
// raw result. Errors get the red variant and open themselves — a failure you
// have to click to see is a failure you miss.

import { useState } from "react";
import { Alert, ChevronDown, Wrench } from "../ui/Icon";
import type { Item } from "../store/reducer";

type Props = { tool: Extract<Item, { kind: "tool" }> };

export function ToolCard({ tool }: Props) {
  const failed = tool.isErr === true;
  const [open, setOpen] = useState(failed);
  const running = tool.result === undefined;

  return (
    <div
      className={`min-w-0 max-w-[min(720px,100%)] flex-none self-stretch overflow-hidden rounded-lg border ${
        failed ? "border-danger-hair bg-danger-bg" : "border-hair-strong bg-card"
      }`}
    >
      <div
        onClick={() => setOpen(!open)}
        className="flex cursor-pointer items-center gap-[10px] px-3 py-2"
      >
        {failed ? (
          <Alert stroke="var(--color-danger)" />
        ) : (
          <Wrench stroke="var(--color-muted-text)" />
        )}
        <span className="text-body-dim font-mono text-xs">{tool.name}</span>
        <span className="text-fainter min-w-0 flex-1 truncate font-mono text-[11.5px]">
          {preview(tool.args)}
        </span>
        <StatusChip failed={failed} running={running} ms={tool.ms} />
        <span
          className="text-ghost transition-transform duration-150"
          style={{ transform: open ? "rotate(180deg)" : undefined }}
        >
          <ChevronDown size={12} />
        </span>
      </div>

      {open && tool.result !== undefined && (
        <pre
          className={`m-0 overflow-x-auto border-t px-[14px] py-[10px] font-mono text-[11.5px] leading-[1.6] ${
            failed
              ? "border-[#2c1a1e] bg-[#120d0f] text-[#c98f8f]"
              : "border-[#1b2124] bg-[#0e1213] text-muted-text"
          }`}
        >
          {tool.result}
        </pre>
      )}
    </div>
  );
}

function StatusChip({
  failed,
  running,
  ms,
}: {
  failed: boolean;
  running: boolean;
  ms?: number;
}) {
  if (running) {
    return (
      <span className="text-amber font-mono text-[10.5px] whitespace-nowrap">
        running…
      </span>
    );
  }
  if (failed) {
    return (
      <span className="text-danger rounded bg-[rgba(217,107,107,.1)] px-[6px] py-[1px] font-mono text-[10.5px]">
        failed
      </span>
    );
  }
  // The design showed "✓ 412 rows · 38ms"; row counts need structured tool
  // metadata the registry doesn't return, so only the honest half ships.
  return (
    <span className="text-ok font-mono text-[10.5px] whitespace-nowrap">
      ✓ {formatMs(ms)}
    </span>
  );
}

/** preview collapses args to one line. Tool args are JSON, but a malformed or
 *  huge blob must still degrade to something readable, not break the row. */
function preview(args: string): string {
  const flat = args.replace(/\s+/g, " ").trim();
  return flat.length > 160 ? flat.slice(0, 160) + "…" : flat;
}

function formatMs(ms?: number): string {
  if (ms === undefined) return "";
  return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`;
}
