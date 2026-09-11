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
  const [selectedId, setSelectedId] = useState(receipt.snapshotId);
  const selected = receipt.requests?.find((request) => request.snapshotId === selectedId) ?? receipt.requests?.find((request) => request.snapshotId === receipt.snapshotId);
  const displayed = selected ? { ...receipt, ...selected } : receipt;
  return <div className="p-5">
    <h2 className="text-body text-sm font-medium">{receipt.answerId ? "Answer sources" : "Request sources"}</h2>
    {(receipt.requests?.length ?? 0) > 0 && <nav aria-label="Original provider requests" className="mt-3 flex flex-wrap gap-2">
      {receipt.requests?.map((request, index) => <button key={request.snapshotId} type="button" aria-pressed={request.snapshotId === selected?.snapshotId}
        className="border-hair text-muted-text aria-pressed:text-teal cursor-pointer rounded border px-2 py-1 text-xs" onClick={() => setSelectedId(request.snapshotId)}>
        Request {request.iteration || index + 1} · {requestLabel(request.requestStatus)}
      </button>)}
    </nav>}
    <RequestEvidenceView receipt={displayed} requestStatus={selected?.requestStatus} />
    {selected && <details className="text-muted-text mt-5 break-all text-[11px] leading-5">
      <summary className="cursor-pointer">Original request reference</summary>
      <p>{selected.snapshotId}<br />{selected.responseId && <>Response: {selected.responseId}<br /></>}{selected.requestSHA256}<br />{selected.serializedBytes} bytes · {selected.version || "No memory retrieval"}</p>
    </details>}
  </div>;
}

function requestLabel(status: string) {
  return status === "completed" ? "Completed response" : status === "interrupted" ? "Request interrupted" : "Prepared for request";
}

