import { describe, expect, it } from "vitest";
import { composerCommand } from "./composerCommand";

describe("composerCommand", () => {
  it("recognizes exact /compact, ignoring surrounding whitespace", () => {
    expect(composerCommand("/compact")).toEqual({ kind: "compact" });
    expect(composerCommand("  /compact\n")).toEqual({ kind: "compact" });
  });

  it("rejects arguments locally like the REPL", () => {
    expect(composerCommand("/compact now")).toEqual({ kind: "usage", message: "Usage: /compact" });
    expect(composerCommand("/compact\tplease")).toEqual({ kind: "usage", message: "Usage: /compact" });
  });

  it("leaves everything else as an ordinary message", () => {
    for (const text of ["//compact", "/compactor", "please /compact", "compact", "/context", ""]) {
      expect(composerCommand(text)).toBeNull();
    }
  });
});
