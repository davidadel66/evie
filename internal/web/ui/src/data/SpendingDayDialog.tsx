import { useEffect, useId, useRef, useState } from "react";
import { inspectSpendingDay, SpendingCategoryError, updateSpendingCategory, type DayTransaction, type SpendingDayPage } from "../api/spending";
import { dayLabel, formatFlow } from "./spendingPresentation";

const control = "border-hair-strong bg-card text-body focus-visible:ring-teal rounded-md border px-3 py-2 text-xs focus-visible:ring-2 focus-visible:outline-none disabled:cursor-default disabled:opacity-40";

export function SpendingDayDialog({ date, onClose, onSaved }: { date: string; onClose: () => void; onSaved: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);
  const [offset, setOffset] = useState(0);
  const [page, setPage] = useState<SpendingDayPage>();
  const [loading, setLoading] = useState(true);
  const [problem, setProblem] = useState("");
  const [retry, setRetry] = useState(0);
  const [saving, setSaving] = useState(false);
  const title = useId();
  useEffect(() => {
    const element = dialog.current;
    const previous = document.activeElement;
    element?.showModal();
    return () => {
      element?.close();
      if (previous instanceof HTMLElement && previous.isConnected) previous.focus({ preventScroll: true });
    };
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setProblem("");
    inspectSpendingDay({ date, offset, pageSize: 50 }, controller.signal).then((next) => {
      if (!controller.signal.aborted) setPage(next);
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setProblem(error instanceof Error ? error.message : "Day transactions could not be loaded.");
    }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [date, offset, retry]);
  const matching = page?.date === date && page.offset === offset ? page : undefined;
  const close = () => { if (!saving) onClose(); };
  const reload = () => { closeButton.current?.focus(); setPage(undefined); setRetry((value) => value + 1); onSaved(); };
  const goToPage = (next: number) => { closeButton.current?.focus(); setOffset(next); };
  return <dialog ref={dialog} aria-labelledby={title} onCancel={(event) => { event.preventDefault(); close(); }} onClick={(event) => { if (event.target === event.currentTarget) close(); }} className="border-hair bg-app text-body fixed inset-0 m-auto max-h-[90dvh] w-[calc(100%_-_2rem)] max-w-4xl overflow-hidden rounded-xl border p-0 shadow-2xl backdrop:bg-black/60">
    <div className="flex max-h-[90dvh] min-h-0 flex-col">
      <header className="border-hair flex shrink-0 items-start justify-between gap-4 border-b px-5 py-4">
        <div className="min-w-0">
          <h2 id={title} className="text-ink text-base font-medium">{dayLabel(date)}</h2>
          {matching && <p className="text-muted-text mt-1 text-xs tabular-nums">{matching.total} {matching.total === 1 ? "transaction" : "transactions"}<span className="ml-4">Net <span className="text-ink">{formatFlow(matching.summary.netCents)}</span></span></p>}
        </div>
        <div className="flex shrink-0 gap-2">
          <button type="button" className={control} onClick={reload} disabled={saving || loading} aria-label="Reload day transactions">Reload</button>
          <button ref={closeButton} type="button" className={control} onClick={close} disabled={saving} aria-label="Close day transactions"><span aria-hidden="true">✕</span></button>
        </div>
      </header>
      <div className="min-h-0 overflow-y-auto px-5 py-2">
        {problem && <p role="alert" className="text-amber-ink my-4 text-xs">{problem}<button type="button" onClick={reload} className="ml-3 underline">Retry</button></p>}
        {loading && <p role="status" className="text-muted-text py-8 text-center text-xs">Loading…</p>}
        {!loading && matching?.total === 0 && <p className="text-muted-text py-10 text-center text-xs">No posted transactions.</p>}
        {matching && !loading && <DayTransactionRows rows={matching.transactions} categories={matching.categories} busy={saving}
          onSaving={setSaving} onReload={reload} onSaved={(updated) => {
            setPage((current) => current ? { ...current, transactions: current.transactions.map((row) => row.id === updated.id ? updated : row) } : current);
            onSaved();
          }} />}
      </div>
      {matching && matching.total > 0 && <footer className="border-hair text-muted-text flex shrink-0 flex-wrap items-center justify-between gap-3 border-t px-5 py-3 text-xs">
        <span>{matching.transactions.length ? `${offset + 1}–${offset + matching.transactions.length} of ${matching.total}` : `${matching.total} transactions`}</span>
        <div className="flex gap-2"><button type="button" className={control} disabled={loading || saving || offset === 0} onClick={() => goToPage(Math.max(0, offset - matching.pageSize))}>Previous page</button><button type="button" className={control} disabled={loading || saving || !matching.hasMore} onClick={() => goToPage(offset + matching.pageSize)}>Next page</button></div>
      </footer>}
    </div>
  </dialog>;
}

type RowProps = { rows: DayTransaction[]; categories: string[]; busy: boolean; onSaving: (value: boolean) => void; onReload: () => void; onSaved: (row: DayTransaction) => void };

export function DayTransactionRows({ rows, categories, busy, onSaving, onReload, onSaved }: RowProps) {
  return <>
    <div aria-hidden="true" className="border-hair text-muted-text hidden grid-cols-[minmax(0,1fr)_7rem_minmax(15rem,1fr)] gap-4 border-b py-3 text-[11px] sm:grid"><span>Transaction / Description</span><span className="text-right">Amount</span><span>Category</span></div>
    <ul className="divide-hair divide-y">{rows.map((row) => <TransactionRow key={row.id} row={row} categories={categories} busy={busy} onSaving={onSaving} onReload={onReload} onSaved={onSaved} />)}</ul>
  </>;
}

function TransactionRow({ row, categories, busy, onSaving, onReload, onSaved }: Omit<RowProps, "rows"> & { row: DayTransaction }) {
  const [editing, setEditing] = useState<string | null | undefined>();
  const [category, setCategory] = useState("");
  const [problem, setProblem] = useState("");
  const [conflict, setConflict] = useState(false);
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);
  const editTrigger = useRef<HTMLButtonElement>(null);
  const focusEntry = useRef<string | null | undefined>(undefined);
  const restoreEditorFocus = useRef(false);
  useEffect(() => {
    if (editing === undefined && restoreEditorFocus.current) {
      editTrigger.current?.focus();
      restoreEditorFocus.current = false;
    }
  }, [editing, row.revision]);
  const merchant = row.merchantName || row.name || "Unnamed transaction";
  const entries = row.entries.length ? row.entries : [{ id: null, category: "", amountCents: (-BigInt(row.netCents)).toString(), source: "" }];
  const save = async (entryId: string | null) => {
    if (savingRef.current || busy || conflict || !category) return;
    savingRef.current = true; setSaving(true); onSaving(true); setProblem("");
    try {
      const updated = await updateSpendingCategory({ transactionId: row.id, entryId, category, revision: row.revision });
      focusEntry.current = entryId ?? updated.entries[0]?.id;
      restoreEditorFocus.current = true;
      setEditing(undefined);
      onSaved(updated);
    } catch (error: unknown) {
      setProblem(error instanceof SpendingCategoryError ? error.message : "Category could not be saved. Reload to check its current value.");
      setConflict(error instanceof SpendingCategoryError && error.conflict);
    } finally {
      savingRef.current = false; setSaving(false); onSaving(false);
    }
  };
  return <li className="grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-3 py-4 sm:grid-cols-[minmax(0,1fr)_7rem_minmax(15rem,1fr)]">
    <div className="min-w-0"><p className="text-body break-words text-xs font-medium">{merchant}</p>{row.name && row.name !== merchant && <p className="text-muted-text mt-1 break-words text-xs">{row.name}</p>}</div>
    <p className={`${BigInt(row.netCents) > 0n ? "text-teal-hover" : "text-body"} max-w-44 overflow-x-auto whitespace-nowrap text-right text-xs tabular-nums`}>{formatFlow(row.netCents)}</p>
    <div className="col-span-2 min-w-0 space-y-2 sm:col-span-1">
      {entries.map((entry, index) => <div key={entry.id ?? "unclassified"}>
        {editing === entry.id ? <form onSubmit={(event) => { event.preventDefault(); void save(entry.id); }} className="space-y-2">
          <select autoFocus aria-label={`Category for ${merchant}${entries.length > 1 ? `, split ${index + 1}` : ""}`} className={`${control} w-full min-w-0`} value={category} onChange={(event) => setCategory(event.target.value)} disabled={busy || conflict}>
            <option value="" disabled>Choose category</option>
            {entry.category && !categories.includes(entry.category) && <option value={entry.category} disabled>{entry.category}</option>}
            {categories.map((name) => <option value={name} key={name}>{name}</option>)}
          </select>
          <div className="flex items-center gap-2"><button type="submit" className={control} disabled={busy || conflict || !category || category === entry.category}>{saving ? "Saving…" : "Save"}</button><button type="button" className={control} disabled={busy} onClick={() => { restoreEditorFocus.current = true; setEditing(undefined); setProblem(""); setConflict(false); }}>Cancel</button></div>
        </form> : <button ref={focusEntry.current === entry.id ? editTrigger : undefined} type="button" onClick={() => { focusEntry.current = entry.id; setEditing(entry.id); setCategory(entry.category); setProblem(""); setConflict(false); }} disabled={busy || categories.length === 0} className="text-body hover:bg-card focus-visible:ring-teal flex max-w-full items-center gap-2 rounded px-2 py-1 text-left text-xs focus-visible:ring-2 focus-visible:outline-none disabled:opacity-50" aria-label={`Edit category for ${merchant}${entries.length > 1 ? `, split ${index + 1}` : ""}: ${entry.category || "Unclassified"}`}>
          <span className={`${entry.category ? "" : "text-amber-ink"} min-w-0 break-words`}>{entry.category || "Unclassified"}</span><svg aria-hidden="true" className="text-muted-text shrink-0" width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3"><path d="m10.5 2.5 3 3M3 10l7.5-7.5 3 3L6 13l-4 1 1-4Z" /></svg>
        </button>}
        {(entries.length > 1 || (-BigInt(entry.amountCents)).toString() !== row.netCents) && <p className="text-muted-text mt-1 px-2 text-[11px] tabular-nums">{formatFlow((-BigInt(entry.amountCents)).toString())}</p>}
      </div>)}
      {!categories.length && <p className="text-muted-text text-xs">No categories set up.</p>}
      {problem && <p role="alert" className="text-amber-ink text-xs">{problem}<button type="button" onClick={onReload} disabled={busy} className="ml-2 underline">Reload</button></p>}
    </div>
  </li>;
}
