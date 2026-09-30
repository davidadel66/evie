import { useEffect, useId, useState } from "react";
import { toolFilePath } from "../artifacts/fileInspection";
import { readMemoryToolResult } from "../artifacts/memoryToolResult";
import type { Item } from "../store/reducer";
import { ChevronDown, Database, FileIcon, Wrench } from "../ui/Icon";
import { ApprovalCard } from "./ApprovalCard";
import { AssistantMessage, DiscardWarning } from "./Message";
import { MemoryActivity, type MemoryOpener } from "./MemoryActivity";
import { elapsedLabel, needsAttention, toolBatchLabel, toolLabel, type ActivityGroup, type ToolItem } from "./activityModel";

type FileOpener = (key: string, trigger: HTMLButtonElement) => void;
type Props = { group: ActivityGroup; onAnswer: (id: string, approve: boolean) => void; onOpenFile?: FileOpener; onOpenTool?: FileOpener; onOpenMemory?: MemoryOpener; onOpenInstructions?: (turnId:string,trigger:HTMLButtonElement)=>void };

export function Activity({ group, onAnswer, onOpenFile, onOpenTool, onOpenMemory, onOpenInstructions }: Props) {
  const [manual, setManual] = useState<boolean | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const bodyID = useId();
  useEffect(() => {
    if (!group.active) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [group.active]);
  const pending = group.activity.some((item) => item.kind === "tool" && item.approval?.state === "pending" && item.result === undefined);
  const open = manual ?? group.active;
  const latestMemory = [...group.activity].reverse().find((item) => item.kind === "memory");
  const visibleActivity = group.activity.filter((item) => item.kind !== "memory" || item === latestMemory || needsAttention(item));
  const elapsed = elapsedLabel(group.turn?.startedAt, group.active ? now : group.turn?.finishedAt);
  const complete = group.turn?.status === "complete" || (!group.turn && group.answer.length > 0);
  const label = pending ? "Waiting for approval" : group.active ? "Working…" : complete ? "Worked" : "Work incomplete";
  const show = group.active || group.activity.length > 0 || !!group.turn;
  if (!show) return null;
  return (
    <section aria-label="Turn activity" className="min-w-0 max-w-[min(900px,100%)] flex-none self-stretch">
      <button type="button" aria-expanded={open} aria-controls={bodyID} onClick={() => setManual(!open)}
        title="Elapsed wall time for this turn, including tools, provider waits and approvals."
        className="text-muted-text hover:text-body focus-visible:outline-teal flex max-w-full cursor-pointer items-center gap-2 rounded-sm py-1 text-left text-[13px] focus-visible:outline-2 focus-visible:outline-offset-4">
        <span>{label}{elapsed && `${complete && !pending ? " for" : " ·"} ${elapsed}`}</span>
        <span className={open ? "rotate-180" : ""}><ChevronDown size={13} /></span>
      </button>
      {onOpenInstructions&&group.turn?.id&&<button type="button" aria-label="Inspect instructions for this turn" onClick={event=>onOpenInstructions(group.turn!.id,event.currentTarget)} className="text-muted-text hover:text-teal mt-1 flex items-center gap-1.5 text-[11px]"><FileIcon size={12}/>Repository instructions</button>}
      <div id={bodyID} hidden={!open} className="border-hair mt-3 space-y-4 border-t pt-4">
        {open && <ActivityItems items={visibleActivity} active={group.active} onAnswer={onAnswer} onOpenFile={onOpenFile} onOpenTool={onOpenTool} onOpenMemory={onOpenMemory} />}
        {open && group.activity.length === 0 && <p className="text-muted-text text-xs">{group.active ? "Waiting for a response…" : "No additional activity to show."}</p>}
      </div>
      {!open && <div className="space-y-3"><ActivityItems items={group.activity.filter((item) => item === latestMemory || needsAttention(item))} active={group.active} onAnswer={onAnswer} onOpenFile={onOpenFile} onOpenTool={onOpenTool} onOpenMemory={onOpenMemory} /></div>}
      {!group.active && !complete && !group.turn?.finishedAt && <p className="text-amber-ink mt-2 text-xs">Completion wasn’t recorded here. Reload to check the saved conversation.</p>}
    </section>
  );
}

function ActivityItems({items, active, onAnswer, onOpenFile, onOpenTool, onOpenMemory}: {items: Item[]; active: boolean; onAnswer: Props["onAnswer"]; onOpenFile?: FileOpener; onOpenTool?: FileOpener; onOpenMemory?: MemoryOpener}) {
  const rows: (Item | ToolItem[])[] = [];
  for (const item of items) {
    if (item.kind === "tool" && !item.name.startsWith("memory_") && !needsAttention(item)) {
      const previous = rows[rows.length - 1];
      if (Array.isArray(previous)) previous.push(item);
      else rows.push([item]);
    } else rows.push(item);
  }
  return rows.map((row) => {
    if (Array.isArray(row)) {
      return <ToolBatch key={row[0].key} tools={row} active={active} onAnswer={onAnswer} onOpenFile={onOpenFile} onOpenTool={onOpenTool} />;
    }
    switch (row.kind) {
      case "memory": return <MemoryActivity key={row.key} activity={row.memory} onOpen={onOpenMemory} />;
      case "assistant": return <AssistantMessage key={row.key} text={row.text} streaming={active && row.streaming} discarded={row.discarded} />;
      case "notice": return <DiscardWarning key={row.key} message={row.text} />;
      case "reasoning": return <details key={row.key} className="text-muted-text text-[13px]">
        <summary className="activity-summary flex cursor-pointer items-center gap-3 py-1"><span className="min-w-0 flex-1 truncate">{row.text.replace(/\s+/g, " ").trim()}</span><span className="shrink-0 text-xs">Reasoning summary</span><ChevronDown size={12}/></summary>
        <p className="border-hair mt-2 max-h-80 overflow-auto border-l pl-4 leading-relaxed whitespace-pre-wrap break-words">{row.text}</p>
      </details>;
      case "tool": return <ToolRow key={row.key} tool={row} active={active} onAnswer={onAnswer} onOpenFile={onOpenFile} onOpenTool={onOpenTool} />;
      default: return null;
    }
  });
}

// Keep an inspected action visible when a single action becomes a batch.
function ToolBatch({ tools, active, onAnswer, onOpenFile, onOpenTool }: {tools: ToolItem[]; active: boolean; onAnswer: Props["onAnswer"]; onOpenFile?: FileOpener; onOpenTool?: FileOpener}) {
  const [manual, setManual] = useState<boolean | null>(null);
  const id = useId();
  const multiple = tools.length > 1;
  const open = !multiple || (manual ?? false);
  return <div className="min-w-0">
    {multiple && <button type="button" aria-expanded={open} aria-controls={id} onClick={() => setManual(!open)} className="activity-summary text-muted-text hover:text-body flex w-full cursor-pointer items-center gap-3 py-1 text-left text-[13px]">
      <ActionIcon kind={tools.some((tool) => toolLabel(tool).kind === "edit") ? "edit" : toolLabel(tools[0]).kind} />
      <span className="min-w-0 flex-1">{toolBatchLabel(tools)}</span><span className="shrink-0 text-xs">{tools.length} actions</span><ChevronDown size={12} />
    </button>}
    <div id={id} hidden={!open} className={multiple ? "border-hair mt-2 ml-2 space-y-2 border-l pl-4" : ""}>
      {tools.map((tool) => <ToolRow key={tool.key} tool={tool} active={active} onAnswer={onAnswer} onOpenFile={onOpenFile} onOpenTool={onOpenTool} onInspect={() => setManual(true)} />)}
    </div>
  </div>;
}

function ToolRow({ tool, active, onAnswer, onInspect, onOpenFile, onOpenTool }: {tool: ToolItem; active: boolean; onAnswer: Props["onAnswer"]; onInspect?: () => void; onOpenFile?: FileOpener; onOpenTool?: FileOpener}) {
  const {label, subject, kind} = toolLabel(tool);
  const path = toolFilePath(tool);
  const fileButton = path && onOpenFile;
  const pending = tool.approval?.state === "pending" && tool.result === undefined;
  const displayLabel = !active && tool.result === undefined && !tool.approval ? "No result recorded" : label;
  const result = readMemoryToolResult(tool);
  const returned = result?.kind === "records" ? `${result.records.length} ${result.records.length === 1 ? "record" : "records"} returned`
    : result?.kind === "search" ? `${result.matches} ${result.matches === 1 ? "match" : "matches"} returned` : "";
  if (fileButton) return <div className="min-w-0">
    <button type="button" title={path} aria-label={`Open ${path}`} onClick={(event) => { onInspect?.(); onOpenFile(tool.key, event.currentTarget); }} className={`activity-summary flex w-full min-w-0 cursor-pointer items-center gap-3 py-1 text-left text-[13px] ${needsAttention(tool) ? "text-amber-ink" : "text-muted-text hover:text-body"}`}>
      <ActionIcon kind={kind} />
      <span>{label}</span>
      <span className="text-teal ml-auto min-w-0 max-w-[55%] truncate underline decoration-teal-deep underline-offset-4">{path.split("/").filter(Boolean).pop() ?? path}</span>
    </button>
    {pending && <div className="mt-3"><ApprovalCard tool={tool} onAnswer={onAnswer} compact /></div>}
  </div>;
  return <div className="min-w-0">
    <button type="button" disabled={!onOpenTool} aria-label={`Inspect ${displayLabel}${subject ? `: ${subject}` : ""}${returned ? `, ${returned}` : ""}`} onClick={(event) => { onInspect?.(); onOpenTool?.(tool.key, event.currentTarget); }}
      className={`activity-summary focus-visible:outline-teal flex w-full min-w-0 cursor-pointer items-center gap-3 rounded-sm py-1 text-left text-[13px] focus-visible:outline-2 focus-visible:outline-offset-2 ${needsAttention(tool) ? "text-amber-ink" : "text-muted-text hover:text-body"}`}>
      <ActionIcon kind={kind} />
      <span className="min-w-0 max-w-[65%] break-words">{displayLabel}</span>
      {subject && <span className="min-w-0 flex-1 truncate" title={subject}>· {subject}</span>}
      {returned && <span className="text-muted-text ml-auto shrink-0 text-xs">{returned}</span>}
      <span className="text-teal ml-auto shrink-0 text-xs">Inspect</span>
    </button>
    {pending && <div className="mt-3"><ApprovalCard tool={tool} onAnswer={onAnswer} /></div>}
  </div>;
}

function ActionIcon({kind}: {kind: ReturnType<typeof toolLabel>["kind"]}) {
  if (kind === "read") return <FileIcon size={16} />;
  if (kind === "data" || kind === "memory") return <Database size={16} />;
  if (kind === "tool") return <Wrench size={16} />;
  return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden="true" className="shrink-0">
    {kind === "command" ? <path d="m4 5 7 7-7 7m9 0h7" /> : kind === "edit" ? <path d="m16 3 5 5L8 21H3v-5ZM13 6l5 5" /> : <><circle cx="10" cy="10" r="7"/><path d="m15 15 6 6"/></>}
  </svg>;
}
