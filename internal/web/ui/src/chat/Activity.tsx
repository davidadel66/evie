import { useEffect, useId, useMemo, useRef, useState } from "react";
import { toolFilePath } from "../artifacts/fileInspection";
import { readMemoryToolResult } from "../artifacts/memoryToolResult";
import type { Item } from "../store/reducer";
import { ChevronDown, Database, FileIcon, Wrench } from "../ui/Icon";
import { ApprovalCard } from "./ApprovalCard";
import { AssistantMessage, DiscardWarning } from "./Message";
import { MemoryActivity, type MemoryOpener } from "./MemoryActivity";
import { startsSubagents, subagentProgressLabel } from "../subagents/subagentView";
import { useSubagentList, useSubagentSession } from "../subagents/useSubagents";
import { actionSummary, awaitingApproval, describeThought, thoughtTitle, elapsedLabel, needsAttention, problemSummary, toolBatchLabel, toolLabel, type ActivityGroup, type ToolItem } from "./activityModel";

type FileOpener = (key: string, trigger: HTMLButtonElement) => void;
type Props = { group: ActivityGroup; onAnswer: (id: string, approve: boolean) => void; onOpenFile?: FileOpener; onOpenTool?: FileOpener; onOpenMemory?: MemoryOpener };

export function Activity({ group, onAnswer, onOpenFile, onOpenTool, onOpenMemory }: Props) {
  const [manual, setManual] = useState<boolean | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const bodyID = useId();
  useEffect(() => {
    if (!group.active) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [group.active]);
  const pending = group.activity.some(awaitingApproval);
  const open = manual ?? false;
  // A receipt that supplied nothing is routine noise; warnings still show below.
  const latestMemory = [...group.activity].reverse().find((item) => item.kind === "memory" && item.memory.acceptedCount + item.memory.excerptCount > 0);
  const visibleActivity = group.activity.filter((item) => item.kind !== "memory" || item === latestMemory || needsAttention(item));
  const collapsedActivity = group.activity.filter((item) => item === latestMemory || (needsAttention(item) && (item.kind !== "tool" || awaitingApproval(item))));
  const problems = problemSummary(group.activity);
  const elapsed = elapsedLabel(group.turn?.startedAt, group.active ? now : group.turn?.finishedAt);
  const complete = group.turn?.status === "complete" || (!group.turn && group.answer.length > 0);
  const summary = complete && !pending ? actionSummary(group.activity) : null;
  // "Searched the web, read a page · 1m 12s", or "Worked for 4s" with no action.
  const label = pending ? "Waiting for approval" : complete ? summary ?? "Worked" : "Work incomplete";
  const workedFor = complete && !pending && !summary;
  const show = group.active || group.activity.length > 0 || !!group.turn;
  if (!show) return null;
  return (
    <section aria-label="Turn activity" className="min-w-0 max-w-[min(900px,100%)] flex-none self-stretch">
      <button type="button" aria-expanded={open} aria-controls={bodyID} onClick={() => setManual(!open)} title={elapsedHint}
        className="text-muted-text hover:text-body focus-visible:outline-teal flex max-w-full cursor-pointer items-center gap-2 rounded-sm py-1 text-left text-[13px] focus-visible:outline-2 focus-visible:outline-offset-4">
        {group.active ? <LiveTitle activity={group.activity} pending={pending} collapsed={!open} />
          : <span className="min-w-0 truncate">{label}{elapsed && workedFor && ` for ${elapsed}`}</span>}
        {elapsed && !workedFor && <span className="shrink-0 tabular-nums">· {elapsed}</span>}
        <span className={`shrink-0 ${open ? "rotate-180" : ""}`}><ChevronDown size={13} /></span>
      </button>
      <div id={bodyID} hidden={!open} className="border-hair-strong divide-hair mt-2 divide-y rounded-[10px] border *:px-4 *:py-2">
        {open && <ActivityItems items={visibleActivity} active={group.active} onAnswer={onAnswer} onOpenFile={onOpenFile} onOpenTool={onOpenTool} onOpenMemory={onOpenMemory} />}
        {open && group.activity.length === 0 && <p className="text-muted-text text-xs">{group.active ? "Waiting for a response…" : "No additional activity to show."}</p>}
      </div>
      {!open && <div className="space-y-3">
        <ActivityItems items={collapsedActivity} active={group.active} onAnswer={onAnswer} onOpenFile={onOpenFile} onOpenTool={onOpenTool} onOpenMemory={onOpenMemory} />
        {problems && <button type="button" aria-controls={bodyID} onClick={() => setManual(true)}
          className="activity-summary text-amber-ink focus-visible:outline-teal flex cursor-pointer items-center gap-3 rounded-sm py-1 text-left text-[13px] focus-visible:outline-2 focus-visible:outline-offset-2">
          <span>{problems}</span><ChevronDown size={12} />
        </button>}
        {group.active && !pending && <StreamingText activity={group.activity} />}
      </div>}
      {!group.active && !complete && !group.turn?.finishedAt && <p className="text-amber-ink mt-2 text-xs">Completion wasn’t recorded here. Reload to check the saved conversation.</p>}
    </section>
  );
}

const elapsedHint = "Elapsed wall time for this turn, including tools, provider waits and approvals.";

// While working the header is the live line. Its title: the approval wait;
// else, when reasoning is the latest thing, that thought's title; else the
// latest requested tool; else Writing while text streams; else Thinking,
// which is all a model that hides its reasoning ever shows here.
function LiveTitle({activity, pending, collapsed}: {activity: Item[]; pending: boolean; collapsed: boolean}) {
  const last = activity[activity.length - 1];
  const tool = [...activity].reverse().find((item): item is ToolItem => item.kind === "tool" && item.result === undefined);
  // A delegate waits on its children, so it reports how far they got. The
  // expanded trace's row reports them itself, so only one of them polls.
  const waiting = !!tool && startsSubagents(tool) && collapsed;
  const {list: children} = useSubagentList(useSubagentSession(), waiting ? tool : undefined, waiting);
  // The clock re-renders every second; the reasoning only changes per delta.
  const thought = useMemo(() => last?.kind === "reasoning" ? thoughtTitle(last.text, last.streaming) : null, [last]);
  let title = "Thinking…";
  if (thought) title = thought;
  else if (tool) {
    const {label, subject} = toolLabel(tool);
    const progress = (waiting && children ? subagentProgressLabel(children) : null) ?? label;
    title = subject ? `${progress} · ${subject}` : progress;
  } else if (last?.kind === "assistant" && last.streaming && !last.discarded) title = "Writing…";
  // The approval wait shows at once and is never held, so an answered
  // approval does not linger on the line.
  const held = useHeld(pending ? null : title);
  if (pending) return <span className="min-w-0 truncate">Waiting for approval</span>;
  const shown = held ?? title;
  return <span role="status" title={shown} className="min-w-0 truncate">
    <span key={shown} className="evie-shimmer evie-fade-in">{shown}</span>
  </span>;
}

function StreamingText({activity}: {activity: Item[]}) {
  const last = activity[activity.length - 1];
  if (last?.kind !== "assistant" || !last.streaming || last.discarded) return null;
  return <AssistantMessage text={last.text} streaming />;
}

const titleHoldMs = 2000;

// Reasoning completes sentences faster than they can be read, so each title
// stays up for a moment before the line moves to the newest. Null pauses
// the line; the first title after a pause shows at once.
function useHeld(value: string | null): string | null {
  const [shown, setShown] = useState(value);
  const since = useRef(0);
  useEffect(() => {
    if (value === null || value === shown) return;
    if (since.current === 0) since.current = Date.now();
    const wait = shown === null ? 0 : Math.max(0, since.current + titleHoldMs - Date.now());
    const timer = window.setTimeout(() => { since.current = Date.now(); setShown(value); }, wait);
    return () => window.clearTimeout(timer);
  }, [value, shown]);
  return shown;
}

export function ActivityItems({items, active, onAnswer, onOpenFile, onOpenTool, onOpenMemory}: {items: Item[]; active: boolean; onAnswer: Props["onAnswer"]; onOpenFile?: FileOpener; onOpenTool?: FileOpener; onOpenMemory?: MemoryOpener}) {
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
      case "assistant": return <AssistantMessage key={row.key} text={row.text} streaming={active && row.streaming} discarded={row.discarded} quiet={!(active && row.streaming)} />;
      case "notice": return <DiscardWarning key={row.key} message={row.text} />;
      case "reasoning": return <ThoughtRow key={row.key} text={row.text} streaming={active && row.streaming} />;
      case "tool": return <ToolRow key={row.key} tool={row} active={active} onAnswer={onAnswer} onOpenFile={onOpenFile} onOpenTool={onOpenTool} />;
      default: return null;
    }
  });
}

