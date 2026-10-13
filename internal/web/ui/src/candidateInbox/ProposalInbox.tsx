// Memories Evie proposed on her own wait here instead of interrupting the
// chat with an approval card (memory architecture Stage 2). The owner
// reviews them one at a time, readable at a glance: the memory itself
// first, then where it applies, its kind and whether it is in Evie's
// wording, then one line of the message Evie read. Chips narrow the queue
// to one place; A, D and → approve, decline and skip. Each card keeps the
// "Always applies" switch, "Everywhere instead" for a Workspace memory Evie
// thinks holds everywhere, and the exact change under Details. Nothing is
// saved until the owner approves it.

import { useCallback, useEffect, useState } from "react";
import { memoryProposalAPI, type MemoryProposalAPI, type MemoryProposalChange, type MemoryProposalItem, type MemoryProposalPage } from "../api/memoryProposals";
import { kindLabel, readApprovalArgs } from "../chat/approvalArgs";
import { scopeLabel, useMemoryPresentation } from "../memory/presentation";

const button = "border-hair-input text-body hover:bg-hover rounded border px-3 py-1.5 text-xs disabled:opacity-40";
const chip = "border-hair text-muted-text rounded-full border px-2 py-px text-[11px] whitespace-nowrap";

export type ProposalInboxState = {
  page?: MemoryProposalPage;
  busy: boolean;
  problem: string;
  notice: string;
  /** The owner's "Always applies" choice per proposal revision. */
  alwaysApplies: Record<string, boolean>;
  /** The owner's "Save everywhere instead" choice per proposal revision. */
  everywhere?: Record<string, boolean>;
};

function switchKey(item: MemoryProposalItem): string {
  return `${item.proposal_id}:${item.revision}`;
}

/** version is the proposal the owner is looking at: its Global version when
 *  they chose "Save everywhere instead", else the proposal itself. */
function version(item: MemoryProposalItem, everywhere: boolean | undefined): { proposal: string; scope_key: string } {
  return everywhere && item.everywhere ? item.everywhere : item;
}

function problemText(error: unknown): string {
  return error instanceof Error ? error.message : "The memory inbox could not complete that request.";
}

/** sameMessage is every listed proposal from the message item came from. */
function sameMessage(items: MemoryProposalItem[], item: MemoryProposalItem): MemoryProposalItem[] {
  return items.filter((other) => other.session_id === item.session_id && (other.source_message ?? "") === (item.source_message ?? ""));
}

export function ProposalInbox({ api = memoryProposalAPI, onChanged }: { api?: MemoryProposalAPI; onChanged?: () => void }) {
  const [state, setState] = useState<ProposalInboxState>({ busy: true, problem: "", notice: "", alwaysApplies: {}, everywhere: {} });
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

  const everywhereChoice = (item: MemoryProposalItem): boolean => Boolean(item.everywhere && state.everywhere?.[switchKey(item)]);
  const standingChoice = (item: MemoryProposalItem): boolean => {
    const chosen = state.alwaysApplies[switchKey(item)];
    if (chosen !== undefined) return chosen;
    const view = readApprovalArgs(item.tool_name, version(item, everywhereChoice(item)).proposal);
    return (view.shape === "memory" || view.shape === "json") && view.classification ? view.classification.standing : false;
  };
  const act = async (run: () => Promise<string>) => {
    setState((current) => ({ ...current, busy: true, problem: "", notice: "" }));
    try {
      const notice = await run();
      onChanged?.();
      await load(notice);
    } catch (error) {
      onChanged?.();
      await load("", problemText(error));
    }
  };
  const decide = (item: MemoryProposalItem, approve: boolean) => act(async () => {
    if (!approve) {
      await api.decline(item.proposal_id);
      return "Declined";
    }
    const result = await api.approve(item, standingChoice(item), everywhereChoice(item));
    return result.standing ? "Saved · always applies" : "Saved";
  });
  const declineAll = (items: MemoryProposalItem[]) => act(async () => {
    for (const item of items) await api.decline(item.proposal_id);
    return `Declined ${items.length}`;
  });

  return <ProposalInboxView state={state}
    onAlwaysApplies={(item, value) => setState((current) => ({ ...current, alwaysApplies: { ...current.alwaysApplies, [switchKey(item)]: value } }))}
    onEverywhere={(item, value) => setState((current) => ({ ...current, everywhere: { ...current.everywhere, [switchKey(item)]: value } }))}
    onApprove={(item) => void decide(item, true)} onDecline={(item) => void decide(item, false)} onDeclineAll={(items) => void declineAll(items)} onRefresh={() => void load()} />;
}

