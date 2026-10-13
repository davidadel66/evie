// Memories Evie proposed on her own wait here instead of interrupting the
// chat with an approval card (memory architecture Stage 2). They come as a
// deck of cards, one at a time. The face holds only the memory, with where
// it applies and its kind as small pills and the "Always applies" and
// "Everywhere instead" choices; flipping the card shows the message Evie
// read, the exact change and "Decline all" for that message. Swipe or press
// → to save, ← to decline, ↑ to skip, Space to flip. Nothing is saved until
// the owner saves it, and a swipe has to travel well past the card's edge.

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { memoryProposalAPI, type MemoryProposalAPI, type MemoryProposalChange, type MemoryProposalItem, type MemoryProposalPage } from "../api/memoryProposals";
import { kindLabel, readApprovalArgs } from "../chat/approvalArgs";
import { scopeLabel, useMemoryPresentation } from "../memory/presentation";
import { ArrowUp, Check, Cross } from "../ui/Icon";

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

export function ProposalInbox({ api = memoryProposalAPI, onChanged, status }: { api?: MemoryProposalAPI; onChanged?: () => void; status?: ReactNode }) {
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

  return <ProposalInboxView state={state} status={status}
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

type Handlers = {
  onAlwaysApplies: (item: MemoryProposalItem, value: boolean) => void;
  onEverywhere?: (item: MemoryProposalItem, value: boolean) => void;
  onApprove: (item: MemoryProposalItem) => void;
  onDecline: (item: MemoryProposalItem) => void;
  onDeclineAll?: (items: MemoryProposalItem[]) => void;
};

export function ProposalInboxView({ state, status, onRefresh, ...handlers }: Handlers & { state: ProposalInboxState; status?: ReactNode; onRefresh: () => void }) {
  const { names } = useMemoryPresentation();
  const [filter, setFilter] = useState("all");
  const [index, setIndex] = useState(0);
  const items = state.page?.items ?? [];
  const pending = state.page?.pending ?? 0;
  const chips = scopeChips(items, names);
  const place = chips.some((option) => option.key === filter) ? filter : "all";
  const queue = place === "all" ? items : items.filter((item) => item.scope_key === place);
  const at = queue.length ? index % queue.length : 0;
  return <section aria-label="Memory review" className="text-body flex min-h-0 flex-1 flex-col">
    <div className="flex flex-wrap items-center gap-2 px-5 pt-4">
      <h2 className="text-ink text-sm font-medium">{queue.length ? `${queue.length} to review` : "Review"}</h2>
      {chips.length > 1 && <div role="group" aria-label="Where it applies" className="flex flex-wrap gap-1.5">
        {[{ key: "all", label: "All", count: items.length }, ...chips].map((option) => <button key={option.key} type="button" aria-pressed={place === option.key}
          onClick={() => { setFilter(option.key); setIndex(0); }}
          className={`rounded-full border px-2.5 py-0.5 text-xs ${place === option.key ? "border-teal-hair text-teal" : "border-hair text-muted-text hover:text-body"}`}>
          {option.label} {option.count}
        </button>)}
      </div>}
      <div className="flex-1" />
      {state.notice && <span role="status" className="text-ok-ink text-xs">{state.notice}</span>}
      {status}
      <button type="button" aria-label="Refresh" title="Refresh" className="text-muted-text hover:text-body rounded px-1.5 text-sm disabled:opacity-40" disabled={state.busy} onClick={onRefresh}>↻</button>
    </div>
    {state.problem && <p role="alert" className="border-danger-hair bg-danger-bg text-danger-ink mx-5 mt-3 rounded border p-3 text-xs whitespace-pre-wrap">{state.problem}</p>}
    {queue.length > 0
      ? <Deck key={switchKey(queue[at])} queue={queue} at={at} all={items} state={state} onSkip={() => setIndex(at + 1)} {...handlers} />
      : state.page && <AllCaughtUp />}
    {pending > items.length && <p className="text-muted-text pb-4 text-center text-xs">{pending - items.length} more after these</p>}
  </section>;
}

function AllCaughtUp() {
  return <div className="flex flex-1 flex-col items-center justify-center gap-3 py-16 text-center">
    <div aria-hidden="true" className="relative h-24 w-20">
      <span className="border-hair bg-card absolute inset-0 -rotate-6 rounded-xl border" />
      <span className="border-hair bg-card absolute inset-0 rotate-6 rounded-xl border" />
      <span className="border-teal-hair bg-card text-teal absolute inset-0 flex items-center justify-center rounded-xl border"><Check size={26} /></span>
    </div>
    <p className="text-ink text-base font-medium">All caught up</p>
    <p className="text-muted-text text-xs">New things Evie notices land here.</p>
  </div>;
}

/** swipeAt is how far, in pixels, a card must travel before letting go acts. */
const swipeAt = 120;

type Move = "save" | "decline" | "skip";

function reducedMotion(): boolean {
  return typeof window !== "undefined" && window.matchMedia?.("(prefers-reduced-motion: reduce)").matches === true;
}

function Deck({ queue, at, all, state, onSkip, onAlwaysApplies, onEverywhere, onApprove, onDecline, onDeclineAll }: Handlers & {
  queue: MemoryProposalItem[];
  at: number;
  all: MemoryProposalItem[];
  state: ProposalInboxState;
  onSkip: () => void;
}) {
  const item = queue[at];
  const behind = [1, 2].filter((offset) => offset < queue.length).map((offset) => queue[(at + offset) % queue.length]);
  const [drag, setDrag] = useState({ x: 0, y: 0, active: false });
  const [flipped, setFlipped] = useState(false);
  const [leaving, setLeaving] = useState<Move>();
  const [refused, setRefused] = useState(false);
  const start = useRef<{ x: number; y: number; moved: boolean } | undefined>(undefined);
  const busy = state.busy || leaving !== undefined;
  const move = (direction: Move) => {
    if (busy) return;
    if (direction === "save" && !item.approvable) {
      setDrag({ x: 0, y: 0, active: false });
      setRefused(true);
      return;
    }
    if (direction === "skip" && queue.length < 2) {
      setDrag({ x: 0, y: 0, active: false });
      return;
    }
    setLeaving(direction);
    window.setTimeout(() => {
      if (direction === "save") onApprove(item);
      else if (direction === "decline") onDecline(item);
      else onSkip();
    }, reducedMotion() ? 0 : 220);
  };
  useEffect(() => {
    const keys = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (event.metaKey || event.ctrlKey || event.altKey || (target && (target.isContentEditable || /^(INPUT|TEXTAREA|SELECT|BUTTON|SUMMARY|A)$/.test(target.tagName)))) return;
      if (event.key === "ArrowRight") move("save");
      else if (event.key === "ArrowLeft") move("decline");
      else if (event.key === "ArrowUp") move("skip");
      else if (event.key === " ") setFlipped((value) => !value);
      else return;
      event.preventDefault();
    };
    window.addEventListener("keydown", keys);
    return () => window.removeEventListener("keydown", keys);
  });
  const pointerDown = (event: React.PointerEvent) => {
    if (busy || (event.target as HTMLElement).closest("button, summary, a, pre, [data-no-drag]")) return;
    start.current = { x: event.clientX, y: event.clientY, moved: false };
    event.currentTarget.setPointerCapture(event.pointerId);
    setDrag({ x: 0, y: 0, active: true });
  };
  const pointerMove = (event: React.PointerEvent) => {
    if (!start.current) return;
    const x = event.clientX - start.current.x, y = event.clientY - start.current.y;
    if (Math.abs(x) + Math.abs(y) > 6) start.current.moved = true;
    setDrag({ x, y, active: true });
  };
  const pointerUp = () => {
    const begun = start.current;
    start.current = undefined;
    if (!begun) return;
    if (!begun.moved) {
      setDrag({ x: 0, y: 0, active: false });
      setFlipped((value) => !value);
    } else if (drag.x > swipeAt) move("save");
    else if (drag.x < -swipeAt) move("decline");
    else if (drag.y < -swipeAt && Math.abs(drag.y) > Math.abs(drag.x)) move("skip");
    else setDrag({ x: 0, y: 0, active: false });
  };
  const fly = leaving === "save" ? "translateX(150%) rotate(20deg)" : leaving === "decline" ? "translateX(-150%) rotate(-20deg)" : leaving === "skip" ? "translateY(-140%)" : "";
  const transform = fly || `translate(${drag.x}px, ${drag.y}px) rotate(${drag.x / 18}deg)`;
  const stamp = (amount: number) => Math.max(0, Math.min(1, amount / swipeAt));
  const upward = -drag.y > Math.abs(drag.x);
  return <div className="flex flex-1 flex-col items-center justify-center px-5 py-8">
    <div className="relative h-[330px] w-full max-w-[420px] [perspective:1400px]">
      {behind.slice().reverse().map((card, depth) => {
        const offset = behind.length - depth;
        return <div key={switchKey(card)} aria-hidden="true" className="border-hair bg-card absolute inset-0 rounded-2xl border shadow-lg transition-transform duration-200"
          style={{ transform: `translateY(${offset * 14}px) scale(${1 - offset * 0.06}) rotate(${offset % 2 ? -2.5 : 2.5}deg)`, opacity: 1 - offset * 0.25 }} />;
      })}
      <div role="group" aria-label="Memory proposal" aria-roledescription="card"
        onPointerDown={pointerDown} onPointerMove={pointerMove} onPointerUp={pointerUp} onPointerCancel={() => { start.current = undefined; setDrag({ x: 0, y: 0, active: false }); }}
        className="absolute inset-0 cursor-grab touch-none select-none active:cursor-grabbing"
        style={{ transform, transition: drag.active ? "none" : "transform 220ms ease, opacity 220ms ease", opacity: fly ? 0 : 1 }}>
        <div className="relative h-full w-full transition-transform duration-500 [transform-style:preserve-3d] motion-reduce:transition-none"
          style={{ transform: flipped ? "rotateY(180deg)" : undefined }}>
          <CardFace item={item} state={state} refused={refused} onAlwaysApplies={onAlwaysApplies} onEverywhere={onEverywhere} />
          <CardBack item={item} state={state} siblings={sameMessage(all, item)} onDeclineAll={onDeclineAll} />
        </div>
        <Stamp label="Save" tone="border-teal text-teal" side="left-6 -rotate-12" amount={upward ? 0 : stamp(drag.x)} />
        <Stamp label="Nope" tone="border-danger text-danger" side="right-6 rotate-12" amount={upward ? 0 : stamp(-drag.x)} />
        <Stamp label="Skip" tone="border-hair-input text-muted-text" side="left-1/2 -translate-x-1/2" amount={upward ? stamp(-drag.y) : 0} />
      </div>
    </div>
    <div className="mt-8 flex items-end gap-5">
      <RoundButton label="Decline" caption="Decline" keyHint="←" size="big" tone="text-danger hover:border-danger" disabled={busy} onClick={() => move("decline")}><Cross size={20} /></RoundButton>
      {queue.length > 1 && <RoundButton label="Skip" caption="Skip" keyHint="↑" disabled={busy} onClick={() => move("skip")}><ArrowUp size={16} /></RoundButton>}
      <RoundButton label={flipped ? "Show the memory" : "Show where it came from"} caption="Flip" keyHint="Space" onClick={() => setFlipped(!flipped)}><span className="text-base leading-none">⟲</span></RoundButton>
      <RoundButton label="Save" caption="Save" keyHint="→" size="big" tone="text-teal hover:border-teal" disabled={busy || !item.approvable} onClick={() => move("save")}><Check size={20} /></RoundButton>
    </div>
  </div>;
}

