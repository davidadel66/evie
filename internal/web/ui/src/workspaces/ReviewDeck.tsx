// A run's review question as a deck of cards (run detail amendment,
// 2026-11-02): one proposed line at a time, kept with a swipe right or →,
// rejected with a swipe left or ←, skipped to the back with ↑, its category
// changed or its merchant rule dropped on the card. Nothing is sent until
// the owner presses Answer on the summary; the answer is exactly the
// step's schema: reject, change and no_rule lists, empty ones left out.
// The deck appears for any question whose input holds a list of objects
// with ids and whose schema names those lists, so finance-review gets it
// without the UI knowing finance.

import { useEffect, useRef, useState, type ReactNode } from "react";
import { ArrowUp, Check, Cross } from "../ui/Icon";
import { reviewAnswer, reviewSummary, seenCategories, type ReviewDecision, type ReviewLine } from "./reviewLines";

const text = (value: unknown): string => typeof value === "string" ? value : value === undefined || value === null ? "" : String(value);

function money(cents: unknown): string {
  if (typeof cents !== "number") return "";
  const sign = cents < 0 ? "−" : "";
  return `${sign}$${(Math.abs(cents) / 100).toLocaleString("en-US", {minimumFractionDigits: 2, maximumFractionDigits: 2})}`;
}

