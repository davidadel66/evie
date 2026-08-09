import { describe, expect, it } from "vitest";
import { fullFileDiff, newFilePane } from "./fileDiff";

describe("fullFileDiff", () => {
  it("splits complete files into context and one changed block", () => {
    expect(fullFileDiff("alpha\nbeta\ngamma\n", "alpha\ndelta\ngamma\n")).toEqual({
      before: {
        sections: [
          { startLine: 1, lines: ["alpha"], kind: "same" },
          { startLine: 2, lines: ["beta"], kind: "removed" },
          { startLine: 3, lines: ["gamma"], kind: "same" },
        ],
        missingFinalNewline: false,
      },
      after: {
        sections: [
          { startLine: 1, lines: ["alpha"], kind: "same" },
          { startLine: 2, lines: ["delta"], kind: "added" },
          { startLine: 3, lines: ["gamma"], kind: "same" },
        ],
        missingFinalNewline: false,
      },
    });
  });

  it("pads unequal replacement sides so unchanged suffixes stay aligned", () => {
    const result = fullFileDiff("a\nb\n", "a\nx\ny\nb\n");
    expect(result.before.sections[1]).toEqual({
      startLine: 2,
      lines: ["", ""],
      kind: "spacer",
    });
    expect(result.before.sections[2]).toEqual({
      startLine: 2,
      lines: ["b"],
      kind: "same",
    });
    expect(result.after.sections[1]).toEqual({
      startLine: 2,
      lines: ["x", "y"],
      kind: "added",
    });
    expect(result.after.sections[2]).toEqual({
      startLine: 4,
      lines: ["b"],
      kind: "same",
    });
  });

  it("surfaces a final-newline-only change", () => {
    const result = fullFileDiff("a\n", "a");
    expect(result.before.missingFinalNewline).toBe(false);
    expect(result.after.missingFinalNewline).toBe(true);
  });

  it("keeps an all-lines-changed large file to one DOM section per pane", () => {
    const before = Array.from({ length: 20_000 }, (_, i) => `before ${i}`).join("\n");
    const after = Array.from({ length: 20_000 }, (_, i) => `after ${i}`).join("\n");
    const result = fullFileDiff(before, after);
    expect(result.before.sections).toHaveLength(1);
    expect(result.after.sections).toHaveLength(1);
  });
});

describe("newFilePane", () => {
  it("renders one complete added section with real line numbers", () => {
    expect(newFilePane("one\ntwo\n")).toEqual({
      sections: [{ startLine: 1, lines: ["one", "two"], kind: "added" }],
      missingFinalNewline: false,
    });
  });
});
