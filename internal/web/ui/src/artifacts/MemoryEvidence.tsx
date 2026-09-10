import { useEffect, useState } from "react";
import { inspectMemoryEvidence, type MemoryEvidenceReceipt } from "../api/memoryEvidence";

export function MemoryEvidence({ sessionId, snapshotId }: { sessionId: string; snapshotId: string }) {
  const [receipt, setReceipt] = useState<MemoryEvidenceReceipt>();
  const [problem, setProblem] = useState<string>();
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    inspectMemoryEvidence(sessionId, snapshotId, controller.signal).then((value) => {
      if (!controller.signal.aborted) { setReceipt(value); setProblem(undefined); }
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) { setReceipt(undefined); setProblem(error instanceof Error ? error.message : "Original memory evidence is unavailable."); }
    });
    return () => controller.abort();
  }, [sessionId, snapshotId, attempt]);
  if (problem) return <div className="p-5"><p role="alert" className="text-amber-ink text-xs">{problem}</p><button type="button" className="text-teal mt-3 text-xs underline" onClick={() => setAttempt((value) => value + 1)}>Retry source inspection</button></div>;
  if (!receipt) return <p className="text-muted-text p-5 text-xs">Loading original evidence…</p>;
  return <MemoryEvidenceView receipt={receipt} />;
}

export function MemoryEvidenceView({ receipt }: { receipt: MemoryEvidenceReceipt }) {
  return <div className="p-5">
    <h2 className="text-body text-sm font-medium">Supplied to model</h2>
    <p className="text-muted-text mt-2 text-xs leading-5">Evidence included in this request. An explicit answer citation is recorded separately when present.</p>
    {receipt.evidence.length === 0 && <p className="text-muted-text mt-4 text-xs">{receipt.status === "empty" ? "The search returned no matches." : "No evidence was supplied by this search."}</p>}
    {receipt.evidence.map((item) => <section key={item.reference.id} className="border-hair mt-5 border-t pt-4">
      <h3 className="text-teal text-xs">Accepted memory</h3>
      {!item.available || !item.evidence ? <p className="text-muted-text mt-2 text-xs">Source unavailable under current access.</p> : <>
        <p className="text-body mt-2 text-sm leading-6 whitespace-pre-wrap">{item.evidence.text}</p>
        <p className="text-muted-text mt-2 text-xs">Original state: {item.reference.status}{item.current_status !== item.reference.status && <> · Current state: {item.current_status}</>}</p>
        {item.evidence.sources.map((source) => <div key={`${source.event_id}:${source.locator_value}`} className="border-hair mt-4 border-l pl-3">
          <blockquote className="text-body text-xs leading-5 whitespace-pre-wrap">{source.evidence || "Source text unavailable."}</blockquote>
          <p className="text-muted-text mt-2 text-xs">{source.authority} · {source.observed_at}</p>
          <details className="text-muted-text mt-2 break-all text-[11px]"><summary className="cursor-pointer">Source reference</summary>
            <p>{source.event_id}<br />{source.source_scope_key}<br />{source.locator_kind}{source.locator_value && ` ${source.locator_value}`}<br />{source.evidence_sha256}</p>
          </details>
        </div>)}
      </>}
    </section>)}
  </div>;
}
