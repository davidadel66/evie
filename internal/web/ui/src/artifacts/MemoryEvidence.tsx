import { useEffect, useState } from "react";
import { inspectMemoryEvidence, type MemoryEvidenceReceipt } from "../api/memoryEvidence";
import { GraphEvidencePath } from "./GraphEvidencePath";
import { memoryEvidenceAnchor } from "./memoryEvidenceAnchor";
import { toolLabel, type ToolItem } from "../chat/activityModel";
import { ToolResultSummary } from "./ToolResultSummary";
import { ToolDebugDetails } from "./ToolInspection";
import { readMemoryToolResult } from "./memoryToolResult";

export function MemoryEvidence({ sessionId, snapshotId, memoryTools = [] }: { sessionId: string; snapshotId: string; memoryTools?: ToolItem[] }) {
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
  if (problem) return <div className="p-5"><MemoryTurnResults tools={memoryTools} /><p role="alert" className="text-amber-ink text-xs">{problem}</p><button type="button" className="text-teal mt-3 text-xs underline" onClick={() => setAttempt((value) => value + 1)}>Retry source inspection</button></div>;
  if (!receipt) return <p className="text-muted-text p-5 text-xs">Loading original evidence…</p>;
  return <MemoryEvidenceView receipt={receipt} memoryTools={memoryTools} />;
}

export function MemoryEvidenceView({ receipt, memoryTools = [] }: { receipt: MemoryEvidenceReceipt; memoryTools?: ToolItem[] }) {
  const [selectedId, setSelectedId] = useState(receipt.snapshotId);
  const selected = receipt.requests?.find((request) => request.snapshotId === selectedId) ?? receipt.requests?.find((request) => request.snapshotId === receipt.snapshotId);
  const displayed = selected ? { ...receipt, ...selected } : receipt;
  return <div className="p-5">
    <MemoryTurnResults tools={memoryTools} />
    <h2 className="text-body text-sm font-medium">Retrieved sources</h2>
    <RequestEvidenceView receipt={displayed} requestStatus={selected?.requestStatus} />
    {((receipt.requests?.length ?? 0) > 0 || memoryTools.length > 0) && <details className="border-hair text-muted-text mt-5 border-t pt-4 text-xs">
      <summary className="cursor-pointer">Debug details</summary>
      <nav aria-label="Original provider requests" className="mt-3 flex flex-wrap gap-2">
      {receipt.requests?.map((request, index) => <button key={request.snapshotId} type="button" aria-pressed={request.snapshotId === selected?.snapshotId}
        className="border-hair text-muted-text aria-pressed:text-teal cursor-pointer rounded border px-2 py-1 text-xs" onClick={() => setSelectedId(request.snapshotId)}>
        Request {request.iteration || index + 1} · {requestLabel(request.requestStatus)}
      </button>)}
      </nav>
    {selected && <div className="text-muted-text mt-5 break-all text-[11px] leading-5">
      <p>Original request reference</p>
      <p>{selected.snapshotId}<br />{selected.responseId && <>Response: {selected.responseId}<br /></>}{selected.requestSHA256}<br />{selected.serializedBytes} bytes · {selected.version || "No memory retrieval"}</p>
    </div>}
    {memoryTools.length > 0 && <div className="border-hair mt-5 space-y-5 border-t pt-4">
      <h3 className="text-body text-xs">Recorded tool logs for this turn</h3>
      {memoryTools.map((tool) => <ToolDebugDetails key={tool.key} tool={tool} />)}
    </div>}
    </details>}
  </div>;
}

function MemoryTurnResults({tools}: {tools: ToolItem[]}) {
  if (!tools.length) return null;
  const ordered = tools.map((tool) => {
    const result = readMemoryToolResult(tool);
    return {tool, hasRecords: result?.kind === "records" && result.records.length > 0};
  }).sort((a, b) => Number(b.hasRecords) - Number(a.hasRecords));
  return <section aria-label="Memory results for this turn" className="border-hair mb-6 space-y-5 border-b pb-6">
    <h2 className="text-body text-sm font-medium">Memory results for this turn</h2>
    {ordered.map(({tool}) => <div key={tool.key} className="space-y-2">
      <p className="text-muted-text text-xs">{toolLabel(tool).label}</p>
      {toolLabel(tool).subject && <p className="text-muted-text text-xs break-words">{toolLabel(tool).subject}</p>}
      <ToolResultSummary tool={tool} />
    </div>)}
  </section>;
}

function requestLabel(status: string) {
  return status === "completed" ? "Completed response" : status === "interrupted" ? "Request interrupted" : "Prepared for request";
}

