import { needsAttention, toolLabel, type ToolItem } from "../chat/activityModel";
import { ToolResultSummary } from "./ToolResultSummary";

export function ToolInspection({tool}: {tool: ToolItem}) {
  const {label, subject} = toolLabel(tool);
  const approval = tool.approval?.state;
  return <div className="space-y-5 p-5">
    <div>
      <h2 className={`text-base font-medium ${needsAttention(tool) ? "text-amber-ink" : "text-ink"}`}>{label}</h2>
      {subject && <p className="text-body mt-3 whitespace-pre-wrap break-words">{subject}</p>}
      {approval && <p className="text-muted-text mt-3 text-xs">Approval: {approval[0].toUpperCase() + approval.slice(1)}</p>}
      {tool.result === undefined && <p className="text-muted-text mt-3 text-xs">No result recorded{approval === "approved" ? " · approval does not confirm execution" : ""}.</p>}
    </div>
    <ToolResultSummary tool={tool} />
    <details className="border-hair border-t pt-4">
      <summary className="text-muted-text hover:text-body focus-visible:outline-teal cursor-pointer rounded-sm text-xs focus-visible:outline-2 focus-visible:outline-offset-4">Debug details</summary>
      <ToolDebugDetails tool={tool} />
    </details>
  </div>;
}

export function ToolDebugDetails({tool}: {tool: ToolItem}) {
  return <div className="mt-4 space-y-4">
    <p className="text-muted-text text-xs">Tool: <code>{tool.name}</code></p>
    <ExactDetail label="Arguments" text={tool.args} />
    {tool.result !== undefined && <ExactDetail label="Result" text={tool.result} />}
  </div>;
}

function ExactDetail({label, text}: {label: string; text: string}) {
  return <div className="min-w-0"><p className="text-muted-text mb-2 text-xs">{label}</p><pre className="bg-code text-body max-h-80 overflow-auto rounded-md p-3 font-mono text-[11.5px] leading-relaxed whitespace-pre-wrap break-words">{text || "(empty)"}</pre></div>;
}
