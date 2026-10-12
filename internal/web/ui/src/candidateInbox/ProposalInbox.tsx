// Memories Evie proposed on her own wait here instead of interrupting the
// chat with an approval card (memory architecture Stage 2). Proposals are
// grouped under the owner's message Evie read, shown as context, and each
// leads with the memory as one readable line. The exact proposal and the
// "Always applies" switch stay on every item; nothing is saved until the
// owner approves it.

import { useCallback, useEffect, useState } from "react";
import { memoryProposalAPI, type MemoryProposalAPI, type MemoryProposalChange, type MemoryProposalItem, type MemoryProposalPage } from "../api/memoryProposals";
import { kindLabel, readApprovalArgs, type MemoryClassification } from "../chat/approvalArgs";
import { scopeLabel, useMemoryPresentation } from "../memory/presentation";

const button = "border-hair-input text-body hover:bg-hover rounded border px-3 py-2 text-xs disabled:opacity-40";

export type ProposalInboxState = {
  page?: MemoryProposalPage;
  busy: boolean;
  problem: string;
  notice: string;
  /** The owner's "Always applies" choice per proposal revision. */
  alwaysApplies: Record<string, boolean>;
};

function switchKey(item: MemoryProposalItem): string {
  return `${item.proposal_id}:${item.revision}`;
}

function problemText(error: unknown): string {
  return error instanceof Error ? error.message : "The memory inbox could not complete that request.";
}

export function ProposalInbox({ api = memoryProposalAPI, onChanged }: { api?: MemoryProposalAPI; onChanged?: () => void }) {
  const [state, setState] = useState<ProposalInboxState>({ busy: true, problem: "", notice: "", alwaysApplies: {} });
  const load = useCallback(async (notice = "", problem = "") => {
    setState((current) => ({ ...current, busy: true }));
    try {
      const page = await api.list();
      setState((current) => ({ ...current, page, busy: false, notice, problem }));
    } catch (error) {
      setState((current) => ({ ...current, busy: false, notice, problem: problem || problemText(error) }));
    }
  }, [api]);
  useEffect(() => { void load(); }, [load]);

  const standingChoice = (item: MemoryProposalItem): boolean => {
    const chosen = state.alwaysApplies[switchKey(item)];
    if (chosen !== undefined) return chosen;
    const view = readApprovalArgs(item.tool_name, item.proposal);
    return (view.shape === "memory" || view.shape === "json") && view.classification ? view.classification.standing : false;
  };
  const decide = async (item: MemoryProposalItem, approve: boolean) => {
    setState((current) => ({ ...current, busy: true, problem: "", notice: "" }));
    try {
      let notice = "Declined. Nothing was saved.";
      if (approve) {
        const result = await api.approve(item, standingChoice(item));
        notice = result.standing ? "Saved. It applies to every request." : "Saved. Evie recalls it when it’s relevant.";
      } else {
        await api.decline(item.proposal_id);
      }
      onChanged?.();
      await load(notice);
    } catch (error) {
      onChanged?.();
      await load("", problemText(error));
    }
  };

  return <ProposalInboxView state={state}
    onAlwaysApplies={(item, value) => setState((current) => ({ ...current, alwaysApplies: { ...current.alwaysApplies, [switchKey(item)]: value } }))}
    onApprove={(item) => void decide(item, true)} onDecline={(item) => void decide(item, false)} onRefresh={() => void load()} />;
}

export function ProposalInboxView({ state, onAlwaysApplies, onApprove, onDecline, onRefresh }: {
  state: ProposalInboxState;
  onAlwaysApplies: (item: MemoryProposalItem, value: boolean) => void;
  onApprove: (item: MemoryProposalItem) => void;
  onDecline: (item: MemoryProposalItem) => void;
  onRefresh: () => void;
}) {
  const items = state.page?.items ?? [];
  const pending = state.page?.pending ?? 0;
  if (items.length === 0 && !state.problem && !state.notice) return null;
  const evieWording = items.some(isEvieProposed);
  return <section aria-label="Proposed by Evie" className="text-body border-hair flex-none border-b p-5 text-sm">
    <div className="flex flex-wrap items-center gap-3">
      <h2 className="text-ink font-medium">Proposed by Evie</h2>
      {pending > 0 && <span className="text-muted-text text-xs">{pending} waiting</span>}
      <div className="flex-1" />
      <button type="button" className={button} disabled={state.busy} onClick={onRefresh}>Refresh</button>
    </div>
    <p className="text-muted-text mt-1 text-xs">Evie noticed these in your conversations. Nothing is saved until you approve it.</p>
    {evieWording && <p className="text-muted-text mt-1 text-xs"><EvieWording /> means you didn’t say it in these words. Approving saves it as Evie’s proposal, not as something you said.</p>}
    {state.problem && <p role="alert" className="border-danger-hair bg-danger-bg text-danger-ink my-3 rounded border p-3 whitespace-pre-wrap">{state.problem}</p>}
    {state.notice && <p role="status" className="text-ok-ink my-3">{state.notice}</p>}
    {items.length > 0 && <div className="mt-4 space-y-5">
      {groupBySource(items).map((group) => <SourceGroup key={group.key} items={group.items} busy={state.busy} alwaysApplies={state.alwaysApplies}
        onAlwaysApplies={onAlwaysApplies} onApprove={onApprove} onDecline={onDecline} />)}
    </div>}
    {pending > items.length && <p className="text-muted-text mt-3 text-xs">{pending - items.length} more after these.</p>}
  </section>;
}

