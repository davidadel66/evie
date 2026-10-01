// Composer commands: the few REPL slash commands the web composer runs
// instead of sending as a message. Matching follows the REPL on the trimmed
// text the composer would send, so `//compact` and `/compactor` stay ordinary
// message text.

export type ComposerCommand = { kind: "compact" } | { kind: "usage"; message: string };

export function composerCommand(text: string): ComposerCommand | null {
  const trimmed = text.trim();
  if (trimmed === "/compact") return { kind: "compact" };
  if (/^\/compact\s/.test(trimmed)) return { kind: "usage", message: "Usage: /compact" };
  return null;
}
