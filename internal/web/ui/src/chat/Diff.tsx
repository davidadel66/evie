// The diff shown before an edit_file runs. The approval payload carries only
// old_string / new_string — never the file — so absolute line numbers are
// unknowable and the design's 14/15/16 gutter would be invented. A −/+ gutter
// is the honest version of the same shape.

import { diffLines } from "diff";

type Props = { oldText: string; newText: string };

export function Diff({ oldText, newText }: Props) {
  const rows = toRows(oldText, newText);

  return (
    <div className="py-[10px] font-mono text-[11.5px] leading-[1.7]">
      {rows.map((row, i) => (
        <div
          key={i}
          className="flex"
          style={{ background: rowBackground(row.sign) }}
        >
          <span
            className="w-[38px] flex-none pr-[10px] text-right"
            style={{ color: gutterColor(row.sign) }}
          >
            {row.sign}
          </span>
          <span
            className="whitespace-pre-wrap"
            style={{ color: textColor(row.sign) }}
          >
            {row.text === "" ? " " : row.text}
          </span>
        </div>
      ))}
    </div>
  );
}

type Sign = "-" | "+" | " ";
type Row = { sign: Sign; text: string };

/** toRows flattens jsdiff's per-change chunks into one row per line. A change
 *  chunk holds many lines, and each needs its own gutter cell. */
function toRows(oldText: string, newText: string): Row[] {
  const rows: Row[] = [];
  for (const part of diffLines(oldText, newText)) {
    const sign: Sign = part.added ? "+" : part.removed ? "-" : " ";
    // A trailing newline yields an empty final element; drop it so the diff
    // doesn't end in a phantom blank row.
    const lines = part.value.split("\n");
    if (lines.at(-1) === "") lines.pop();
    for (const text of lines) rows.push({ sign, text });
  }
  return rows;
}

// Colours lifted from the design's approval card.
function rowBackground(sign: Sign): string | undefined {
  if (sign === "-") return "rgba(217,107,107,.08)";
  if (sign === "+") return "rgba(95,174,125,.09)";
  return undefined;
}

function gutterColor(sign: Sign): string {
  if (sign === "-") return "var(--color-danger-gutter)";
  if (sign === "+") return "var(--color-ok-gutter)";
  return "var(--color-ghost)";
}

function textColor(sign: Sign): string {
  if (sign === "-") return "var(--color-danger-ink)";
  if (sign === "+") return "var(--color-ok-ink)";
  return "var(--color-muted)";
}
