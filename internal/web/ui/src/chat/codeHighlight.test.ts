import { describe, expect, it } from "vitest";
import type { HighlightResult } from "@streamdown/code";
import { code, evieCodeTheme, exactHighlightTokens } from "./codeHighlight";

function highlight(source: string): Promise<HighlightResult> {
  return new Promise((resolve) => {
    const result = code.highlight({code: source, language: "go", themes: [evieCodeTheme, evieCodeTheme]}, resolve);
    if (result) resolve(result);
  });
}

describe("exact file highlighting", () => {
  it("rejects cached tokens belonging to an equal-length interior edit", async () => {
    const prefix = "//" + "prefix ".repeat(30) + "\n";
    const suffix = "\n//" + "suffix ".repeat(30);
    const before = prefix + 'const value = "FIRST"' + suffix;
    const after = prefix + 'const value = "OTHER"' + suffix;
    const initial = await highlight(before);
    expect(exactHighlightTokens(initial, before)).toBeDefined();
    const next = await highlight(after);
    const accepted = exactHighlightTokens(next, after);
    // The installed dependency currently returns a sampled-cache collision;
    // this remains valid if its cache is later fixed.
    expect(accepted ? accepted.flat().map(token => token.content).join("") : after).toContain("OTHER");
    expect(exactHighlightTokens(initial, after)).toBeUndefined();
  });
});
