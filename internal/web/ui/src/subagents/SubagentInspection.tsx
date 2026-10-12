import { useEffect, useState } from "react";
import { subagentAPI, type Subagent, type SubagentAPI, type SubagentTrace } from "../api/subagents";
import { ToolDebugDetails, ToolInspection } from "../artifacts/ToolInspection";
import { ActivityItems } from "../chat/Activity";
import type { ToolItem } from "../chat/activityModel";
import { ChevronDown } from "../ui/Icon";
import { subagentActionText, subagentElapsed, subagentFinished, subagentProgressLabel, subagentStateLabel, subagentStateTone } from "./subagentView";
import { useSubagentList } from "./useSubagents";

// The children one chat row started (chat-activity.spec.md, subagent
// inspector amendment): each child's state and latest action, kept current
// while any is unfinished; one child's own trace; and one of its actions.
export function SubagentInspection({tool, sessionId, api = subagentAPI}: {tool: ToolItem; sessionId: string; api?: SubagentAPI}) {
  const {list, problem} = useSubagentList(sessionId, tool, tool.result === undefined, api);
  const [open, setOpen] = useState<string>();
  const now = useNow(!!list?.some((child) => !subagentFinished(child.state)));
  const child = list?.find((item) => item.executionId === open);
  if (child) return <SubagentDetail child={child} sessionId={sessionId} api={api} now={now} onBack={() => setOpen(undefined)} />;
  if (!list) return <div className="p-5">{problem ? <p role="alert" className="text-danger text-xs">{problem}</p> : <p role="status" className="text-muted-text text-xs">Loading subagents…</p>}</div>;
  if (list.length === 0) return <ToolInspection tool={tool} />;
  return <SubagentList list={list} tool={tool} now={now} problem={problem} onOpen={setOpen} />;
}

export function SubagentList({list, tool, now, problem, onOpen}: {list: Subagent[]; tool: ToolItem; now: number; problem?: string; onOpen: (executionId: string) => void}) {
  return <div className="space-y-5 p-5">
    <div>
      <h2 className="text-ink text-base font-medium">{list.length === 1 ? "1 subagent" : `${list.length} subagents`}</h2>
      <p className="text-muted-text mt-1 text-xs">{subagentProgressLabel(list)}{tool.result === undefined ? "" : " · results returned to Evie"}</p>
      {problem && <p role="alert" className="text-amber-ink mt-2 text-xs">{problem}</p>}
    </div>
    <ul className="space-y-1">
      {list.map((item) => <li key={item.executionId}>
        <button type="button" aria-label={`Open subagent: ${item.objective}`} onClick={() => onOpen(item.executionId)}
          className="hover:bg-hover focus-visible:outline-teal -mx-2 flex w-[calc(100%+1rem)] min-w-0 cursor-pointer flex-col gap-1 rounded-md px-2 py-2 text-left focus-visible:outline-2">
          <span className="flex w-full min-w-0 items-baseline gap-3 text-[13px]">
            <StateLabel child={item} />
            <span className="text-body line-clamp-2 min-w-0 flex-1 break-words">{item.objective}</span>
            <span className="text-muted-text shrink-0 text-xs">{subagentElapsed(item, now)}</span>
          </span>
          <span className="text-muted-text block w-full truncate text-xs">
            {item.latest ? subagentActionText(item.latest) : item.state === "admitted" ? "Waiting for a free slot" : "No actions yet"}
            {item.actions > 0 && ` · ${item.actions} ${item.actions === 1 ? "action" : "actions"}`}
          </span>
        </button>
      </li>)}
    </ul>
    <details className="border-hair border-t pt-4">
      <summary className="text-muted-text hover:text-body focus-visible:outline-teal cursor-pointer rounded-sm text-xs focus-visible:outline-2 focus-visible:outline-offset-4">Debug details</summary>
      <ToolDebugDetails tool={tool} />
    </details>
  </div>;
}

