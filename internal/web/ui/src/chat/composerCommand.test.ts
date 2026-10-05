import { describe, expect, it } from "vitest";
import { composerCommand, slashMatches } from "./composerCommand";

describe("composerCommand", () => {
  it("recognizes exact /compact, ignoring surrounding whitespace", () => {
    expect(composerCommand("/compact")).toEqual({ kind: "compact" });
    expect(composerCommand("  /compact\n")).toEqual({ kind: "compact" });
  });

  it("rejects arguments locally like the REPL", () => {
    expect(composerCommand("/compact now")).toEqual({ kind: "usage", message: "Usage: /compact" });
    expect(composerCommand("/compact\tplease")).toEqual({ kind: "usage", message: "Usage: /compact" });
  });

  it("recognizes exact /model and rejects arguments", () => {
    expect(composerCommand(" /model\n")).toEqual({ kind: "model" });
    expect(composerCommand("/model openai/gpt")).toEqual({ kind: "usage", message: "Usage: /model" });
  });

  it("leaves everything else as an ordinary message", () => {
    for (const text of ["//compact", "/compactor", "please /compact", "compact", "/context", "/models", "//model", ""]) {
      expect(composerCommand(text)).toBeNull();
    }
  });

  it("offers matching commands only while the draft is a bare slash prefix", () => {
    expect(slashMatches("/").map((command) => command.name)).toEqual(["/model", "/compact"]);
    expect(slashMatches("/co").map((command) => command.name)).toEqual(["/compact"]);
    expect(slashMatches("/model").map((command) => command.name)).toEqual(["/model"]);
    for (const text of ["", "hello", " /model", "/model x", "/model\n", "/x", "//"]) {
      expect(slashMatches(text)).toEqual([]);
    }
  });
});