// A thought reads as its title; only a thought with more to say than its
// title opens, to its full text.
function ThoughtRow({text, streaming}: {text: string; streaming: boolean}) {
  const thought = useMemo(() => describeThought(text, streaming), [text, streaming]);
  const title = thought.title ?? "Thinking…";
  if (!thought.more) return <p className="text-muted-text py-1 text-[13px]">{title}</p>;
  return <details className="group text-muted-text text-[13px]">
    <summary className="activity-summary hover:text-body flex cursor-pointer items-center gap-2 py-1">
      <span className="min-w-0 truncate" title={title}>{title}</span>
      <span className="shrink-0 transition-transform duration-150 group-open:rotate-180"><ChevronDown size={12} /></span>
    </summary>
    <p className="mt-1 mb-1 max-h-80 overflow-auto leading-relaxed whitespace-pre-wrap break-words">{text}</p>
  </details>;
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
  // A delegate row waits on its children, so it reports how far they got.
  const waiting = active && tool.result === undefined && startsSubagents(tool);
  const {list: children} = useSubagentList(useSubagentSession(), waiting ? tool : undefined, waiting);
  const progress = waiting && children ? subagentProgressLabel(children) : null;
  const displayLabel = progress ?? (!active && tool.result === undefined && !tool.approval ? "No result recorded" : label);
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