function isEvieProposed(item: MemoryProposalItem): boolean {
  if (item.change?.action === "update" || item.change?.action === "retire") return item.change.evie_proposed === true && !item.change.evidence;
  const view = readApprovalArgs(item.tool_name, item.proposal);
  return (view.shape === "memory" || view.shape === "json") && view.evieProposed === true;
}

/** groupBySource keeps proposals from one message together, in list order. */
function groupBySource(items: MemoryProposalItem[]): { key: string; items: MemoryProposalItem[] }[] {
  const groups = new Map<string, MemoryProposalItem[]>();
  for (const item of items) {
    const key = `${item.session_id}\u0000${item.source_message ?? ""}`;
    groups.set(key, [...(groups.get(key) ?? []), item]);
  }
  return [...groups].map(([key, grouped]) => ({ key, items: grouped }));
}

function EvieWording() {
  return <span className="border-amber-hair-strong text-amber-ink rounded border px-1.5 py-px text-[11px] font-medium whitespace-nowrap">Evie’s wording</span>;
}

/** sourceClampAt is the length past which a source message starts folded. */
const sourceClampAt = 280;

function SourceGroup({ items, busy, alwaysApplies, onAlwaysApplies, onApprove, onDecline }: {
  items: MemoryProposalItem[];
  busy: boolean;
  alwaysApplies: Record<string, boolean>;
  onAlwaysApplies: (item: MemoryProposalItem, value: boolean) => void;
  onApprove: (item: MemoryProposalItem) => void;
  onDecline: (item: MemoryProposalItem) => void;
}) {
  const [open, setOpen] = useState(false);
  const first = items[0];
  const message = first.source_message ?? "";
  const queued = new Date(first.queued_at);
  const long = message.length > sourceClampAt;
  return <article aria-label="Proposals from one message" className="border-hair bg-card overflow-hidden rounded border">
    <div className="border-hair border-b px-4 py-3">
      <p className="text-muted-text text-xs">
        {first.reason === "background" && <span className="text-teal font-medium">Found in your conversations · </span>}
        {first.session_title || "A conversation"}{Number.isNaN(queued.getTime()) ? "" : ` · ${queued.toLocaleString()}`}
      </p>
      {message && <figure className="mt-2">
        <figcaption className="text-muted-text text-xs">From your message</figcaption>
        <blockquote className={`border-hair text-body mt-1 border-l-2 pl-3 whitespace-pre-wrap ${long && !open ? "line-clamp-3" : ""}`}>{message}</blockquote>
        {long && <button type="button" className="text-teal mt-1 cursor-pointer text-xs hover:underline" onClick={() => setOpen(!open)}>{open ? "Show less" : "Show all"}</button>}
      </figure>}
    </div>
    <ul className="divide-hair divide-y">
      {items.map((item) => <ProposalItem key={item.proposal_id} item={item} busy={busy} chosen={alwaysApplies[switchKey(item)]}
        onAlwaysApplies={onAlwaysApplies} onApprove={onApprove} onDecline={onDecline} />)}
    </ul>
  </article>;
}

/** ownerText shows a Claim's owner anchor as the owner's name. */
function ownerText(text: string | undefined, ownerName: string): string {
  if (!text) return "";
  for (const prefix of ["owner — ", "Denied: owner — "]) {
    if (text.startsWith(prefix)) return `${prefix.startsWith("Denied") ? "Not: " : ""}${ownerName} — ${text.slice(prefix.length)}`;
  }
  return text;
}

/** ChangeSummary shows a background update or retirement of an existing
 *  memory in plain terms, with the owner's words it cites. */
function ChangeSummary({ change, ownerName }: { change: MemoryProposalChange; ownerName: string }) {
  return <div>
    <p className="text-ink font-medium">{change.action === "update" ? "Update a memory" : "Retire a memory"}{change.evie_proposed && !change.evidence && <> <EvieWording /></>}</p>
    <p className={change.action === "update" ? "text-muted-text line-through" : "text-body"}>{ownerText(change.current, ownerName) || "a memory that can no longer be read"}</p>
    {change.action === "update" && <p className="text-body">→ {ownerText(change.replacement, ownerName)}</p>}
    {change.evidence && <YourWords text={change.evidence} />}
  </div>;
}

