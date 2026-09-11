import type { MemoryActivityData } from "../api/memoryEvidence";
import { Database } from "../ui/Icon";

export type MemoryOpener = (snapshotId: string, trigger: HTMLButtonElement) => void;

export function MemoryActivity({ activity, onOpen }: { activity: MemoryActivityData; onOpen?: MemoryOpener }) {
  const unavailable = ["failed", "unavailable", "partial"].includes(activity.status);
  const kinds = activity.excerptCount > 0 ? activity.acceptedCount > 0 ? "Accepted memory · Conversation excerpt" : "Conversation excerpt" : "Accepted memory";
  const label = unavailable ? "Memory unavailable" : activity.status === "cancelled" ? "Memory search cancelled" : activity.status === "exhausted" ? "Memory budget exhausted" : kinds;
  const recorded = activity.requestStatus === "prepared" || activity.requestStatus === "interrupted";
  const detail = activity.status === "empty" ? "No matches" : `${activity.acceptedCount + activity.excerptCount} ${recorded ? "recorded" : "supplied"}`;
  const states = [
    activity.requestStatus === "interrupted" && "Request interrupted",
    activity.historicalCount && `${activity.historicalCount} historical`,
    activity.retiredCount && `${activity.retiredCount} marked retired`,
    activity.conflictCount && `${activity.conflictCount} with conflicts`,
  ].filter(Boolean).join(" · ");
  return <button type="button" aria-label={recorded ? "Inspect recorded memory" : "Inspect supplied memory"} onClick={(event) => onOpen?.(activity.snapshotId, event.currentTarget)} disabled={!onOpen}
    className="text-muted-text hover:text-body flex w-full cursor-pointer items-center gap-3 py-1 text-left text-[13px] disabled:cursor-default">
    <Database size={16} />
    <span>{label}{states && <span className="mt-1 block text-xs">{states}</span>}</span>
    <span className="text-teal ml-auto text-xs">{detail}</span>
  </button>;
}
