export type ChatTextSize = "compact" | "default" | "large";

export const defaultChatTextSize: ChatTextSize = "default";

export function isChatTextSize(value: string | null): value is ChatTextSize {
  return value === "compact" || value === "default" || value === "large";
}

export function resolveChatTextSize(value: string | null): ChatTextSize {
  return isChatTextSize(value) ? value : defaultChatTextSize;
}