function day(value: unknown): string {
  if (typeof value !== "string") return "";
  const date = new Date(value.length === 10 ? `${value}T00:00:00` : value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString("en-US", {month: "short", day: "numeric"});
}

const known = new Set(["id", "merchant_name", "name", "net_cents", "date", "category", "confidence", "reason", "rule"]);

const swipeAt = 120;
type Move = "keep" | "reject" | "skip";

function reducedMotion(): boolean {
  return typeof window !== "undefined" && window.matchMedia?.("(prefers-reduced-motion: reduce)").matches === true;
}

export function ReviewDeck({lines, busy, onAnswer, onClose}: {
  lines: ReviewLine[];
  busy: boolean;
  onAnswer: (answer: Record<string, unknown>) => void;
  onClose: () => void;
}) {
  const [decisions, setDecisions] = useState<Record<string, ReviewDecision>>({});
  const [noRule, setNoRule] = useState<Set<string>>(() => new Set());
  // Undecided lines in the order they are shown; a skip moves one to the back.
  const [queue, setQueue] = useState<string[]>(() => lines.map((line) => line.id));
  const [changing, setChanging] = useState(false);
  const [drag, setDrag] = useState({x: 0, y: 0, active: false});
  const [leaving, setLeaving] = useState<Move>();
  const start = useRef<{x: number; y: number; moved: boolean} | undefined>(undefined);
  const byId = new Map(lines.map((line) => [line.id, line]));
  const current = byId.get(queue[0] ?? "");
  const done = lines.length - queue.length;
  const categories = seenCategories(lines);

  const decide = (id: string, decision: ReviewDecision) => {
    setDecisions((value) => ({...value, [id]: decision}));
    setQueue((value) => value.filter((other) => other !== id));
  };
  const move = (direction: Move) => {
    if (!current || busy || leaving) return;
    if (direction === "skip" && queue.length < 2) { setDrag({x: 0, y: 0, active: false}); return; }
    setLeaving(direction);
    window.setTimeout(() => {
      if (direction === "keep") decide(current.id, decisions[current.id]?.kind === "change" ? decisions[current.id] : {kind: "keep"});
      else if (direction === "reject") decide(current.id, {kind: "reject"});
      else setQueue((value) => [...value.slice(1), value[0]]);
      setLeaving(undefined); setChanging(false); setDrag({x: 0, y: 0, active: false});
    }, reducedMotion() ? 0 : 220);
  };
  const root = useRef<HTMLDivElement>(null);
  const card = useRef<HTMLDivElement>(null);
  // The card takes focus so the arrow keys work at once, even though the
  // owner opened the deck from a button.
  useEffect(() => { card.current?.focus({preventScroll: true}); }, [queue[0]]);
  useEffect(() => {
    const keys = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (target && (target.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName))) return;
      if (target && /^(BUTTON|SUMMARY|A)$/.test(target.tagName) && !root.current?.contains(target)) return;
      if (event.key === "ArrowRight") move("keep");
      else if (event.key === "ArrowLeft") move("reject");
      else if (event.key === "ArrowUp") move("skip");
      else return;
      event.preventDefault();
    };
    window.addEventListener("keydown", keys);
    return () => window.removeEventListener("keydown", keys);
  });
  const pointerDown = (event: React.PointerEvent) => {
    if (busy || leaving || (event.target as HTMLElement).closest("button, select, input, a")) return;
    start.current = {x: event.clientX, y: event.clientY, moved: false};
    event.currentTarget.setPointerCapture(event.pointerId);
    setDrag({x: 0, y: 0, active: true});
  };
  const pointerMove = (event: React.PointerEvent) => {
    if (!start.current) return;
    const x = event.clientX - start.current.x, y = event.clientY - start.current.y;
    if (Math.abs(x) + Math.abs(y) > 6) start.current.moved = true;
    setDrag({x, y, active: true});
  };
  const pointerUp = () => {
    const begun = start.current;
    start.current = undefined;
    if (!begun) return;
    if (!begun.moved) setDrag({x: 0, y: 0, active: false});
    else if (drag.x > swipeAt) move("keep");
    else if (drag.x < -swipeAt) move("reject");
    else if (drag.y < -swipeAt && Math.abs(drag.y) > Math.abs(drag.x)) move("skip");
    else setDrag({x: 0, y: 0, active: false});
  };

  if (!current) {
    return <div ref={root} className="border-hair bg-card mt-3 rounded-[10px] border px-4 py-3.5">
      <p className="text-ink text-[13px] font-semibold">{reviewSummary(lines, decisions, noRule)}</p>
      <p className="text-muted-text mt-1 text-xs">Nothing is sent until you answer. Rejected lines are not proposed again.</p>
      <div className="mt-3 flex flex-wrap items-center gap-2.5">
        <button type="button" disabled={busy} onClick={() => onAnswer(reviewAnswer(lines, decisions, noRule))} className="bg-teal text-primary-foreground rounded-[7px] px-3.5 py-2 text-[13px] font-semibold disabled:opacity-40">Answer</button>
        <button type="button" onClick={() => { setDecisions({}); setNoRule(new Set()); setQueue(lines.map((line) => line.id)); }} className="text-muted-text hover:text-body text-xs">Review again</button>
        <button type="button" onClick={onClose} className="text-muted-text hover:text-body ml-auto text-xs">Close</button>
      </div>
    </div>;
  }

  const f = current.fields;
  const decision = decisions[current.id];
  const category = decision?.kind === "change" ? decision.category : text(f.category);
  const confidence = typeof f.confidence === "number" ? `${Math.round(f.confidence * 100)}%` : "";
  const extra = Object.entries(f).filter(([key, value]) => !known.has(key) && (typeof value === "string" || typeof value === "number" || typeof value === "boolean"));
  const fly = leaving === "keep" ? "translateX(150%) rotate(20deg)" : leaving === "reject" ? "translateX(-150%) rotate(-20deg)" : leaving === "skip" ? "translateY(-140%)" : "";
  const transform = fly || `translate(${drag.x}px, ${drag.y}px) rotate(${drag.x / 18}deg)`;
  const stamp = (amount: number) => Math.max(0, Math.min(1, amount / swipeAt));
  const upward = -drag.y > Math.abs(drag.x);
  return <div ref={root} className="mt-3">
    <div className="text-muted-text mb-2 flex items-center text-[11.5px]">
      <span>{done + 1} of {lines.length}</span>
      <span className="ml-auto">→ keep · ← reject · ↑ skip</span>
    </div>
    <div className="relative h-[232px] [perspective:1400px]">
      {queue.length > 1 && <div aria-hidden="true" className="border-hair bg-card absolute inset-0 rounded-[12px] border" style={{transform: "translateY(10px) scale(0.96)", opacity: 0.6}} />}
      <div ref={card} tabIndex={-1} role="group" aria-label={`Proposal ${done + 1} of ${lines.length}`} aria-roledescription="card"
        onPointerDown={pointerDown} onPointerMove={pointerMove} onPointerUp={pointerUp} onPointerCancel={() => { start.current = undefined; setDrag({x: 0, y: 0, active: false}); }}
        className="border-hair-bubble bg-card absolute inset-0 flex cursor-grab touch-none flex-col rounded-[12px] border px-4 py-3.5 shadow-lg outline-none select-none active:cursor-grabbing"
        style={{transform, transition: drag.active ? "none" : "transform 220ms ease, opacity 220ms ease", opacity: fly ? 0 : 1}}>
        <div className="flex items-baseline gap-3">
          <span className="text-ink min-w-0 flex-1 truncate text-[15px] font-semibold">{text(f.merchant_name) || text(f.name) || current.id}</span>
          <span className="text-ink flex-none text-[15px] font-medium tabular-nums">{money(f.net_cents)}</span>
        </div>
        <div className="text-fainter mt-0.5 flex items-baseline gap-3 font-mono text-[11px]">
          <span className="min-w-0 flex-1 truncate">{text(f.merchant_name) ? text(f.name) : ""}</span>
          <span className="flex-none">{day(f.date)}</span>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          {changing
            ? <select autoFocus aria-label="Category" value={category} onChange={(event) => { const value = event.target.value; if (value === "__other") { setChanging(true); return; } setDecisions((d) => ({...d, [current.id]: value === text(f.category) ? {kind: "keep"} : {kind: "change", category: value}})); setChanging(false); }}
              className="border-hair-input bg-code text-ink rounded-[6px] border px-2 py-1 text-xs">
              {[...new Set([text(f.category), ...categories])].filter(Boolean).map((option) => <option key={option} value={option}>{option}</option>)}
            </select>
            : <button type="button" onClick={() => setChanging(true)} title="Change the category" className={`${decision?.kind === "change" ? "border-teal text-teal" : "border-teal-hair text-teal"} rounded-full border px-2.5 py-0.5 text-xs font-medium`}>{category || "No category"}{decision?.kind === "change" ? " · changed" : ""}</button>}
          {confidence && <span className="text-fainter text-[11px]">{confidence} sure</span>}
          {f.rule === true && <button type="button" aria-pressed={noRule.has(current.id)} onClick={() => setNoRule((value) => { const next = new Set(value); if (next.has(current.id)) next.delete(current.id); else next.add(current.id); return next; })}
            className={`${noRule.has(current.id) ? "border-hair-input text-muted-text line-through" : "border-hair-input text-body"} ml-auto rounded-full border px-2.5 py-0.5 text-[11px]`}>Merchant rule</button>}
        </div>
        {changing && <input aria-label="Another category" placeholder="Or type a category and press Enter" onKeyDown={(event) => { if (event.key === "Enter") { const value = (event.target as HTMLInputElement).value.trim(); if (value) { setDecisions((d) => ({...d, [current.id]: {kind: "change", category: value}})); setChanging(false); } } if (event.key === "Escape") setChanging(false); }}
          className="border-hair-input bg-code text-ink placeholder:text-fainter mt-2 w-full rounded-[6px] border px-2 py-1 text-xs outline-none" />}
        {text(f.reason) && <p className="text-muted-text mt-3 line-clamp-3 text-[12.5px] leading-[1.5]">{text(f.reason)}</p>}
        {extra.length > 0 && <p className="text-fainter mt-2 truncate text-[11px]">{extra.map(([key, value]) => `${key} ${String(value)}`).join(" · ")}</p>}
        <Stamp label="Keep" tone="border-teal text-teal" side="left-6 -rotate-12" amount={upward ? 0 : stamp(drag.x)} />
        <Stamp label="Reject" tone="border-danger text-danger" side="right-6 rotate-12" amount={upward ? 0 : stamp(-drag.x)} />
        <Stamp label="Skip" tone="border-hair-input text-muted-text" side="left-1/2 -translate-x-1/2" amount={upward ? stamp(-drag.y) : 0} />
      </div>
    </div>
    <div className="mt-4 flex items-end justify-center gap-5">
      <Round label="Reject" keyHint="←" size="big" tone="text-danger hover:border-danger" disabled={busy} onClick={() => move("reject")}><Cross size={18} /></Round>
      {queue.length > 1 && <Round label="Skip" keyHint="↑" disabled={busy} onClick={() => move("skip")}><ArrowUp size={15} /></Round>}
      <Round label="Keep" keyHint="→" size="big" tone="text-teal hover:border-teal" disabled={busy} onClick={() => move("keep")}><Check size={18} /></Round>
    </div>
    <div className="mt-3 flex items-center text-[11.5px]">
      <span className="text-fainter">{reviewSummary(lines, decisions, noRule)}</span>
      <button type="button" onClick={onClose} className="text-muted-text hover:text-body ml-auto">Close</button>
    </div>
  </div>;
}

function Stamp({label, tone, side, amount}: {label: string; tone: string; side: string; amount: number}) {
  if (amount <= 0) return null;
  return <span aria-hidden="true" className={`bg-card pointer-events-none absolute top-10 rounded-lg border-2 px-3 py-1 text-base font-bold tracking-widest uppercase ${tone} ${side}`} style={{opacity: amount}}>{label}</span>;
}

function Round({label, keyHint, size, tone = "text-body hover:border-hair-input", disabled, onClick, children}: {
  label: string; keyHint: string; size?: "big"; tone?: string; disabled?: boolean; onClick: () => void; children: ReactNode;
}) {
  return <span className="flex flex-col items-center gap-1">
    <button type="button" aria-label={label} title={`${label} (${keyHint})`} disabled={disabled} onClick={onClick}
      className={`border-hair bg-card flex items-center justify-center rounded-full border shadow-md transition-transform hover:scale-105 active:scale-95 disabled:opacity-30 ${size === "big" ? "h-12 w-12" : "h-9 w-9"} ${tone}`}>{children}</button>
    <span aria-hidden="true" className="text-faint text-[10px]">{label}</span>
  </span>;
}
