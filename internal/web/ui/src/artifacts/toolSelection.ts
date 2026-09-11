import type { Item } from "../store/reducer";
import { activityTurns, type ToolItem } from "../chat/activityModel";

export type ToolSelection = {sessionId: string; key: string};

export function selectedToolInspection(selection: ToolSelection | undefined, sessionId: string | undefined, items: Item[]) {
  if (!selection || selection.sessionId !== sessionId) return null;
  const item = items.find((item) => item.key === selection.key);
  return item?.kind === "tool" ? item : null;
}

export function selectedMemoryTools(selection: {sessionId: string; snapshotId: string}, sessionId: string | undefined, items: Item[]): ToolItem[] {
  if (selection.sessionId !== sessionId) return [];
  const turn = activityTurns(items, false).find((group) => group.activity.some((item) => item.kind === "memory" && item.memory.snapshotId === selection.snapshotId));
  return turn?.activity.filter((item): item is ToolItem => item.kind === "tool" && item.name.startsWith("memory_")) ?? [];
}
