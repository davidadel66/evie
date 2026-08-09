// The gate. While pending it's the loudest thing on screen — amber chrome, the
// change spelled out, two buttons. Once answered it collapses to a one-line
// record and stays in the transcript: what was approved is history worth
// keeping (docs/decisions.md, "reads wide, writes gated").

import { Check, Cross } from "../ui/Icon";
import type { Item } from "../store/reducer";
import { readApprovalArgs } from "./approvalArgs";
import { Diff } from "./Diff";

type Tool = Extract<Item, { kind: "tool" }>;

type Props = {
  tool: Tool;
  onAnswer: (reqId: string, approve: boolean) => void;
};

export function ApprovalCard({ tool, onAnswer }: Props) {
  const approval = tool.approval!;
  if (approval.state === "pending") {
    return <Pending tool={tool} onAnswer={onAnswer} />;
  }
  return <Resolved tool={tool} />;
}

function Pending({ tool, onAnswer }: Props) {
  const view = readApprovalArgs(tool.name, tool.args);
  const preview = tool.approval!.preview;
  const reqId = tool.approval!.reqId;
  const subject = preview?.path || view.subject;

  return (
    <div
      className={`border-amber-hair-strong bg-amber-card min-w-0 flex-none self-stretch overflow-hidden rounded-[9px] border ${
        preview ? "max-w-[min(1100px,100%)]" : "max-w-[min(720px,100%)]"
      }`}
    >
      <div className="border-amber-hair bg-amber-bg flex items-center gap-[9px] border-b px-[13px] py-[9px]">
        <span className="bg-amber h-[7px] w-[7px] rounded-full" />
        <span className="text-amber-ink font-sans text-xs font-semibold">
          Approval required
        </span>
        <span className="text-muted-text min-w-0 truncate font-mono text-[11.5px]">
          {tool.name}
          {subject && ` · ${subject}`}
        </span>
      </div>

      {preview ? (
        <Diff oldText={preview.oldText} newText={preview.newText} isNew={preview.isNew} />
      ) : view.shape === "diff" ? (
        <Diff oldText={view.oldText} newText={view.newText} />
      ) : null}
      {!preview && view.shape === "statement" && (
        <pre className="text-body m-0 overflow-x-auto px-[14px] py-3 font-mono text-[11.5px] leading-[1.6]">
          {view.statement}
        </pre>
      )}
      {!preview && view.shape === "json" && (
        <pre className="text-muted-text m-0 overflow-x-auto px-[14px] py-3 font-mono text-[11.5px] leading-[1.6]">
          {view.json}
        </pre>
      )}

      <div className="border-amber-hair flex items-center gap-[10px] border-t px-[13px] py-[10px]">
        <button
          onClick={() => onAnswer(reqId, true)}
          className="bg-ok flex cursor-pointer items-center gap-2 rounded-[7px] border-none px-[14px] py-[7px] font-sans text-[12.5px] font-semibold text-[#0a0c0d]"
        >
          Approve <Key label="Y" dark />
        </button>
        <button
          onClick={() => onAnswer(reqId, false)}
          className="text-body flex cursor-pointer items-center gap-2 rounded-[7px] border border-[#333c3f] bg-transparent px-[14px] py-[6px] font-sans text-[12.5px] font-medium"
        >
          Decline <Key label="N" />
        </button>
        <span className="text-fainter min-w-0 truncate font-sans text-[11px]">
          Evie is paused until you decide
        </span>
      </div>
    </div>
  );
}

/** Resolved is the design's compact aftermath row. Declined and expired share
 *  the muted treatment but not the words — the model was told different
 *  things, so David should see which happened. */
function Resolved({ tool }: { tool: Tool }) {
  const state = tool.approval!.state;
  const approved = state === "approved";
  const view = readApprovalArgs(tool.name, tool.args);
  const preview = tool.approval!.preview;

  if (preview) {
    return (
      <div className="border-hair-strong bg-card min-w-0 max-w-[min(1100px,100%)] flex-none self-stretch overflow-hidden rounded-lg border">
        <div className="border-hair-strong flex items-center gap-[10px] border-b px-3 py-2">
          {approved ? (
            <Check stroke="var(--color-ok)" />
          ) : (
            <Cross stroke="var(--color-muted-text)" />
          )}
          <span className={approved ? "text-body-dim font-mono text-xs" : "text-muted-text font-mono text-xs line-through"}>
            {tool.name}
          </span>
          <span className="text-fainter min-w-0 flex-1 truncate font-mono text-[11.5px]">
            {preview.path}
          </span>
          <span className={approved ? "text-ok font-mono text-[10.5px]" : "text-muted-text font-mono text-[10.5px]"}>
            {label(state)}
          </span>
        </div>
        <Diff oldText={preview.oldText} newText={preview.newText} isNew={preview.isNew} />
      </div>
    );
  }

  return (
    <div
      className={`border-hair-strong bg-card flex min-w-0 max-w-[min(720px,100%)] flex-none items-center gap-[10px] self-stretch rounded-lg border px-3 py-2 ${
        approved ? "" : "opacity-85"
      }`}
    >
      {approved ? (
        <Check stroke="var(--color-ok)" />
      ) : (
        <Cross stroke="var(--color-muted-text)" />
      )}
      <span
        className={`font-mono text-xs ${
          approved ? "text-body-dim" : "text-muted-text line-through"
        }`}
      >
        {tool.name}
      </span>
      <span
        className={`min-w-0 flex-1 truncate font-mono text-[11.5px] ${
          approved ? "text-fainter" : "text-ghost"
        }`}
      >
        {view.shape === "json" ? tighten(view.json) : view.subject}
      </span>
      <span
        className={`font-mono text-[10.5px] whitespace-nowrap ${
          approved ? "text-ok" : "text-muted-text"
        }`}
      >
        {label(state)}
      </span>
    </div>
  );
}

/** tighten collapses pretty-printed JSON back to one line for the summary row,
 *  where the full shape doesn't fit anyway. */
function tighten(json: string): string {
  return json.replace(/\s+/g, " ").trim();
}

function label(state: string): string {
  if (state === "approved") return "Approved";
  if (state === "declined") return "Declined";
  return "Expired — never seen";
}

function Key({ label, dark }: { label: string; dark?: boolean }) {
  return (
    <kbd
      className={`rounded-[3px] px-[5px] py-[1px] font-mono text-[10px] font-semibold ${
        dark ? "bg-[rgba(0,0,0,.25)]" : "bg-hair-strong"
      }`}
    >
      {label}
    </kbd>
  );
}