function Stamp({ label, tone, side, amount }: { label: string; tone: string; side: string; amount: number }) {
  if (amount <= 0) return null;
  return <span aria-hidden="true" className={`bg-card pointer-events-none absolute top-16 rounded-lg border-2 px-3 py-1 text-lg font-bold tracking-widest uppercase ${tone} ${side}`}
    style={{ opacity: amount }}>{label}</span>;
}

function RoundButton({ label, caption, keyHint, size, tone = "text-body hover:border-hair-input", disabled, onClick, children }: {
  label: string; caption: string; keyHint: string; size?: "big"; tone?: string; disabled?: boolean; onClick: () => void; children: ReactNode;
}) {
  return <span className="flex flex-col items-center gap-1.5">
    <button type="button" aria-label={label} title={`${label} (${keyHint})`} disabled={disabled} onClick={onClick}
      className={`border-hair bg-card flex items-center justify-center rounded-full border shadow-md transition-transform hover:scale-105 active:scale-95 disabled:opacity-30 disabled:hover:scale-100 ${size === "big" ? "h-14 w-14" : "h-10 w-10"} ${tone}`}>
      {children}
    </button>
    <span aria-hidden="true" className="text-faint text-[10px]">{caption}</span>
  </span>;
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

/** read is what a card shows of one proposal. */
function useCardRead(item: MemoryProposalItem, state: ProposalInboxState) {
  const { ownerName, names } = useMemoryPresentation();
  const everywhere = state.everywhere?.[switchKey(item)];
  const shown = version(item, everywhere);
  const view = readApprovalArgs(item.tool_name, shown.proposal, ownerName);
  const classification = view.shape === "memory" || view.shape === "json" ? view.classification : undefined;
  const change: MemoryProposalChange | undefined = item.change?.action === "update" || item.change?.action === "retire" ? item.change : undefined;
  const line = change
    ? splitLine(ownerText(change.action === "update" ? change.replacement : change.current, ownerName) || "a memory that can no longer be read", ownerName)
    : view.shape === "memory" ? splitLine(view.statement, ownerName) : { label: "", value: "A change to memory" };
  return {
    names, everywhere: Boolean(everywhere), shown, view, classification, change, line,
    was: change?.action === "update" ? splitLine(ownerText(change.current, ownerName), ownerName).value : "",
    evieWording: change ? change.evie_proposed === true && !change.evidence : (view.shape === "memory" || view.shape === "json") && view.evieProposed === true,
    yourWords: change?.evidence || (view.shape === "memory" ? view.evidence : ""),
    alwaysApplies: state.alwaysApplies[switchKey(item)] ?? classification?.standing ?? false,
    json: view.shape === "memory" || view.shape === "json" ? view.json : shown.proposal,
  };
}

const face = "border-hair bg-card absolute inset-0 flex flex-col rounded-2xl border p-6 shadow-xl [backface-visibility:hidden]";
const pill = "border-hair text-muted-text rounded-full border px-2 py-px text-[11px] whitespace-nowrap";

function CardFace({ item, state, refused, onAlwaysApplies, onEverywhere }: {
  item: MemoryProposalItem; state: ProposalInboxState; refused: boolean;
  onAlwaysApplies: (item: MemoryProposalItem, value: boolean) => void;
  onEverywhere?: (item: MemoryProposalItem, value: boolean) => void;
}) {
  const read = useCardRead(item, state);
  const editable = item.approvable && !state.busy;
  return <div className={face}>
    <div className="flex flex-wrap items-center gap-1.5">
      <span className={pill}>{placeName(read.shown.scope_key, read.names)}</span>
      {read.classification && <span className={pill}>{kindLabel(read.classification.kind)}</span>}
      {read.change && <span className={pill}>{read.change.action === "update" ? "Update" : "Forget"}</span>}
      {read.evieWording && <span title="You didn’t say it in these words. Saving keeps it as Evie’s proposal, not as something you said."
        className="border-amber-hair-strong text-amber-ink rounded-full border px-2 py-px text-[11px] whitespace-nowrap">Evie’s wording</span>}
    </div>
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center text-center">
      {read.line.label && <p className="text-muted-text text-xs tracking-wide">{read.line.label}</p>}
      <p className="text-ink mt-2 line-clamp-5 text-xl leading-snug font-medium">{read.change?.action === "retire" ? "Forget: " : ""}{read.line.not ? "Not: " : ""}{read.line.value}</p>
      {read.was && <p className="text-muted-text mt-2 text-xs line-through" aria-label="Was">{read.was}</p>}
    </div>
    {(refused || !item.approvable) && item.problem && <p className="text-amber-ink mb-2 text-center text-xs">Can’t save: {item.problem}</p>}
    {item.reason === "no_explicit_cue" && <p className="text-amber-ink mb-2 text-center text-xs">You didn’t ask Evie to remember this.</p>}
    <div className="flex flex-wrap items-center justify-center gap-2" data-no-drag>
      {read.classification && <Toggle label="Always applies" title={read.alwaysApplies ? "Evie sees this on every request." : "Evie recalls this only when it’s relevant."}
        checked={read.alwaysApplies} onChange={editable ? (value) => onAlwaysApplies(item, value) : undefined} />}
      {item.everywhere && <Toggle label="Everywhere instead" title={`Evie thinks this may apply beyond ${scopeLabel(item.scope_key, read.names)}.`}
        checked={read.everywhere} onChange={editable && onEverywhere ? (value) => onEverywhere(item, value) : undefined} />}
    </div>
  </div>;
}

function Toggle({ label, title, checked, onChange }: { label: string; title: string; checked: boolean; onChange?: (value: boolean) => void }) {
  return <button type="button" role="switch" aria-checked={checked} aria-label={label} title={title} disabled={!onChange} onClick={() => onChange?.(!checked)}
    className={`flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs disabled:opacity-50 ${checked ? "border-teal-hair text-teal bg-teal-deep/40" : "border-hair text-muted-text hover:text-body"}`}>
    <span className={`h-1.5 w-1.5 rounded-full ${checked ? "bg-teal" : "bg-hair-strong"}`} />
    {label}
  </button>;
}

function CardBack({ item, state, siblings, onDeclineAll }: {
  item: MemoryProposalItem; state: ProposalInboxState; siblings: MemoryProposalItem[];
  onDeclineAll?: (items: MemoryProposalItem[]) => void;
}) {
  const read = useCardRead(item, state);
  const queued = new Date(item.queued_at);
  return <div className={`${face} [transform:rotateY(180deg)]`}>
    <p className="text-muted-text text-[11px]">{item.session_title || "A conversation"}{Number.isNaN(queued.getTime()) ? "" : ` · ${queued.toLocaleString()}`}</p>
    <div className="mt-3 min-h-0 flex-1 overflow-auto" data-no-drag>
      {read.yourWords && <>
        <p className="text-muted-text text-[11px]">Your words</p>
        <p className="text-ink mt-1 text-sm">“{read.yourWords}”</p>
      </>}
      {item.source_message && <>
        <p className={`text-muted-text text-[11px] ${read.yourWords ? "mt-3" : ""}`}>From your message</p>
        <p className="text-body mt-1 text-sm whitespace-pre-wrap">{item.source_message}</p>
      </>}
      {read.view.shape === "memory" && read.view.identities.length > 0 && <ul className="text-muted-text mt-3 space-y-0.5 text-xs">{read.view.identities.map((identity) => <li key={identity}>{identity}</li>)}</ul>}
    </div>
    <div className="text-muted-text mt-3 flex flex-wrap items-center gap-3 text-xs" data-no-drag>
      {siblings.length > 1 && onDeclineAll && <button type="button" disabled={state.busy} onClick={() => onDeclineAll(siblings)} className="hover:text-body underline underline-offset-2 disabled:opacity-40">
        Decline all {siblings.length} from this message</button>}
      <details className="min-w-0 flex-1">
        <summary className="hover:text-body cursor-pointer">Exact change{item.revision > 1 ? " · updated against newer memory" : ""}</summary>
        <pre className="mt-2 max-h-28 overflow-auto text-[11px] whitespace-pre-wrap break-all">{read.json}</pre>
      </details>
    </div>
  </div>;
}