function RequestEvidenceView({ receipt, requestStatus }: { receipt: MemoryEvidenceReceipt; requestStatus?: string }) {
  const heading = requestStatus === "completed" ? "Supplied to model" : requestStatus === "interrupted" ? "Request interrupted" : requestStatus === "prepared" ? "Prepared for request" : "Recorded request evidence";
  const explanation = requestStatus === "completed" ? "Evidence included with a completed request. This does not establish that the answer cited or used each source."
    : requestStatus === "interrupted" ? "This saved request did not produce a committed response. Provider delivery and source use are unconfirmed."
    : "This request was saved before dispatch. No completed response is recorded.";
  return <div className="mt-4">
    <h3 className="text-body text-sm font-medium">{heading}</h3>
    <p className="text-muted-text mt-2 text-xs leading-5">{explanation}</p>
    {receipt.evidence.length === 0 && <p className="text-muted-text mt-4 text-xs">{receipt.status === "empty" ? "The search returned no matches." : "No evidence is recorded for this request."}</p>}
    {receipt.evidence.map((item) => <section key={item.reference.id} className="border-hair mt-5 border-t pt-4">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <h3 className="text-teal">{item.reference.kind === "conversation_excerpt" ? "Conversation excerpt" : "Accepted memory"}</h3>
        {item.reference.intent === "historical" && <span className="text-muted-text">Historical</span>}
        {(item.reference.status === "retired" || item.current_status === "retired" || item.reference.current_status === "retired") && <span className="text-amber-ink">{item.current_status === "retired" ? "Retired now" : item.reference.current_status === "retired" ? "Retired at retrieval" : "Retired in historical view"}</span>}
      </div>
      {!item.available || !item.evidence ? <p className="text-muted-text mt-2 text-xs">Source unavailable under current access.</p> : <>
        <p className="text-body mt-2 text-sm leading-6 whitespace-pre-wrap">{item.evidence.text}</p>
        {item.reference.kind === "accepted_memory" && item.reference.claim_id && <details className="text-muted-text mt-2 break-all text-[11px] leading-5">
          <summary className="cursor-pointer">Original Claim reference</summary>
          <p>Claim ID: {item.reference.claim_id}<br />Claim version: {item.reference.claim_operation_id || "Unavailable"}</p>
        </details>}
        {item.reference.kind === "conversation_excerpt" && <p className="text-muted-text mt-2 text-xs">Attributed conversation evidence; preserves what was said and its uncertainty.</p>}
        {(item.reference.conflicts?.length ?? 0) > 0 && <div className="text-amber-ink mt-3 text-xs leading-5">
          <p>Conflicting accepted Claims</p>
          {item.reference.conflicts?.map((conflict) => <p key={`${conflict.code}:${conflict.claim_ids.join(":")}`}>{conflict.predicate_token}: {conflict.code === "opposite_polarity" ? "Opposite assertions" : conflict.code === "one_cardinality_overlap" ? "Different accepted values" : "Conflicting evidence"} · {conflict.claim_ids.join(", ")}</p>)}
        </div>}
        {item.reference.paths.includes("newer_owner_statement") && (item.reference.related_claim_ids?.length ?? 0) > 0 && <div className="text-amber-ink mt-3 text-xs leading-5">
          <p>Potential discrepancy</p>
          <p>Newer owner wording to compare with accepted Claims: {item.reference.related_claim_ids?.join(", ")}. Recency alone does not establish a correction.</p>
        </div>}
        <p className="text-muted-text mt-2 text-xs">Original state: {item.reference.status}{item.reference.current_status && item.reference.current_status !== item.reference.status && <> · At retrieval: {item.reference.current_status}</>}{item.current_status && (item.current_status !== item.reference.status || item.current_status !== (item.reference.current_status ?? item.reference.status)) && <> · Current state: {item.current_status}</>}</p>
        {(item.reference.correction_mode || item.reference.current_correction_mode || item.evidence.current_correction_mode) && <div className="text-muted-text mt-2 text-xs leading-5">
          {item.reference.correction_mode && <p>Original correction: {correctionLabel(item.reference.correction_mode)}</p>}
          {item.reference.current_correction_mode && item.reference.current_correction_mode !== item.reference.correction_mode && <p>Correction at retrieval: {correctionLabel(item.reference.current_correction_mode)}</p>}
          {item.evidence.current_correction_mode && item.evidence.current_correction_mode !== item.reference.current_correction_mode && <p>Current correction: {correctionLabel(item.evidence.current_correction_mode)}</p>}
        </div>}
        {item.reference.kind === "accepted_memory" && item.evidence.claim && <div className="text-muted-text mt-2 text-xs leading-5">
          <p>Accepted on: {item.evidence.claim.transaction_time}</p>
          <p>Valid from: {(item.evidence.effective_valid_time ?? item.evidence.claim.valid_time).from ?? "Unknown"} · Valid until: {(item.evidence.effective_valid_time ?? item.evidence.claim.valid_time).to ?? "Unknown"}</p>
        </div>}
        {(item.reference.intent === "historical" || item.reference.valid_at_constrained) && <details className="text-muted-text mt-2 text-xs leading-5">
          <summary className="cursor-pointer">Read filters</summary>
          <p>Known by: {item.reference.as_known_at}</p>
          {item.reference.valid_at_constrained && <p>Valid at: {item.reference.valid_at}</p>}
        </details>}
        {item.evidence.sources.map((source) => <div key={`${source.event_id}:${source.locator_value}`} className="border-hair mt-4 border-l pl-3">
          <blockquote className="text-body text-xs leading-5 whitespace-pre-wrap">{source.evidence || "Source text unavailable."}</blockquote>
          <p className="text-muted-text mt-2 text-xs">{source.actor && <>Speaker: {source.actor} · </>}Authority: {source.authority} · {source.observed_at}</p>
          <details className="text-muted-text mt-2 break-all text-[11px]"><summary className="cursor-pointer">Source reference</summary>
            <p>{source.event_id}<br />{source.session_id}<br />{source.source_scope_key}<br />{source.locator_kind}{source.locator_value && ` ${source.locator_value}`}<br />{source.evidence_sha256}</p>
          </details>
        </div>)}
      </>}
    </section>)}
  </div>;
}

function correctionLabel(mode: string) {
  return mode === "error" ? "Corrected as an error" : mode === "changed" ? "Changed over time" : mode;
}
