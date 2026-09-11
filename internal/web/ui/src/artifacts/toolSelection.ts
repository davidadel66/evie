import type { Item } from "../store/reducer";

export type ToolSelection = {sessionId: string; key: string};

export function selectedToolInspection(selection: ToolSelection | undefined, sessionId: string | undefined, items: Item[]) {
  if (!selection || selection.sessionId !== sessionId) return null;
  const item = items.find((item) => item.key === selection.key);
  return item?.kind === "tool" ? item : null;
}