function RequestEvidenceView({ receipt, requestStatus }: { receipt: MemoryEvidenceReceipt; requestStatus?: string }) {
  if (requestStatus === "completed" && receipt.evidence.length === 0) {
    const outcome = ({failed: "Retrieval failed for this request.", unavailable: "Retrieval was unavailable for this request.", partial: "Retrieval partially completed without recorded sources.", cancelled: "Retrieval was cancelled for this request.", exhausted: "The retrieval budget was exhausted for this request."} as Record<string, string>)[receipt.status];
    return <p className="text-muted-text mt-3 text-xs leading-5">{outcome ?? "No retrieval sources were recorded for this request."}</p>;
  }
  const heading = requestStatus === "completed" ? "Supplied to model" : requestStatus === "interrupted" ? "Request interrupted" : requestStatus === "prepared" ? "Prepared for request" : "Recorded request evidence";
  const explanation = requestStatus === "completed" ? "Shown to the model with this request; the answer may not have used it."
    : requestStatus === "interrupted" ? "This saved request did not produce a committed response. Provider delivery and source use are unconfirmed."
    : "This request was saved before dispatch. No completed response is recorded.";
  return <div className="mt-4">
    <h3 className="text-body text-sm font-medium">{heading}</h3>
    <p className="text-muted-text mt-2 text-xs leading-5">{explanation}</p>
    {receipt.evidence.length === 0 && <p className="text-muted-text mt-4 text-xs">No retrieval sources were recorded for this request. This does not describe records returned separately by memory tools.</p>}
    {receipt.evidence.map((item) => { const excerpt = item.reference.kind === "conversation_excerpt"; return <section key={item.reference.id} id={memoryEvidenceAnchor(item.reference.id)} className="border-hair mt-5 border-t pt-4">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <h3 className="text-teal">{item.reference.kind === "conversation_excerpt" ? "Conversation excerpt" : "Accepted memory"}</h3>
        {item.reference.intent === "historical" && <span className="text-muted-text">Historical</span>}
        {(item.reference.status === "retired" || item.current_status === "retired" || item.reference.current_status === "retired") && <span className="text-amber-ink">{item.current_status === "retired" ? "Retired now" : item.reference.current_status === "retired" ? "Retired at retrieval" : "Retired in historical view"}</span>}
      </div>
      {!item.available || !item.evidence ? <p className="text-muted-text mt-2 text-xs">Source unavailable under current access.</p> : <>
        {excerpt ? <ExcerptQuote text={item.evidence.text} sources={item.evidence.sources} />
          : <p className="text-body mt-2 text-sm leading-6 whitespace-pre-wrap">{item.evidence.text}</p>}
        {item.reference.kind === "accepted_memory" && item.reference.claim_id && <details className="text-muted-text mt-2 break-all text-[11px] leading-5">
          <summary className="cursor-pointer">Original Claim reference</summary>
          <p>Claim ID: {item.reference.claim_id}<br />Claim version: {item.reference.claim_operation_id || "Unavailable"}</p>
        </details>}
        <GraphEvidencePath item={item} evidence={receipt.evidence} />
        {!excerpt && item.reference.retrieval_generation && <details className="text-muted-text mt-2 break-all text-[11px] leading-5">
          <summary className="cursor-pointer">Original discovery details</summary>
          <p>Index generation: {item.reference.retrieval_generation}</p>
          <p>Discovery paths: {item.reference.paths.join(", ")}</p>
          <p>Discovery does not change source authority or establish that the answer used this evidence.</p>
        </details>}
        {(item.reference.conflicts?.length ?? 0) > 0 && <div className="text-amber-ink mt-3 text-xs leading-5">
          <p>Conflicting accepted Claims</p>
          {item.reference.conflicts?.map((conflict) => <p key={`${conflict.code}:${conflict.claim_ids.join(":")}`}>{conflict.predicate_token}: {conflict.code === "opposite_polarity" ? "Opposite assertions" : conflict.code === "one_cardinality_overlap" ? "Different accepted values" : "Conflicting evidence"} · {conflict.claim_ids.join(", ")}</p>)}
        </div>}
        {item.reference.paths.includes("newer_owner_statement") && (item.reference.related_claim_ids?.length ?? 0) > 0 && <div className="text-amber-ink mt-3 text-xs leading-5">
          <p>Potential discrepancy</p>
          <p>Newer owner wording to compare with accepted Claims: {item.reference.related_claim_ids?.join(", ")}. Recency alone does not establish a correction.</p>
        </div>}
        {(!excerpt || stateChanged(item)) && <p className={`${excerpt ? "text-amber-ink" : "text-muted-text"} mt-2 text-xs`}><StateLine item={item} /></p>}
        {(item.reference.correction_mode || item.reference.current_correction_mode || item.evidence.current_correction_mode) && <div className="text-muted-text mt-2 text-xs leading-5">
          {item.reference.correction_mode && <p>Original correction: {correctionLabel(item.reference.correction_mode)}</p>}
          {item.reference.current_correction_mode && item.reference.current_correction_mode !== item.reference.correction_mode && <p>Correction at retrieval: {correctionLabel(item.reference.current_correction_mode)}</p>}
          {item.evidence.current_correction_mode && item.evidence.current_correction_mode !== item.reference.current_correction_mode && <p>Current correction: {correctionLabel(item.evidence.current_correction_mode)}</p>}
        </div>}
        {item.reference.kind === "accepted_memory" && item.evidence.claim && <div className="text-muted-text mt-2 text-xs leading-5">
          <p>Accepted on: {item.evidence.claim.transaction_time}</p>
          <p>Valid from: {(item.evidence.effective_valid_time ?? item.evidence.claim.valid_time).from ?? "Unknown"} · Valid until: {(item.evidence.effective_valid_time ?? item.evidence.claim.valid_time).to ?? "Unknown"}</p>
        </div>}
        {!excerpt && (item.reference.intent === "historical" || item.reference.valid_at_constrained || item.reference.as_known_at_constrained) && <details className="text-muted-text mt-2 text-xs leading-5">
          <summary className="cursor-pointer">Read filters</summary>
          <p>Known by: {item.reference.as_known_at}</p>
          {item.reference.valid_at_constrained && <p>Valid at: {item.reference.valid_at}</p>}
        </details>}
        {excerpt && <ExcerptDetails item={item} />}
        {!excerpt && item.evidence.sources.map((source) => <div key={`${source.event_id}:${source.locator_value}`} className="border-hair mt-4 border-l pl-3">
          <blockquote className="text-body text-xs leading-5 whitespace-pre-wrap">{source.evidence || (source.authority === "evie_proposed" ? "Evie proposed this value; your words don’t state it." : "Source text unavailable.")}</blockquote>
          <p className="text-muted-text mt-2 text-xs">{source.actor && <>Speaker: {source.actor} · </>}Authority: {source.authority} · {source.observed_at}</p>
          <details className="text-muted-text mt-2 break-all text-[11px]"><summary className="cursor-pointer">Source reference</summary>
            <p>{source.event_id}<br />{source.session_id}<br />{source.source_scope_key}<br />{source.locator_kind}{source.locator_value && ` ${source.locator_value}`}<br />{source.evidence_sha256}</p>
          </details>
        </div>)}
      </>}
    </section>; })}
  </div>;
}

