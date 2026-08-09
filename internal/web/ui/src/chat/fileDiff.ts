export type DiffKind = "same" | "removed" | "added" | "spacer";
export type DiffSection = {
  startLine: number;
  lines: string[];
  kind: DiffKind;
};
export type DiffPane = {
  sections: DiffSection[];
  missingFinalNewline: boolean;
};

/** edit_file performs exactly one contiguous replacement, so a common
 * line-prefix/suffix split identifies its changed block in linear time. That
 * avoids a general diff's pathological runtime while still rendering every
 * byte of both complete files. */
export function fullFileDiff(beforeText: string, afterText: string): {
  before: DiffPane;
  after: DiffPane;
} {
  const before = splitFile(beforeText);
  const after = splitFile(afterText);
  let prefix = 0;
  while (
    prefix < before.lines.length &&
    prefix < after.lines.length &&
    before.lines[prefix] === after.lines[prefix]
  ) {
    prefix += 1;
  }

  let suffix = 0;
  while (
    suffix < before.lines.length - prefix &&
    suffix < after.lines.length - prefix &&
    before.lines[before.lines.length - 1 - suffix] ===
      after.lines[after.lines.length - 1 - suffix]
  ) {
    suffix += 1;
  }

  return {
    before: {
      sections: sections(
        before.lines,
        prefix,
        suffix,
        "removed",
        Math.max(0, after.lines.length - before.lines.length),
      ),
      missingFinalNewline: before.missingFinalNewline,
    },
    after: {
      sections: sections(
        after.lines,
        prefix,
        suffix,
        "added",
        Math.max(0, before.lines.length - after.lines.length),
      ),
      missingFinalNewline: after.missingFinalNewline,
    },
  };
}

export function newFilePane(text: string): DiffPane {
  const file = splitFile(text);
  return {
    sections:
      file.lines.length === 0
        ? []
        : [{ startLine: 1, lines: file.lines, kind: "added" }],
    missingFinalNewline: file.missingFinalNewline,
  };
}

function sections(
  lines: string[],
  prefix: number,
  suffix: number,
  changedKind: "removed" | "added",
  spacerLines: number,
): DiffSection[] {
  const out: DiffSection[] = [];
  if (prefix > 0) {
    out.push({ startLine: 1, lines: lines.slice(0, prefix), kind: "same" });
  }

  const changedEnd = lines.length - suffix;
  if (changedEnd > prefix) {
    out.push({
      startLine: prefix + 1,
      lines: lines.slice(prefix, changedEnd),
      kind: changedKind,
    });
  }

  if (spacerLines > 0) {
    out.push({
      startLine: changedEnd + 1,
      lines: Array.from({ length: spacerLines }, () => ""),
      kind: "spacer",
    });
  }

  if (suffix > 0) {
    out.push({
      startLine: changedEnd + 1,
      lines: lines.slice(changedEnd),
      kind: "same",
    });
  }
  return out;
}

function splitFile(text: string): {
  lines: string[];
  missingFinalNewline: boolean;
} {
  if (text === "") return { lines: [], missingFinalNewline: false };
  const hasFinalNewline = text.endsWith("\n");
  const lines = text.split("\n");
  if (hasFinalNewline) lines.pop();
  return { lines, missingFinalNewline: !hasFinalNewline };
}
