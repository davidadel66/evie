// Memories Evie proposed on her own wait here instead of interrupting the
// chat with an approval card (memory architecture Stage 2). Each item shows
// the same exact proposal and "Always applies" switch the card would; nothing
// is saved until the owner approves it.

import { useCallback, useEffect, useState } from "react";
import { memoryProposalAPI, type MemoryProposalAPI, type MemoryProposalChange, type MemoryProposalItem, type MemoryProposalPage } from "../api/memoryProposals";
import { AlwaysApplies, MemoryApproval } from "../chat/ApprovalCard";
import { readApprovalArgs } from "../chat/approvalArgs";
import { useMemoryPresentation } from "../memory/presentation";

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
  return <section aria-label="Proposed by Evie" className="text-body border-hair flex-none border-b p-5 text-sm">
    <div className="flex flex-wrap items-center gap-3">
      <h2 className="text-ink font-medium">Proposed by Evie</h2>
      {pending > 0 && <span className="text-muted-text text-xs">{pending} waiting</span>}
      <div className="flex-1" />
      <button type="button" className={button} disabled={state.busy} onClick={onRefresh}>Refresh</button>
    </div>
    <p className="text-muted-text mt-1 text-xs">Evie noticed these herself, during a chat or later in your conversations. Nothing is saved until you approve it.</p>
    {state.problem && <p role="alert" className="border-danger-hair bg-danger-bg text-danger-ink my-3 rounded border p-3 whitespace-pre-wrap">{state.problem}</p>}
    {state.notice && <p role="status" className="text-ok-ink my-3">{state.notice}</p>}
    {items.length > 0 && <ul className="mt-3 space-y-3">
      {items.map((item) => <ProposalItem key={item.proposal_id} item={item} busy={state.busy} chosen={state.alwaysApplies[switchKey(item)]}
        onAlwaysApplies={onAlwaysApplies} onApprove={onApprove} onDecline={onDecline} />)}
    </ul>}
    {pending > items.length && <p className="text-muted-text mt-3 text-xs">{pending - items.length} more after these.</p>}
  </section>;
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
  return <div className="px-4 py-3 text-sm">
    <p className="text-ink font-medium">{change.action === "update" ? "Update" : "Retire"}</p>
    <p className={change.action === "update" ? "text-muted-text line-through" : "text-body"}>{ownerText(change.current, ownerName) || "a memory that can no longer be read"}</p>
    {change.action === "update" && <p className="text-body">→ {ownerText(change.replacement, ownerName)}</p>}
    {change.evidence
      ? <><p className="text-muted-text mt-2 text-xs">Your words</p><blockquote className="border-hair text-body mt-1 border-l-2 pl-3 whitespace-pre-wrap">{change.evidence}</blockquote></>
      : change.evie_proposed
        ? <p className="text-amber-ink mt-2 text-xs">Evie’s proposal: your words don’t state this value. Approving saves it as Evie-proposed, not as something you said.</p>
        : null}
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
  const { ownerName } = useMemoryPresentation();
  const view = readApprovalArgs(item.tool_name, item.proposal, ownerName);
  const classification = view.shape === "memory" || view.shape === "json" ? view.classification : undefined;
  const alwaysApplies = chosen ?? classification?.standing ?? false;
  const queued = new Date(item.queued_at);
  const change = item.change?.action === "update" || item.change?.action === "retire" ? item.change : undefined;
  return <li className="border-hair bg-card overflow-hidden rounded border" aria-label="Memory proposal">
    <p className="text-muted-text px-4 pt-3 text-xs">
      {item.reason === "background" && <span className="text-teal mr-2 font-medium">Found in your conversations</span>}
      From {item.session_title || "a conversation"}{Number.isNaN(queued.getTime()) ? "" : ` · ${queued.toLocaleString()}`}
      {item.revision > 1 && " · Updated against newer memory since Evie proposed it"}
    </p>
    {item.reason === "no_explicit_cue" && <p className="text-amber-ink px-4 pt-2 text-xs">Evie said you asked for this, but your message didn’t ask her to remember anything, so it waits here.</p>}
    {change
      ? <ChangeSummary change={change} ownerName={ownerName} />
      : view.shape === "memory"
        ? <MemoryApproval view={view} />
        : <pre className="text-muted-text m-0 overflow-x-auto px-4 py-3 font-mono text-[11.5px] leading-[1.6]">{item.proposal}</pre>}
    {classification && <AlwaysApplies classification={classification} checked={alwaysApplies}
      onChange={item.approvable && !busy ? (value) => onAlwaysApplies(item, value) : undefined} />}
    {!item.approvable && item.problem && <p className="text-amber-ink px-4 py-2 text-xs">This can’t be approved now: {item.problem}</p>}
    <div className="border-hair flex items-center gap-3 border-t px-4 py-3">
      <button type="button" className={`${button} border-teal-hair text-teal`} disabled={busy || !item.approvable} onClick={() => onApprove(item)}>Approve</button>
      <button type="button" className={button} disabled={busy} onClick={() => onDecline(item)}>Decline</button>
    </div>
  </li>;
}