function correctionLabel(mode: string) {
  return mode === "error" ? "Corrected as an error" : mode === "changed" ? "Changed over time" : mode;
}

type EvidenceItem = MemoryEvidenceReceipt["evidence"][number];
type EvidenceSource = NonNullable<EvidenceItem["evidence"]>["sources"][number];

function StateLine({ item }: { item: EvidenceItem }) {
  return <>Original state: {item.reference.status}{item.reference.current_status && item.reference.current_status !== item.reference.status && <> · At retrieval: {item.reference.current_status}</>}{item.current_status && (item.current_status !== item.reference.status || item.current_status !== (item.reference.current_status ?? item.reference.status)) && <> · Current state: {item.current_status}</>}</>;
}

function stateChanged(item: EvidenceItem) {
  const { status, current_status } = item.reference;
  return status !== "active" || (!!current_status && current_status !== status) || (!!item.current_status && item.current_status !== status);
}

// An excerpt reads as a quote with who said it and when; exact identities stay
// one disclosure away.
function ExcerptQuote({ text, sources }: { text: string; sources: EvidenceSource[] }) {
  const extra = sources.filter((source) => source.evidence && source.evidence !== text);
  return <>
    <blockquote className="border-teal-deep text-body mt-2 border-l-2 pl-3 text-sm leading-6 whitespace-pre-wrap">{text}</blockquote>
    {extra.map((source) => <blockquote key={`${source.event_id}:${source.locator_value}`} className="border-hair text-body mt-2 border-l-2 pl-3 text-xs leading-5 whitespace-pre-wrap">{source.evidence}</blockquote>)}
    {sources.map((source) => <p key={`${source.event_id}:${source.locator_value}:who`} className="text-muted-text mt-1.5 text-xs">{[speakerName(source.actor), readableTime(source.observed_at)].filter(Boolean).join(" · ")}</p>)}
  </>;
}

function ExcerptDetails({ item }: { item: EvidenceItem }) {
  return <details className="text-muted-text mt-2 break-all text-[11px] leading-5">
    <summary className="cursor-pointer">Details</summary>
    <p><StateLine item={item} /></p>
    {item.evidence?.sources.map((source) => <p key={`${source.event_id}:${source.locator_value}`} className="mt-2">
      {source.actor && <>Speaker: {source.actor} · </>}Authority: {source.authority} · {source.observed_at}<br />
      {source.event_id}<br />{source.session_id}<br />{source.source_scope_key}<br />{source.locator_kind}{source.locator_value && ` ${source.locator_value}`}<br />{source.evidence_sha256}
    </p>)}
    {item.reference.retrieval_generation && <p className="mt-2">Index generation: {item.reference.retrieval_generation}<br />Discovery paths: {item.reference.paths.join(", ")}</p>}
    {(item.reference.intent === "historical" || item.reference.valid_at_constrained || item.reference.as_known_at_constrained) && <p className="mt-2">
      Known by: {item.reference.as_known_at}{item.reference.valid_at_constrained && <><br />Valid at: {item.reference.valid_at}</>}
    </p>}
  </details>;
}

function speakerName(actor?: string) {
  return actor === "owner" ? "You" : actor === "assistant" ? "Evie" : actor;
}

function readableTime(value?: string) {
  if (!value) return undefined;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString(undefined, { month: "short", day: "numeric", year: "numeric", hour: "numeric", minute: "2-digit" });
}