/** placeName names where a memory applies, in a word or two. */
function placeName(scopeKey: string, names: Record<string, string>): string {
  if (scopeKey === "global") return "Everywhere";
  if (scopeKey.startsWith("session:")) return "This chat";
  return scopeLabel(scopeKey, names);
}

/** scopeChips counts the queue by where each proposal applies. */
function scopeChips(items: MemoryProposalItem[], names: Record<string, string>): { key: string; label: string; count: number }[] {
  const counts = new Map<string, number>();
  for (const item of items) counts.set(item.scope_key, (counts.get(item.scope_key) ?? 0) + 1);
  return [...counts].map(([key, count]) => ({ key, label: placeName(key, names), count })).sort((a, b) => b.count - a.count || a.label.localeCompare(b.label));
}

export function ProposalInboxView({ state, onAlwaysApplies, onEverywhere, onApprove, onDecline, onDeclineAll, onRefresh }: {
  state: ProposalInboxState;
  onAlwaysApplies: (item: MemoryProposalItem, value: boolean) => void;
  onEverywhere?: (item: MemoryProposalItem, value: boolean) => void;
  onApprove: (item: MemoryProposalItem) => void;
  onDecline: (item: MemoryProposalItem) => void;
  onDeclineAll?: (items: MemoryProposalItem[]) => void;
  onRefresh: () => void;
}) {
  const { names } = useMemoryPresentation();
  const [filter, setFilter] = useState("all");
  const [index, setIndex] = useState(0);
  const items = state.page?.items ?? [];
  const pending = state.page?.pending ?? 0;
  const chips = scopeChips(items, names);
  const place = chips.some((option) => option.key === filter) ? filter : "all";
  const queue = place === "all" ? items : items.filter((item) => item.scope_key === place);
  const at = queue.length ? Math.min(index, queue.length - 1) : 0;
  const item = queue[at];
  const skip = (step: number) => setIndex(queue.length ? (at + step + queue.length) % queue.length : 0);
  // A, D and the arrow keys work while the review shows a card and no field
  // has focus.
  useEffect(() => {
    if (!item) return;
    const keys = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (event.metaKey || event.ctrlKey || event.altKey || state.busy || (target && (target.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName)))) return;
      const key = event.key.toLowerCase();
      if (key === "a" && item.approvable) onApprove(item);
      else if (key === "d") onDecline(item);
      else if (key === "arrowright" || key === "s") skip(1);
      else if (key === "arrowleft") skip(-1);
      else return;
      event.preventDefault();
    };
    window.addEventListener("keydown", keys);
    return () => window.removeEventListener("keydown", keys);
  });
  if (items.length === 0 && !state.problem && !state.notice) return null;
  return <section aria-label="Proposed by Evie" className="text-body border-hair flex-none border-b px-5 py-4 text-sm">
    <div className="flex flex-wrap items-center gap-2">
      <h2 className="text-ink font-medium">Review</h2>
      {queue.length > 0 && <span className="text-muted-text text-xs">{at + 1} of {queue.length}</span>}
      <div className="flex-1" />
      {state.notice && <span role="status" className="text-ok-ink text-xs">{state.notice}</span>}
      <button type="button" aria-label="Refresh" title="Refresh" className="text-muted-text hover:text-body rounded px-1.5 text-sm disabled:opacity-40" disabled={state.busy} onClick={onRefresh}>↻</button>
    </div>
    {chips.length > 1 && <div role="group" aria-label="Where it applies" className="mt-2 flex flex-wrap gap-1.5">
      {[{ key: "all", label: "All", count: items.length }, ...chips].map((option) => <button key={option.key} type="button" aria-pressed={place === option.key}
        onClick={() => { setFilter(option.key); setIndex(0); }}
        className={`rounded-full border px-2.5 py-0.5 text-xs ${place === option.key ? "border-teal-hair text-teal" : "border-hair text-muted-text hover:text-body"}`}>
        {option.label} {option.count}
      </button>)}
    </div>}
    {state.problem && <p role="alert" className="border-danger-hair bg-danger-bg text-danger-ink mt-3 rounded border p-3 whitespace-pre-wrap">{state.problem}</p>}
    {item && <ProposalCard key={switchKey(item)} item={item} siblings={sameMessage(items, item)} busy={state.busy}
      chosen={state.alwaysApplies[switchKey(item)]} everywhere={state.everywhere?.[switchKey(item)]}
      onAlwaysApplies={onAlwaysApplies} onEverywhere={onEverywhere} onApprove={onApprove} onDecline={onDecline}
      onSkip={queue.length > 1 ? () => skip(1) : undefined} onDeclineAll={onDeclineAll} />}
    {pending > items.length && <p className="text-muted-text mt-2 text-xs">{pending - items.length} more after these</p>}
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

