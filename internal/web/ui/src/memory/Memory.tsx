import { useCallback, useEffect, useRef, useState } from "react";
import {
  inspectMemoryObject,
  listMemoryObjects,
  listMemoryScopes,
  type MemoryTimeFilter,
  type SemanticObjectInspection,
  type SemanticObjectPage,
  type SemanticObjectSummary,
  type SemanticScope,
} from "../api/memory";
import { LatestMemoryRequest } from "./latestRequest";

type ListKind = "entity" | "claim";

export function Memory() {
  const [scopes, setScopes] = useState<SemanticScope[]>([]);
  const [scopeKey, setScopeKey] = useState("");
  const [kind, setKind] = useState<ListKind>("entity");
  const [history, setHistory] = useState(false);
  const [validAt, setValidAt] = useState("");
  const [asKnownAt, setAsKnownAt] = useState("");
  const [page, setPage] = useState<SemanticObjectPage>();
  const [detail, setDetail] = useState<SemanticObjectInspection>();
  const [problem, setProblem] = useState("");
  const listRequests = useRef(new LatestMemoryRequest());
  const detailRequests = useRef(new LatestMemoryRequest());

  useEffect(() => {
    void listMemoryScopes()
      .then((result) => {
        setScopes(result.scopes);
        setProblem("");
      })
      .catch((error: unknown) => setProblem(errorMessage(error, "Memory scopes are unavailable")));
  }, []);

  const filter = useCallback((): MemoryTimeFilter => ({
    history,
    validAt: history && validAt ? new Date(validAt).toISOString() : undefined,
    asKnownAt: history && asKnownAt ? new Date(asKnownAt).toISOString() : undefined,
  }), [history, validAt, asKnownAt]);

  const load = useCallback(async (cursor?: string) => {
    if (!scopeKey) return;
    detailRequests.current.invalidate();
    setDetail(undefined);
    await listRequests.current.run(
      () => listMemoryObjects({ scopeKey, kinds: [kind], pageSize: 20, cursor, ...filter() }),
      (result) => {
        detailRequests.current.invalidate();
        setPage(result);
        setDetail(undefined);
        setProblem("");
      },
      (error) => setProblem(errorMessage(error, "Memory listing failed")),
    );
  }, [filter, kind, scopeKey]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => () => {
    listRequests.current.invalidate();
    detailRequests.current.invalidate();
  }, []);

  const invalidateListing = () => {
    listRequests.current.invalidate();
    detailRequests.current.invalidate();
    setPage(undefined);
    setDetail(undefined);
  };

  const inspect = async (object: SemanticObjectSummary) => {
    setDetail(undefined);
    await detailRequests.current.run(
      () => inspectMemoryObject(scopeKey, object.object_kind, object.object_id, filter()),
      (result) => {
        setDetail(result);
        setProblem("");
      },
      (error) => setProblem(errorMessage(error, "Memory detail failed")),
    );
  };

  return (
    <MemoryView
      scopes={scopes} scopeKey={scopeKey} kind={kind} history={history}
      validAt={validAt} asKnownAt={asKnownAt} page={page} detail={detail} problem={problem}
      onScope={(value) => { invalidateListing(); setScopeKey(value); }}
      onKind={(value) => { invalidateListing(); setKind(value); }}
      onHistory={(value) => { invalidateListing(); setHistory(value); }}
      onValidAt={(value) => { invalidateListing(); setValidAt(value); }}
      onAsKnownAt={(value) => { invalidateListing(); setAsKnownAt(value); }}
      onRefresh={() => void load()} onNext={(cursor) => void load(cursor)}
      onInspect={(object) => void inspect(object)}
    />
  );
}

