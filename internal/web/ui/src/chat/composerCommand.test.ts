import { describe, expect, it } from "vitest";
import type { ChatSkill } from "../api/skills";
import { composerCommand, draftText, menuChoice, outgoingMessage, slashMenu } from "./composerCommand";

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
    const names = (text: string) => slashMenu(text, []).commands.map((command) => command.name);
    expect(names("/")).toEqual(["/model", "/compact"]);
    expect(names("/co")).toEqual(["/compact"]);
    expect(names("/model")).toEqual(["/model"]);
    for (const text of ["", "hello", " /model", "/model x", "/model\n", "/x", "//"]) {
      expect(slashMenu(text, [])).toEqual({ commands: [], skills: [] });
    }
  });
});

// skills.spec.md, Owner invocation: the "/" menu lists the chat's skills
// after the built-in commands, which win a clash; a draft that starts with
// /name for one of them sends the rest of the text with that skill.
const skills: ChatSkill[] = [
  { name: "grilling", description: "Grill the owner about a plan.", ownerOnly: false },
  { name: "handoff", description: "Write a handoff prompt.", argumentHint: "[next phase]", ownerOnly: false },
  { name: "owl", description: "Night shift checklist.", ownerOnly: true },
  { name: "model", description: "A skill that clashes with /model.", ownerOnly: false },
];

describe("slash menu with skills", () => {
  it("lists the chat's skills after the commands, and a built-in wins a clash", () => {
    const menu = slashMenu("/", skills);
    expect(menu.commands.map((command) => command.name)).toEqual(["/model", "/compact"]);
    expect(menu.skills.map((skill) => skill.name)).toEqual(["grilling", "handoff", "owl"]);
  });

  it("filters commands and skills by what is typed", () => {
    expect(slashMenu("/g", skills)).toEqual({ commands: [], skills: [skills[0]] });
    expect(slashMenu("/mo", skills).skills).toEqual([]);
    expect(slashMenu("/grilling", skills).skills.map((skill) => skill.name)).toEqual(["grilling"]);
    expect(slashMenu("/grilling ", skills)).toEqual({ commands: [], skills: [] });
  });

  it("runs a chosen command and fills the draft with a chosen skill", () => {
    expect(menuChoice({ kind: "command", name: "/compact" })).toEqual({ run: "/compact" });
    expect(menuChoice({ kind: "skill", name: "grilling" })).toEqual({ draft: "/grilling " });
  });
});

describe("outgoingMessage", () => {
  it("sends the text after /name with that skill", () => {
    expect(outgoingMessage("/grilling my plan to open a bakery", skills)).toEqual({ message: "my plan to open a bakery", skill: "grilling" });
    expect(outgoingMessage("  /owl\tcheck the doors  \n", skills)).toEqual({ message: "check the doors", skill: "owl" });
    expect(outgoingMessage("/handoff\nline one\nline two", skills)).toEqual({ message: "line one\nline two", skill: "handoff" });
  });

  it("sends a bare /name as its own text", () => {
    expect(outgoingMessage("/grilling", skills)).toEqual({ message: "/grilling", skill: "grilling" });
    expect(outgoingMessage(" /grilling \n", skills)).toEqual({ message: "/grilling", skill: "grilling" });
  });

  it("refuses a skill the chat doesn't have instead of sending it as text", () => {
    expect(outgoingMessage("/wrap it up", skills)).toEqual({ unknownSkill: "wrap" });
    expect(outgoingMessage("/grilling my plan", [])).toEqual({ unknownSkill: "grilling" });
    expect(outgoingMessage("/grilling my plan", undefined)).toEqual({ unknownSkill: "grilling" });
  });

  it("leaves text that does not start with a skill name as a message", () => {
    for (const text of ["hello /grilling", "//grilling", "/etc/hosts is broken", "/Grilling now", "/ grilling", "plain"]) {
      expect(outgoingMessage(text, skills)).toEqual({ message: text });
    }
  });

  it("returns a queued message to the draft as it was typed", () => {
    expect(draftText({ message: "my plan", skill: "grilling" })).toBe("/grilling my plan");
    expect(draftText({ message: "/grilling", skill: "grilling" })).toBe("/grilling");
    expect(draftText({ message: "hello" })).toBe("hello");
  });
});
