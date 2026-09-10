import { useEffect, useId, useState } from "react";
import type { Item } from "../store/reducer";
import { ChevronDown, Database, FileIcon, Wrench } from "../ui/Icon";
import { ApprovalCard } from "./ApprovalCard";
import { AssistantMessage, DiscardWarning } from "./Message";
import { elapsedLabel, needsAttention, toolBatchLabel, toolLabel, type ActivityGroup, type ToolItem } from "./activityModel";

type Props = { group: ActivityGroup; onAnswer: (id: string, approve: boolean) => void };

export function Activity({ group, onAnswer }: Props) {
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
      <div id={bodyID} hidden={!open} className="border-hair mt-3 space-y-4 border-t pt-4">
        {open && <ActivityItems items={group.activity} active={group.active} onAnswer={onAnswer} />}
        {open && group.activity.length === 0 && <p className="text-muted-text text-xs">{group.active ? "Waiting for a response…" : "No additional activity to show."}</p>}
      </div>
      {!open && <div className="space-y-3"><ActivityItems items={group.activity.filter(needsAttention)} active={group.active} onAnswer={onAnswer} /></div>}
      {!group.active && !complete && <p className="text-amber-ink mt-2 text-xs">Completion wasn’t recorded here. Reload to check the saved conversation.</p>}
    </section>
  );
}

function ActivityItems({items, active, onAnswer}: {items: Item[]; active: boolean; onAnswer: Props["onAnswer"]}) {
  const rows: (Item | ToolItem[])[] = [];
  for (const item of items) {
    if (item.kind === "tool" && !needsAttention(item)) {
      const previous = rows[rows.length - 1];
      if (Array.isArray(previous)) previous.push(item);
      else rows.push([item]);
    } else rows.push(item);
  }
  return rows.map((row) => {
    if (Array.isArray(row)) {
      return <ToolBatch key={row[0].key} tools={row} active={active} onAnswer={onAnswer} />;
    }
    switch (row.kind) {
      case "assistant": return <AssistantMessage key={row.key} text={row.text} streaming={active && row.streaming} discarded={row.discarded} />;
      case "notice": return <DiscardWarning key={row.key} message={row.text} />;
      case "reasoning": return <details key={row.key} className="text-muted-text text-[13px]">
        <summary className="activity-summary flex cursor-pointer items-center gap-3 py-1"><span className="min-w-0 flex-1 truncate">{row.text.replace(/\s+/g, " ").trim()}</span><span className="shrink-0 text-xs">Reasoning summary</span><ChevronDown size={12}/></summary>
        <p className="border-hair mt-2 max-h-80 overflow-auto border-l pl-4 leading-relaxed whitespace-pre-wrap break-words">{row.text}</p>
      </details>;
      case "tool": return <ToolRow key={row.key} tool={row} active={active} onAnswer={onAnswer} />;
      default: return null;
    }
  });
}

// Keep each tool's disclosure mounted when a single action becomes a batch.
// If someone is reading its details, the new batch stays expanded.
function ToolBatch({ tools, active, onAnswer }: {tools: ToolItem[]; active: boolean; onAnswer: Props["onAnswer"]}) {
  const [manual, setManual] = useState<boolean | null>(null);
  const [openTools, setOpenTools] = useState<Set<string>>(() => new Set());
  const id = useId();
  const multiple = tools.length > 1;
  const open = !multiple || (manual ?? openTools.size > 0);
  return <div className="min-w-0">
    {multiple && <button type="button" aria-expanded={open} aria-controls={id} onClick={() => setManual(!open)} className="activity-summary text-muted-text hover:text-body flex w-full cursor-pointer items-center gap-3 py-1 text-left text-[13px]">
      <ActionIcon kind={tools.some((tool) => toolLabel(tool).kind === "edit") ? "edit" : toolLabel(tools[0]).kind} />
      <span className="min-w-0 flex-1">{toolBatchLabel(tools)}</span><span className="shrink-0 text-xs">{tools.length} actions</span><ChevronDown size={12} />
    </button>}
    <div id={id} hidden={!open} className={multiple ? "border-hair mt-2 ml-2 space-y-2 border-l pl-4" : ""}>
      {tools.map((tool) => <ToolRow key={tool.key} tool={tool} active={active} onAnswer={onAnswer} onToggle={(expanded) => setOpenTools((previous) => {
        const next = new Set(previous);
        if (expanded) next.add(tool.key); else next.delete(tool.key);
        return next;
      })} />)}
    </div>
  </div>;
}

function ToolRow({ tool, active, onAnswer, onToggle }: {tool: ToolItem; active: boolean; onAnswer: Props["onAnswer"]; onToggle?: (open: boolean) => void}) {
  const {label, subject, kind} = toolLabel(tool);
  const pending = tool.approval?.state === "pending" && tool.result === undefined;
  return <div className="min-w-0">
    <details className="min-w-0" open={tool.isErr || undefined} onToggle={(event) => onToggle?.(event.currentTarget.open)}>
      <summary className={`activity-summary flex cursor-pointer items-center gap-3 py-1 text-[13px] ${needsAttention(tool) ? "text-amber-ink" : "text-muted-text hover:text-body"}`}>
        <ActionIcon kind={kind} />
        <span className="min-w-0 max-w-[65%] break-words">{!active && tool.result === undefined && !tool.approval ? "No result recorded" : label}</span>
        {subject && <span className="min-w-0 flex-1 truncate" title={subject}>· {subject}</span>}
        <ChevronDown size={12} />
      </summary>
      <div className="border-hair mt-2 space-y-3 border-l pb-1 pl-5">
        <p className="text-muted-text text-xs">Tool: <code>{tool.name}</code></p>
        <ExactDetail label="Arguments" text={tool.args} />
        {tool.approval && !pending && <ApprovalCard tool={tool} onAnswer={onAnswer} />}
        {tool.result !== undefined && <ExactDetail label="Result" text={tool.result} />}
      </div>
    </details>
    {pending && <div className="mt-3"><ApprovalCard tool={tool} onAnswer={onAnswer} /></div>}
  </div>;
}

function ExactDetail({label, text}: {label: string; text: string}) {
  return <div className="min-w-0"><p className="text-muted-text mb-1 text-xs">{label}</p><pre className="bg-code text-body max-h-80 overflow-auto rounded-md p-3 font-mono text-[11.5px] leading-relaxed whitespace-pre-wrap break-words">{text || "(empty)"}</pre></div>;
}

function ActionIcon({kind}: {kind: ReturnType<typeof toolLabel>["kind"]}) {
  if (kind === "read") return <FileIcon size={16} />;
  if (kind === "data") return <Database size={16} />;
  if (kind === "tool") return <Wrench size={16} />;
  return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden="true" className="shrink-0">
    {kind === "command" ? <path d="m4 5 7 7-7 7m9 0h7" /> : kind === "edit" ? <path d="m16 3 5 5L8 21H3v-5ZM13 6l5 5" /> : <><circle cx="10" cy="10" r="7"/><path d="m15 15 6 6"/></>}
  </svg>;
}