function YourWords({ text }: { text: string }) {
  return <p className="text-muted-text mt-1 text-xs">Your words: <q className="text-body">{text}</q></p>;
}

/** applies says where an approved memory applies, in a few words. */
function applies(scopeKey: string, names: Record<string, string>): string {
  if (scopeKey === "global") return "applies everywhere";
  if (scopeKey.startsWith("session:")) return "applies in this conversation";
  return `applies in ${scopeLabel(scopeKey, names)}`;
}

function StandingSwitch({ classification, checked, onChange }: { classification: MemoryClassification; checked: boolean; onChange?: (value: boolean) => void }) {
  return <div className="flex min-w-0 items-center gap-2">
    <button type="button" role="switch" aria-checked={checked} aria-label="Always applies" disabled={!onChange} onClick={() => onChange?.(!checked)}
      className={`flex h-[18px] w-[32px] flex-none cursor-pointer items-center rounded-full border-none p-[2px] disabled:cursor-default ${checked ? "bg-teal justify-end" : "bg-hair-strong justify-start"}`}>
      <span className="bg-app h-[14px] w-[14px] rounded-full" />
    </button>
    <span className="text-xs"><span className="text-ink">Always applies</span> <span className="text-muted-text">· {checked ? "Evie sees this on every request." : "Evie recalls this only when it’s relevant."}</span>
      <span className="sr-only"> ({kindLabel(classification.kind)})</span></span>
  </div>;
}

function ProposalItem({ item, busy, chosen, onAlwaysApplies, onApprove, onDecline }: {
  item: MemoryProposalItem;
  busy: boolean;
  chosen?: boolean;
  onAlwaysApplies: (item: MemoryProposalItem, value: boolean) => void;
  onApprove: (item: MemoryProposalItem) => void;
  onDecline: (item: MemoryProposalItem) => void;
}) {
  const { ownerName, names } = useMemoryPresentation();
  const view = readApprovalArgs(item.tool_name, item.proposal, ownerName);
  const classification = view.shape === "memory" || view.shape === "json" ? view.classification : undefined;
  const alwaysApplies = chosen ?? classification?.standing ?? false;
  const change = item.change?.action === "update" || item.change?.action === "retire" ? item.change : undefined;
  const meta = [classification && kindLabel(classification.kind), applies(item.scope_key, names)].filter(Boolean).join(" · ");
  const json = view.shape === "memory" || view.shape === "json" ? view.json : item.proposal;
  return <li className="px-4 py-3" aria-label="Memory proposal">
    {change
      ? <ChangeSummary change={change} ownerName={ownerName} />
      : view.shape === "memory"
        ? <div>
            <p className="text-ink text-[15px] leading-snug font-medium">{view.statement}{view.evieProposed && <> <EvieWording /></>}</p>
            {view.evidence && <YourWords text={view.evidence} />}
            {view.identities.length > 0 && <ul className="text-muted-text mt-1 space-y-0.5 text-xs">{view.identities.map((line) => <li key={line}>{line}</li>)}</ul>}
          </div>
        : <pre className="text-muted-text m-0 overflow-x-auto font-mono text-[11.5px] leading-[1.6]">{item.proposal}</pre>}
    <p className="text-muted-text mt-1 text-xs">{meta}</p>
    {item.reason === "no_explicit_cue" && <p className="text-amber-ink mt-2 text-xs">Evie said you asked for this, but your message didn’t ask her to remember anything, so it waits here.</p>}
    {item.revision > 1 && <p className="text-muted-text mt-2 text-xs">Updated against newer memory since Evie proposed it.</p>}
    {!item.approvable && item.problem && <p className="text-amber-ink mt-2 text-xs">This can’t be approved now: {item.problem}</p>}
    <div className="mt-3 flex flex-wrap items-center gap-3">
      {classification && <StandingSwitch classification={classification} checked={alwaysApplies}
        onChange={item.approvable && !busy ? (value) => onAlwaysApplies(item, value) : undefined} />}
      <div className="flex-1" />
      <button type="button" className={button} disabled={busy} onClick={() => onDecline(item)}>Decline</button>
      <button type="button" className={`${button} border-teal-hair text-teal`} disabled={busy || !item.approvable} onClick={() => onApprove(item)}>Approve</button>
    </div>
    <details className="text-muted-text mt-2 text-xs"><summary className="cursor-pointer">Exact change</summary><pre className="mt-2 overflow-auto whitespace-pre-wrap break-all">{json}</pre></details>
  </li>;
}