function SubagentDetail({child, sessionId, api, now, onBack}: {child: Subagent; sessionId: string; api: SubagentAPI; now: number; onBack: () => void}) {
  const trace = useSubagentTrace(sessionId, child, api);
  const [selected, setSelected] = useState<string>();
  const action = trace.value?.items.find((item): item is ToolItem => item.kind === "tool" && item.key === selected);
  if (action) return <div>
    <BackButton label="Back to subagent" onClick={() => setSelected(undefined)} />
    <ToolInspection tool={action} />
  </div>;
  return <SubagentDetailView child={child} trace={trace.value} problem={trace.problem} now={now} onBack={onBack} onSelect={setSelected} />;
}

export function SubagentDetailView({child, trace, problem, now, onBack, onSelect}: {child: Subagent; trace?: SubagentTrace; problem?: string; now: number; onBack: () => void; onSelect: (key: string) => void}) {
  const live = !subagentFinished(child.state);
  return <div>
    <BackButton label="All subagents" onClick={onBack} />
    <div className="space-y-5 px-5 pb-5">
    <div className="space-y-2">
      <p className="flex items-baseline gap-3 text-[13px]"><StateLabel child={child} /><span className="text-muted-text text-xs">
        {[subagentElapsed(child, now), `${child.actions} ${child.actions === 1 ? "action" : "actions"}`, child.worker].filter(Boolean).join(" · ")}
      </span></p>
      <p className="text-body max-h-60 overflow-auto text-[13px] leading-relaxed whitespace-pre-wrap break-words">{child.objective}</p>
      {child.context && <details className="text-muted-text text-xs">
        <summary className="hover:text-body cursor-pointer">Context Evie gave it</summary>
        <p className="mt-2 max-h-60 overflow-auto leading-relaxed whitespace-pre-wrap break-words">{child.context}</p>
      </details>}
    </div>
    <div className="border-hair space-y-3 border-t pt-4">
      {problem && <p role="alert" className="text-amber-ink text-xs">{problem}</p>}
      {!trace ? !problem && <p role="status" className="text-muted-text text-xs">Loading its activity…</p>
        : <ActivityItems items={trace.items} active={live} onAnswer={() => undefined} onOpenTool={(key) => onSelect(key)} onOpenFile={(key) => onSelect(key)} />}
      {trace && trace.items.length <= 1 && <p className="text-muted-text text-xs">{live ? "Starting…" : "No activity was recorded."}</p>}
    </div>
    </div>
  </div>;
}

function StateLabel({child}: {child: Subagent}) {
  const tone = subagentStateTone(child.state);
  const color = tone === "live" ? "text-teal" : tone === "attention" ? "text-amber-ink" : tone === "done" ? "text-body" : "text-muted-text";
  return <span className={`${color} shrink-0 text-xs font-medium`}>{subagentStateLabel(child.state)}</span>;
}

function BackButton({label, onClick}: {label: string; onClick: () => void}) {
  return <button type="button" onClick={onClick} className="text-muted-text hover:text-body focus-visible:outline-teal flex cursor-pointer items-center gap-1.5 rounded-sm px-5 pt-5 pb-3 text-xs focus-visible:outline-2">
    <span className="rotate-90"><ChevronDown size={12} /></span>{label}
  </button>;
}

// A child's trace, read again while it is unfinished.
function useSubagentTrace(sessionId: string, child: Subagent, api: SubagentAPI) {
  const [value, setValue] = useState<SubagentTrace>();
  const [problem, setProblem] = useState("");
  const live = !subagentFinished(child.state);
  useEffect(() => {
    const controller = new AbortController();
    let timer: number | undefined;
    const tick = () => api.trace(sessionId, child.executionId, controller.signal).then((next) => {
      if (controller.signal.aborted) return;
      setValue(next);
      setProblem("");
      if (live) timer = window.setTimeout(tick, 3000);
    }, (error: unknown) => {
      if (controller.signal.aborted) return;
      setProblem(error instanceof Error ? error.message : "The subagent could not be read.");
      if (live) timer = window.setTimeout(tick, 6000);
    });
    void tick();
    return () => { controller.abort(); window.clearTimeout(timer); };
  }, [api, sessionId, child.executionId, live]);
  return {value, problem};
}

function useNow(ticking: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!ticking) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [ticking]);
  return now;
}
