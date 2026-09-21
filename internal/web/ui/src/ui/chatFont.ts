export type ChatFont = "default" | "system" | "serif";

export function resolveChatFont(value: string | null): ChatFont {
  return value === "system" || value === "serif" ? value : "default";
}