type Line = { label: string; value: string; not?: boolean };

/** splitLine turns "David — workspace purpose: X" into a small label and
 *  the value; the owner's own name is left out, another subject kept. */
function splitLine(text: string, ownerName: string): Line {
  let rest = text;
  const not = rest.startsWith("Not: ");
  if (not) rest = rest.slice("Not: ".length);
  if (rest.startsWith(`${ownerName} — `)) rest = rest.slice(ownerName.length + 3);
  const colon = rest.indexOf(": ");
  return colon < 0 ? { label: "", value: rest, not } : { label: rest.slice(0, colon), value: rest.slice(colon + 2), not };
}

function EvieWording() {
  return <span title="You didn’t say it in these words. Approving saves it as Evie’s proposal, not as something you said."
    className="border-amber-hair-strong text-amber-ink rounded-full border px-2 py-px text-[11px] whitespace-nowrap">Evie’s wording</span>;
}

function Switch({ label, title, checked, onChange }: { label: string; title: string; checked: boolean; onChange?: (value: boolean) => void }) {
  return <span title={title} className="flex items-center gap-1.5 text-xs">
    <button type="button" role="switch" aria-checked={checked} aria-label={label} disabled={!onChange} onClick={() => onChange?.(!checked)}
      className={`flex h-[16px] w-[28px] flex-none cursor-pointer items-center rounded-full border-none p-[2px] disabled:cursor-default ${checked ? "bg-teal justify-end" : "bg-hair-strong justify-start"}`}>
      <span className="bg-app h-[12px] w-[12px] rounded-full" />
    </button>
    <span className={checked ? "text-ink" : "text-muted-text"}>{label}</span>
  </span>;
}

