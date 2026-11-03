import { describe, expect, it } from "vitest";
import type { ChatSkill, ChatSkills } from "../api/skills";
import { composerCommand, draftText, menuChoice, outgoingMessage, slashMenu, unknownSkillNotice } from "./composerCommand";

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

  it("leaves everything else to be sent, as text or with a skill", () => {
    for (const text of ["//compact", "/compactor", "please /compact", "compact", "/context", "/models", "//model", ""]) {
      expect(composerCommand(text)).toBeNull();
    }
    // What is sent instead: "//" escapes to literal text in any chat; in a
    // chat with skills an unknown /name is refused, and in one without
    // skills it is ordinary text.
    const withSkills: ChatSkills = { available: true, skills: [] };
    const withoutSkills: ChatSkills = { available: false, skills: [] };
    expect(outgoingMessage("//compact", withSkills)).toEqual({ message: "/compact" });
    expect(outgoingMessage("//model", withoutSkills)).toEqual({ message: "/model" });
    expect(outgoingMessage("please /compact", withSkills)).toEqual({ message: "please /compact" });
    expect(outgoingMessage("compact", withSkills)).toEqual({ message: "compact" });
    expect(outgoingMessage("/compactor", withSkills)).toEqual({ unknownSkill: "compactor" });
    expect(outgoingMessage("/models", withSkills)).toEqual({ unknownSkill: "models" });
    expect(outgoingMessage("/compactor", withoutSkills)).toEqual({ message: "/compactor" });
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
const chat: ChatSkills = { available: true, skills };

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
    expect(outgoingMessage("/grilling my plan to open a bakery", chat)).toEqual({ message: "my plan to open a bakery", skill: "grilling" });
    expect(outgoingMessage("  /owl\tcheck the doors  \n", chat)).toEqual({ message: "check the doors", skill: "owl" });
    expect(outgoingMessage("/handoff\nline one\nline two", chat)).toEqual({ message: "line one\nline two", skill: "handoff" });
  });

  it("sends a bare /name as its own text", () => {
    expect(outgoingMessage("/grilling", chat)).toEqual({ message: "/grilling", skill: "grilling" });
    expect(outgoingMessage(" /grilling \n", chat)).toEqual({ message: "/grilling", skill: "grilling" });
  });

  it("refuses a skill the chat doesn't have instead of sending it as text", () => {
    expect(outgoingMessage("/wrap it up", chat)).toEqual({ unknownSkill: "wrap" });
    expect(outgoingMessage("/tmp is full", chat)).toEqual({ unknownSkill: "tmp" });
    expect(outgoingMessage("/grilling my plan", { available: true, skills: [] })).toEqual({ unknownSkill: "grilling" });
    expect(outgoingMessage("/grilling my plan", undefined)).toEqual({ unknownSkill: "grilling" });
  });

  it("sends slash text as an ordinary message in a chat without skills", () => {
    const older: ChatSkills = { available: false, skills: [] };
    expect(outgoingMessage("/tmp is full", older)).toEqual({ message: "/tmp is full" });
    expect(outgoingMessage(" /grilling my plan ", older)).toEqual({ message: "/grilling my plan" });
  });

  it("sends text after // literally, from its second slash, in any chat", () => {
    for (const skills of [chat, { available: false, skills: [] }, undefined]) {
      expect(outgoingMessage("//grilling is a skill", skills)).toEqual({ message: "/grilling is a skill" });
      expect(outgoingMessage(" //tmp is full\n", skills)).toEqual({ message: "/tmp is full" });
    }
  });

  it("leaves text that does not start with a skill name as a message", () => {
    for (const text of ["hello /grilling", "/etc/hosts is broken", "/Grilling now", "/ grilling", "plain"]) {
      expect(outgoingMessage(text, chat)).toEqual({ message: text });
    }
  });

  it("returns a queued message to the draft as it is sent again", () => {
    expect(draftText({ message: "my plan", skill: "grilling" })).toBe("/grilling my plan");
    expect(draftText({ message: "/grilling", skill: "grilling" })).toBe("/grilling");
    expect(draftText({ message: "hello" })).toBe("hello");
    // Slash text sent as text goes back escaped, so it stays text.
    expect(draftText({ message: "/tmp is full" })).toBe("//tmp is full");
    for (const outgoing of [{ message: "my plan", skill: "grilling" }, { message: "/grilling", skill: "grilling" }, { message: "/compact" }, { message: "hello" }]) {
      expect(outgoingMessage(draftText(outgoing), chat)).toEqual(outgoing);
    }
  });
});

// While the chat's skills load, or after loading them failed, a /name send
// is refused with a notice saying which.
describe("unknownSkillNotice", () => {
  it("says the chat has no such skill once its skills are known", () => {
    expect(unknownSkillNotice("wrap", chat)).toBe("This chat has no skill /wrap.");
    expect(unknownSkillNotice("wrap", chat, "Chat changed; refresh and try again")).toBe("This chat has no skill /wrap.");
  });

  it("says the skills haven't loaded yet while they load", () => {
    expect(unknownSkillNotice("grilling", undefined)).toBe("This chat's skills haven't loaded yet. Try again in a moment.");
  });

  it("says the skills couldn't be loaded while loading them fails", () => {
    for (const problem of ["This chat's skills could not be loaded.", "Failed to fetch"]) {
      expect(unknownSkillNotice("grilling", undefined, problem)).toBe("This chat's skills couldn't be loaded, so /grilling wasn't sent. Try again in a moment.");
    }
  });
});