export function MemoryView({
  scopes, scopeKey, kind, history, validAt, asKnownAt, page, detail, problem,
  onScope, onKind, onHistory, onValidAt, onAsKnownAt, onRefresh, onNext, onInspect,
}: {
  scopes: SemanticScope[];
  scopeKey: string;
  kind: ListKind;
  history: boolean;
  validAt: string;
  asKnownAt: string;
  page?: SemanticObjectPage;
  detail?: SemanticObjectInspection;
  problem?: string;
  onScope: (value: string) => void;
  onKind: (value: ListKind) => void;
  onHistory: (value: boolean) => void;
  onValidAt: (value: string) => void;
  onAsKnownAt: (value: string) => void;
  onRefresh: () => void;
  onNext: (cursor: string) => void;
  onInspect: (object: SemanticObjectSummary) => void;
}) {
  return (
    <main className="min-h-0 flex-1 overflow-auto p-6">
      <div className="mx-auto flex max-w-[1100px] flex-col gap-5">
        <header>
          <h1 className="text-ink text-base font-semibold">Semantic Memory</h1>
          <p className="text-muted mt-1">Read-only exact inspection. Choose one scope; sibling scopes are never combined.</p>
        </header>
        {problem && <p role="alert" className="text-red">{problem}</p>}
        <section aria-label="Memory query" className="border-hair bg-surface grid gap-4 rounded-lg border p-4 md:grid-cols-2">
          <label className="text-muted flex flex-col gap-1">
            Memory scope
            <select className="border-hair bg-app text-ink rounded border px-3 py-2" value={scopeKey} onChange={(event) => onScope(event.target.value)}>
              <option value="">Select one scope…</option>
              {scopes.map((scope) => <option key={scope.scope_key} value={scope.scope_key}>{scope.scope_key}{scope.quarantined ? " — quarantined" : ""}</option>)}
            </select>
          </label>
          <fieldset className="flex items-end gap-2">
            <legend className="text-muted mb-1">Records</legend>
            {(["entity", "claim"] as const).map((value) => (
              <button type="button" key={value} aria-pressed={kind === value} className="border-hair rounded border px-3 py-2 capitalize" onClick={() => onKind(value)}>{value === "entity" ? "Entities" : "Claims"}</button>
            ))}
          </fieldset>
          <label className="text-muted flex items-center gap-2">
            <input type="checkbox" checked={history} onChange={(event) => onHistory(event.target.checked)} />
            Historical view
          </label>
          {history && <div className="grid gap-2 sm:grid-cols-2">
            <label className="text-muted flex flex-col gap-1">Valid Time<input aria-label="Valid Time" type="datetime-local" className="border-hair bg-app rounded border px-2 py-1" value={validAt} onChange={(event) => onValidAt(event.target.value)} /></label>
            <label className="text-muted flex flex-col gap-1">Transaction Time<input aria-label="Transaction Time" type="datetime-local" className="border-hair bg-app rounded border px-2 py-1" value={asKnownAt} onChange={(event) => onAsKnownAt(event.target.value)} /></label>
          </div>}
          <button type="button" disabled={!scopeKey} className="border-hair rounded border px-3 py-2 md:col-span-2" onClick={onRefresh}>Refresh exact read</button>
        </section>
        {scopes.find((scope) => scope.scope_key === scopeKey)?.quarantined && (
          <p role="status" className="text-amber">This scope is quarantined: {scopes.find((scope) => scope.scope_key === scopeKey)?.quarantine_reason}</p>
        )}
        {page && <section aria-live="polite">
          <h2 className="text-body font-semibold">{kind === "entity" ? "Entities" : "Claims"}</h2>
          <p className="text-fainter mt-1 font-mono text-xs">scope={page.metadata.selected_scope} · Valid Time={formatTime(page.metadata.valid_at)} · Transaction Time={formatTime(page.metadata.as_known_at)}</p>
          <div className="mt-3 grid gap-2">
            {page.objects.length === 0 && <p className="text-muted">No records at the effective times.</p>}
            {page.objects.map((object) => <button type="button" key={object.object_id} className="border-hair bg-surface rounded border p-3 text-left" onClick={() => onInspect(object)}>
              <span className="text-ink block font-medium">{objectTitle(object)}</span>
              <span className="text-fainter mt-1 block font-mono text-xs">{object.object_id} · {object.status} · {object.scope_key}</span>
            </button>)}
          </div>
          {page.next_cursor && <button type="button" className="border-hair mt-3 rounded border px-3 py-2" onClick={() => onNext(page.next_cursor!)}>Next page</button>}
        </section>}
        {detail && <MemoryDetail detail={detail} />}
      </div>
    </main>
  );
}