function ProposalCard({ item, siblings, busy, chosen, everywhere, onAlwaysApplies, onEverywhere, onApprove, onDecline, onSkip, onDeclineAll }: {
  item: MemoryProposalItem;
  siblings: MemoryProposalItem[];
  busy: boolean;
  chosen?: boolean;
  everywhere?: boolean;
  onAlwaysApplies: (item: MemoryProposalItem, value: boolean) => void;
  onEverywhere?: (item: MemoryProposalItem, value: boolean) => void;
  onApprove: (item: MemoryProposalItem) => void;
  onDecline: (item: MemoryProposalItem) => void;
  onSkip?: () => void;
  onDeclineAll?: (items: MemoryProposalItem[]) => void;
}) {
  const { ownerName, names } = useMemoryPresentation();
  const shown = version(item, everywhere);
  const view = readApprovalArgs(item.tool_name, shown.proposal, ownerName);
  const classification = view.shape === "memory" || view.shape === "json" ? view.classification : undefined;
  const alwaysApplies = chosen ?? classification?.standing ?? false;
  const change: MemoryProposalChange | undefined = item.change?.action === "update" || item.change?.action === "retire" ? item.change : undefined;
  const line = change
    ? splitLine(ownerText(change.action === "update" ? change.replacement : change.current, ownerName) || "a memory that can no longer be read", ownerName)
    : view.shape === "memory" ? splitLine(view.statement, ownerName) : { label: "", value: "A change to memory" };
  const was = change?.action === "update" ? splitLine(ownerText(change.current, ownerName), ownerName).value : "";
  const evieWording = change ? change.evie_proposed === true && !change.evidence : (view.shape === "memory" || view.shape === "json") && view.evieProposed === true;
  const yourWords = change?.evidence || (view.shape === "memory" ? view.evidence : "");
  const quote = yourWords || item.source_message || "";
  const queued = new Date(item.queued_at);
  const json = view.shape === "memory" || view.shape === "json" ? view.json : shown.proposal;
  return <article aria-label="Memory proposal" className="border-hair bg-card mt-3 rounded-lg border p-4">
    {line.label && <p className="text-muted-text text-xs">{line.label}</p>}
    <p className="text-ink text-base leading-snug font-medium">{change?.action === "retire" ? "Forget: " : ""}{line.not ? "Not: " : ""}{line.value}</p>
    {was && <p className="text-muted-text mt-0.5 text-xs line-through" aria-label="Was">{was}</p>}
    <div className="mt-2 flex flex-wrap items-center gap-1.5">
      <span className={chip}>{placeName(shown.scope_key, names)}</span>
      {classification && <span className={chip}>{kindLabel(classification.kind)}</span>}
      {change && <span className={chip}>{change.action === "update" ? "Update" : "Forget"}</span>}
      {evieWording && <EvieWording />}
    </div>
    {quote && <blockquote title={quote} className="border-hair text-muted-text mt-3 line-clamp-2 border-l-2 pl-3 text-xs whitespace-pre-wrap">
      {yourWords ? <span className="text-body">“{quote}”</span> : quote}
    </blockquote>}
    {item.reason === "no_explicit_cue" && <p className="text-amber-ink mt-2 text-xs">You didn’t ask Evie to remember this.</p>}
    {!item.approvable && item.problem && <p className="text-amber-ink mt-2 text-xs">Can’t approve: {item.problem}</p>}
    <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2">
      {classification && <Switch label="Always applies" title={alwaysApplies ? "Evie sees this on every request." : "Evie recalls this only when it’s relevant."}
        checked={alwaysApplies} onChange={item.approvable && !busy ? (value) => onAlwaysApplies(item, value) : undefined} />}
      {item.everywhere && <Switch label="Everywhere instead" title={`Evie thinks this may apply beyond ${scopeLabel(item.scope_key, names)}.`}
        checked={Boolean(everywhere)} onChange={item.approvable && !busy && onEverywhere ? (value) => onEverywhere(item, value) : undefined} />}
      <div className="flex-1" />
      <button type="button" className={button} disabled={busy} onClick={() => onDecline(item)} title="D">Decline</button>
      {onSkip && <button type="button" className={button} disabled={busy} onClick={onSkip} title="→">Skip</button>}
      <button type="button" className={`${button} border-teal-hair text-teal`} disabled={busy || !item.approvable} onClick={() => onApprove(item)} title="A">Approve</button>
    </div>
    <div className="text-muted-text mt-3 flex flex-wrap items-center gap-3 text-xs">
      {siblings.length > 1 && onDeclineAll && <button type="button" disabled={busy} onClick={() => onDeclineAll(siblings)} className="hover:text-body underline underline-offset-2 disabled:opacity-40">
        Decline all {siblings.length} from this message</button>}
      <details className="min-w-0 flex-1">
        <summary className="hover:text-body cursor-pointer">Details</summary>
        <p className="mt-2">{item.session_title || "A conversation"}{Number.isNaN(queued.getTime()) ? "" : ` · ${queued.toLocaleString()}`}
          {item.revision > 1 && " · updated against newer memory"}</p>
        {yourWords && item.source_message && <p className="mt-1 whitespace-pre-wrap">From your message: {item.source_message}</p>}
        {view.shape === "memory" && view.identities.length > 0 && <ul className="mt-1 space-y-0.5">{view.identities.map((identity) => <li key={identity}>{identity}</li>)}</ul>}
        <pre className="mt-2 overflow-auto whitespace-pre-wrap break-all">{json}</pre>
      </details>
    </div>
  </article>;
}
