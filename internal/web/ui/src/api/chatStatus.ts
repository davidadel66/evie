import { ApiError, errorText, readBlocks } from "./stream";

/** One chat's status (multi-chat.spec.md Stage 4). finishedAt is when its
 *  latest turn ended in this server run, in Unix milliseconds. */
export type ChatStatus = { sessionId: string; running?: boolean; waiting?: number; finishedAt?: number };

/** followChatStatus reports every chat's status now and after each change,
 *  until the server ends the stream or signal aborts. */
export async function followChatStatus(onStatus: (chats: ChatStatus[]) => void, signal: AbortSignal): Promise<void> {
  const res = await fetch("/api/chats/status", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: "{}",
    signal,
  });
  if (!res.ok) throw new ApiError(res.status, await errorText(res));
  if (!res.body) return;
  await readBlocks(res.body, (name, data) => {
    if (name !== "chat_status") return;
    try {
      const value = JSON.parse(data) as { chats?: unknown };
      if (Array.isArray(value.chats)) onStatus(value.chats as ChatStatus[]);
    } catch {
      // A malformed update is skipped; the next one carries the whole status.
    }
  });
}
