import { scopeLabel, useMemoryPresentation } from "../memory/presentation";
import type { MemoryToolResult as Result } from "./memoryToolResult";

export function MemoryResultView({result}: {result: Result}) {
  const {names} = useMemoryPresentation();
  if (result.kind === "search") return <div className="space-y-2 text-sm">
    <ReadFilters result={result} />
    <p className="text-body">{searchOutcome(result)}</p>
    {result.status === "empty" && <p className="text-muted-text text-xs leading-5">No matches for this search. Saved records may still exist.</p>}
    {result.truncated && <p className="text-amber-ink text-xs">Results were limited.</p>}
  </div>;
  return <section aria-label="Recorded memory results" className="space-y-3">
    <h3 className="text-body text-sm font-medium">{result.records.length} {result.records.length === 1 ? "record" : "records"} returned</h3>
    <p className="text-muted-text text-xs leading-5">Recorded by this tool at the time of the turn.</p>
    <ReadFilters result={result} />
    <div className="max-h-[32rem] space-y-4 overflow-y-auto">
      {result.records.map((record, index) => <article key={index} className="border-hair border-l-2 pl-3">
        <p className="text-body text-sm leading-6 whitespace-pre-wrap break-words">{record.polarity === "denied" && <strong className="text-amber-ink">Not: </strong>}{record.value}</p>
        {(record.subject || record.predicate) && <p className="text-muted-text mt-1 text-xs">{[record.subject, record.predicate].filter(Boolean).join(" · ")}</p>}
        <p className="text-muted-text mt-1 text-xs">{record.scope ? scopeLabel(record.scope, names) : "Scope not recorded"} · {record.status}</p>
        {record.conflicts > 0 && <p className="text-amber-ink mt-1 text-xs">{record.conflicts} {record.conflicts === 1 ? "conflict warning" : "conflict warnings"} recorded</p>}
        {(record.validFrom || record.validTo) && <p className="text-muted-text mt-1 text-xs">Valid from: {record.validFrom ?? "Unknown"} · until: {record.validTo ?? "Unknown"}</p>}
        {record.polarity === "unknown" && <p className="text-amber-ink mt-1 text-xs">Polarity not recorded</p>}
      </article>)}
    </div>
    {result.more && <p className="text-amber-ink text-xs">More records existed beyond this returned page.</p>}
  </section>;
}

function ReadFilters({result}: {result: Result}) {
  if (!result.validAt && !result.asKnownAt) return null;
  return <div className="text-muted-text space-y-1 text-xs">
    <p>Read filters</p>
    {result.validAt && <p>Valid at: {result.validAt}</p>}
    {result.asKnownAt && <p>Known by: {result.asKnownAt}</p>}
  </div>;
}

function searchOutcome(result: Extract<Result, {kind: "search"}>) {
  const count = `${result.matches} ${result.matches === 1 ? "match" : "matches"}`;
  if (result.status === "empty" || result.status === "success") return `${count} returned`;
  const outcome = result.status === "partial" ? "Search partially completed" : result.status === "failed" ? "Search failed" : result.status === "unavailable" ? "Search unavailable" : result.status === "cancelled" ? "Search cancelled" : result.status === "exhausted" ? "Search budget exhausted" : "Search outcome not recognized";
  return `${outcome} · ${count} returned`;
}
