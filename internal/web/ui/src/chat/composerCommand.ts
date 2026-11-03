// Composer commands: the few REPL slash commands the web composer runs
// instead of sending as a message. Matching follows the REPL on the trimmed
// text the composer would send, so `//compact` and `/compactor` stay ordinary
// message text. A draft that starts with /name for one of the chat's skills
// sends the rest of its text with that skill (skills.spec.md, Owner
// invocation).

import type { ChatSkill } from "../api/skills";

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

/** outgoingMessage reads a draft the composer sends. `/name rest` for one of
 *  the chat's skills sends rest with that skill, and a bare `/name` sends
 *  itself. `/name` for a skill the chat doesn't have (or before its skills
 *  have loaded) is refused rather than sent as text. Anything else is an
 *  ordinary message. */
export function outgoingMessage(text: string, skills: readonly ChatSkill[] | undefined): Outgoing | { unknownSkill: string } {
  const trimmed = text.trim();
  const match = /^\/([a-z][a-z0-9-]{0,63})(?:\s+([\s\S]*))?$/.exec(trimmed);
  if (!match || builtIn(match[1])) return { message: trimmed };
  const [, name, rest = ""] = match;
  if (!skills?.some((skill) => skill.name === name)) return { unknownSkill: name };
  return { message: rest.trim() || trimmed, skill: name };
}

/** draftText is a sent or queued message as David typed it. */
export function draftText(outgoing: Outgoing): string {
  if (!outgoing.skill || outgoing.message === `/${outgoing.skill}`) return outgoing.message;
  return `/${outgoing.skill} ${outgoing.message}`;
}
