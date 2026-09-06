import type { Item } from "../store/reducer";
export type HistoryPage = { sessionId: string; items: Item[]; before?: string };
export async function readHistory(sessionId: string, before: string | undefined, signal: AbortSignal): Promise<HistoryPage> {
  const response = await fetch("/api/context-sessions/history", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ sessionId, before }), signal });
  const value = await response.json();
  if (!response.ok) throw new Error(value.error ?? "Conversation history could not be loaded.");
  if (value.sessionId !== sessionId || !Array.isArray(value.items)) throw new Error("Conversation history changed. Try again.");
  return value;
}
