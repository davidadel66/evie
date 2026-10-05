// Composer commands: the few REPL slash commands the web composer runs
// instead of sending as a message. Matching follows the REPL on the trimmed
// text the composer would send, so `//compact` and `/compactor` stay ordinary
// message text.

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

/** Commands the menu offers for a draft that is still a bare "/" prefix. */
export function slashMatches(text: string): (typeof composerCommands)[number][] {
  if (!/^\/\S*$/.test(text)) return [];
  return composerCommands.filter((command) => command.name.startsWith(text));
}