function MemoryDetail({ detail }: { detail: SemanticObjectInspection }) {
  return <section aria-label="Memory record detail" className="border-hair bg-surface rounded-lg border p-4">
    <h2 className="text-body font-semibold">Record detail</h2>
    <p className="text-fainter mt-1 font-mono text-xs">{detail.object_kind}:{detail.object_id} · {detail.status}</p>
    {detail.entity && <p className="text-muted mt-3">Entity: {detail.entity.canonical_name} ({detail.entity.entity_type})</p>}
    {detail.claim && <div className="text-muted mt-3">
      <p>Claim: {detail.claim.subject_entity_id} {detail.claim.predicate.token} {claimObject(detail.claim)}</p>
      <p>Predicate: {detail.claim.predicate.label} v{detail.claim.predicate.version} · {detail.claim.predicate.cardinality}</p>
      <p>Valid Time: {formatTime(detail.claim.valid_time.from)} → {formatTime(detail.claim.valid_time.to)} · Transaction Time: {formatTime(detail.claim.transaction_time)}</p>
    </div>}
    <h3 className="text-body mt-4 font-semibold">Provenance</h3>
    {detail.sources.length === 0 && <p className="text-fainter">No Source Links.</p>}
    {detail.sources.map(({ source }) => <article key={source.source_link_id ?? source.event_id} className="border-hair mt-2 border-l pl-3">
      <p className="text-muted">{source.authority} · {source.eligibility} · event {source.event_id}</p>
      <p className="text-fainter font-mono text-xs">scope={source.source_scope_key} · observed={source.observed_at} · {source.evidence_sha256}</p>
      <p className="text-body mt-1">{source.evidence || "Source text unavailable in this scope."}</p>
    </article>)}
    <h3 className="text-body mt-4 font-semibold">Lifecycle</h3>
    <ol className="text-muted mt-2 list-decimal pl-5">{detail.lifecycle.map((state) => <li key={`${state.operation_id}:${state.state}`}>{state.state} · revision {state.scope_revision} · {formatTime(state.transaction_time)}</li>)}</ol>
    {detail.conflicts.length > 0 && <><h3 className="text-body mt-4 font-semibold">Conflicts</h3>{detail.conflicts.map((conflict) => <p className="text-amber mt-1" key={`${conflict.code}:${conflict.claim_ids.join(":")}`}>{conflict.code}: {conflict.claim_ids.join(", ")}</p>)}</>}
    <h3 className="text-body mt-4 font-semibold">Operation history</h3>
    <ol className="text-muted mt-2 list-decimal pl-5">{detail.operations.map((operation) => <li key={operation.operation_id}><span className="font-mono">{operation.operation_id}</span> · {operation.kind} schema v{operation.schema_version} · {formatTime(operation.transaction_time)}<br /><span className="text-fainter text-xs">proposal {operation.proposal_sha256} · effect {operation.effect_sha256}</span></li>)}</ol>
  </section>;
}

function objectTitle(object: SemanticObjectSummary) {
  if (object.entity) return object.entity.canonical_name;
  if (object.claim) return `${object.claim.subject_entity_id} ${object.claim.predicate.token} ${claimObject(object.claim)}`;
  return `${object.object_kind} ${object.object_id}`;
}

function claimObject(claim: { object: SemanticClaimObject }) {
  if (claim.object.entity_id) return claim.object.entity_id;
  return claim.object.literal ? `${claim.object.literal.kind}:${claim.object.literal.value}` : "unknown";
}

type SemanticClaimObject = { entity_id?: string; literal?: { kind: string; value: string } };
function formatTime(value?: string | null) { return value ? new Date(value).toISOString() : "open"; }
function errorMessage(error: unknown, fallback: string) { return error instanceof Error ? error.message : fallback; }
