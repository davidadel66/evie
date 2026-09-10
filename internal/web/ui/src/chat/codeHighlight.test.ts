import { describe, expect, it } from "vitest";
import type { HighlightResult } from "@streamdown/code";
import { code, evieCodeTheme, exactHighlightTokens, fileCodeTheme } from "./codeHighlight";

function highlight(source: string): Promise<HighlightResult> {
  return new Promise((resolve) => {
    const result = code.highlight({code: source, language: "go", themes: [evieCodeTheme, evieCodeTheme]}, resolve);
    if (result) resolve(result);
  });
}

describe("exact file highlighting", () => {
  it("colors file source with the requested editor palette without changing its text", async () => {
    const source = 'package main\nimport "context"\nfunc Hello() string { return "hello" }';
    const result = await new Promise<HighlightResult>((resolve) => {
      const immediate = code.highlight({code: source, language: "go", themes: [fileCodeTheme, fileCodeTheme]}, resolve);
      if (immediate) resolve(immediate);
    });
    const tokens = exactHighlightTokens(result, source);
    expect(tokens).toBeDefined();
    const colors = tokens!.flat().map(token => token.htmlStyle?.color ?? token.color);
    expect(colors).toContain("#F47076");
    expect(colors).toContain("#C586FF");
    expect(colors).toContain("#85D77A");
    expect(tokens!.flat().find(token => token.content.includes("context"))?.htmlStyle?.color).toBe("#85D77A");
  });

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
