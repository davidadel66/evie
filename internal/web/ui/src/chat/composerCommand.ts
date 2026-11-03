// Composer commands: the few REPL slash commands the web composer runs
// instead of sending as a message, matched like the REPL on the trimmed text
// the composer would send, so `/compactor` is not /compact. Everything else
// is sent (outgoingMessage): in a chat with skills a draft that starts with
// /name sends the rest of its text with that skill, and an unknown /name is
// refused (skills.spec.md, Owner invocation); in a chat without skills slash
// text is an ordinary message; in any chat a leading `//` sends the text
// from its second slash literally.

import type { ChatSkill, ChatSkills } from "../api/skills";

export type ComposerCommand = { kind: "compact" } | { kind: "model" } | { kind: "usage"; message: string };

export function composerCommand(text: string): ComposerCommand | null {
  const trimmed = text.trim();
  if (trimmed === "/compact") return { kind: "compact" };
  if (/^\/compact\s/.test(trimmed)) return { kind: "usage", message: "Usage: /compact" };
  // /model opens the picker; the web composer takes no model argument.
  if (trimmed === "/model") return { kind: "model" };
  if (/^\/model\s/.test(trimmed)) return { kind: "usage", message: "Usage: /model" };
  return null;
}

// The commands the composer's "/" menu offers, in display order.
export const composerCommands = [
  { name: "/model", description: "Choose the model for this chat" },
  { name: "/compact", description: "Compact earlier context to free space" },
] as const;

export type SlashMenu = { commands: (typeof composerCommands)[number][]; skills: ChatSkill[] };

const builtIn = (name: string) => composerCommands.some((command) => command.name === `/${name}`);

/** What the "/" menu offers for a draft that is still a bare "/" prefix: the
 *  matching commands, then the matching skills. A skill named like a
 *  built-in command is left out; the command wins. */
export function slashMenu(text: string, skills: readonly ChatSkill[]): SlashMenu {
  if (!/^\/\S*$/.test(text)) return { commands: [], skills: [] };
  return {
    commands: composerCommands.filter((command) => command.name.startsWith(text)),
    skills: skills.filter((skill) => !builtIn(skill.name) && `/${skill.name}`.startsWith(text)),
  };
}

export type MenuOption = { kind: "command"; name: string } | { kind: "skill"; name: string };

/** Choosing a command runs it; choosing a skill fills the draft with
 *  `/name ` for David to type his request after it. */
export function menuChoice(option: MenuOption): { run: string } | { draft: string } {
  return option.kind === "command" ? { run: option.name } : { draft: `/${option.name} ` };
}

/** A message as it is sent: its text, and the skill it invokes. */
export type Outgoing = { message: string; skill?: string };

/** outgoingMessage reads a draft the composer sends, given the chat's
 *  skills (undefined until they have loaded). A leading `//` sends the text
 *  from its second slash as an ordinary message in any chat. In a chat
 *  without skills anything else is an ordinary message too. In a chat with
 *  skills, `/name rest` for one of them sends rest with that skill, and a
 *  bare `/name` sends itself; `/name` for a skill the chat doesn't have (or
 *  before its skills have loaded) is refused rather than sent as text. */
export function outgoingMessage(text: string, chat: ChatSkills | undefined): Outgoing | { unknownSkill: string } {
  const trimmed = text.trim();
  if (trimmed.startsWith("//")) return { message: trimmed.slice(1) };
  if (chat?.available === false) return { message: trimmed };
  const match = /^\/([a-z][a-z0-9-]{0,63})(?:\s+([\s\S]*))?$/.exec(trimmed);
  if (!match || builtIn(match[1])) return { message: trimmed };
  const [, name, rest = ""] = match;
  if (!chat?.skills.some((skill) => skill.name === name)) return { unknownSkill: name };
  return { message: rest.trim() || trimmed, skill: name };
}

/** unknownSkillNotice says why a `/name` draft was not sent: the chat has
 *  no such skill, or its skills haven't loaded yet, or loading them failed
 *  (problem) and is being retried. */
export function unknownSkillNotice(name: string, chat: ChatSkills | undefined, problem?: string): string {
  if (chat) return `This chat has no skill /${name}.`;
  if (problem) return `This chat's skills couldn't be loaded, so /${name} wasn't sent. Try again in a moment.`;
  return "This chat's skills haven't loaded yet. Try again in a moment.";
}

/** draftText is a sent or queued message as a draft that sends it again:
 *  `/name` and the text for one sent with a skill, and slash text sent as
 *  text escaped with `//` so it stays text. */
export function draftText(outgoing: Outgoing): string {
  if (outgoing.skill) return outgoing.message === `/${outgoing.skill}` ? outgoing.message : `/${outgoing.skill} ${outgoing.message}`;
  return outgoing.message.startsWith("/") ? `/${outgoing.message}` : outgoing.message;
}
