import { useCallback, useEffect, useMemo, useRef, useState } from "react";
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
import { Database, Layers } from "../ui/Icon";
import { MemoryReviewTabs } from "../candidateInbox/MemoryReviewTabs";
import { LatestMemoryRequest } from "./latestRequest";
import { buildKnowledgeGraph, layoutKnowledgeGraph, parallelEdgeOffset, type KnowledgeEdge, type KnowledgeLayoutNode } from "./knowledgeGraph";
import { metadataTimeFilter, pinMemoryRead, sameMemorySnapshot } from "./readSnapshot";

type ListKind = "entity" | "claim";
export type MemoryViewMode = "graph" | "records";

export function Memory(props: { onOpenDetail?: (detail: SemanticObjectInspection) => void } = {}) {
  return <MemoryReviewTabs><AcceptedMemory {...props} /></MemoryReviewTabs>;
}

function AcceptedMemory({ onOpenDetail }: { onOpenDetail?: (detail: SemanticObjectInspection) => void } = {}) {
  const [scopes, setScopes] = useState<SemanticScope[]>([]);
  const [scopeKey, setScopeKey] = useState("");
  const [view, setView] = useState<MemoryViewMode>("graph");
  const [recordKind, setRecordKind] = useState<ListKind>("claim");
  const [atTime, setAtTime] = useState(false);
  const [validAt, setValidAt] = useState("");
  const [asKnownAt, setAsKnownAt] = useState("");
  const [entityPage, setEntityPage] = useState<SemanticObjectPage>();
  const [claimPage, setClaimPage] = useState<SemanticObjectPage>();
  const [entityRecordPage, setEntityRecordPage] = useState<SemanticObjectPage>();
  const [claimRecordPage, setClaimRecordPage] = useState<SemanticObjectPage>();
  const [detail, setDetail] = useState<SemanticObjectInspection>();
  const [problem, setProblem] = useState("");
  const graphRequest = useRef(0);
  const detailRequests = useRef(new LatestMemoryRequest());

  useEffect(() => {
    void listMemoryScopes()
      .then((result) => { setScopes(result.scopes); setProblem(""); })
      .catch((error: unknown) => setProblem(errorMessage(error, "Memory scopes are unavailable")));
  }, []);

  const filter = useCallback((): MemoryTimeFilter => ({
    history: atTime,
    validAt: atTime ? toISOString(validAt) : undefined,
    asKnownAt: atTime ? toISOString(asKnownAt) : undefined,
  }), [atTime, validAt, asKnownAt]);

  const load = useCallback(async () => {
    if (!scopeKey) return;
    const request = ++graphRequest.current;
    detailRequests.current.invalidate();
    setDetail(undefined);
    try {
      const temporal = pinMemoryRead(filter());
      const [entities, claims] = await Promise.all([
        listMemoryObjects({ scopeKey, kinds: ["entity"], pageSize: 100, ...temporal }),
        listMemoryObjects({ scopeKey, kinds: ["claim"], pageSize: 100, ...temporal }),
      ]);
      if (request !== graphRequest.current) return;
      if (!sameMemorySnapshot(entities.metadata, claims.metadata)) {
        throw new Error("Semantic Memory changed while the graph was loading. Refresh to read one exact revision.");
      }
      setEntityPage(entities);
      setClaimPage(claims);
      setEntityRecordPage(entities);
      setClaimRecordPage(claims);
      setProblem("");
    } catch (error) {
      if (request === graphRequest.current) setProblem(errorMessage(error, "Memory graph could not be read"));
    }
  }, [filter, scopeKey]);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => () => { graphRequest.current++; detailRequests.current.invalidate(); }, []);

  const inspect = async (object: SemanticObjectSummary) => {
    setDetail(undefined);
    const metadata = claimPage?.metadata ?? entityPage?.metadata;
    const temporal = metadata ? metadataTimeFilter(metadata, atTime) : pinMemoryRead(filter());
    await detailRequests.current.run(
      () => inspectMemoryObject(scopeKey, object.object_kind, object.object_id, temporal),
      (result) => { setDetail(result); onOpenDetail?.(result); setProblem(""); },
      (error) => setProblem(errorMessage(error, "Memory detail failed")),
    );
  };

  const nextPage = async (kind: ListKind, cursor: string) => {
    if (!scopeKey) return;
    const request = ++graphRequest.current;
    try {
      const metadata = kind === "entity" ? entityRecordPage?.metadata : claimRecordPage?.metadata;
      const temporal = metadata ? metadataTimeFilter(metadata, atTime) : pinMemoryRead(filter());
      const page = await listMemoryObjects({ scopeKey, kinds: [kind], pageSize: 100, cursor, ...temporal });
      if (request !== graphRequest.current) return;
      if (kind === "entity") setEntityRecordPage(page);
      else setClaimRecordPage(page);
      setProblem("");
    } catch (error) {
      if (request === graphRequest.current) setProblem(errorMessage(error, "Memory page could not be read"));
    }
  };

  const resetRead = () => {
    graphRequest.current++;
    detailRequests.current.invalidate();
    setEntityPage(undefined);
    setClaimPage(undefined);
    setEntityRecordPage(undefined);
    setClaimRecordPage(undefined);
    setDetail(undefined);
  };

  return (
    <MemoryView
      scopes={scopes}
      scopeKey={scopeKey}
      view={view}
      recordKind={recordKind}
      atTime={atTime}
      validAt={validAt}
      asKnownAt={asKnownAt}
      entityPage={entityPage}
      claimPage={claimPage}
      entityRecordPage={entityRecordPage}
      claimRecordPage={claimRecordPage}
      detail={detail}
      problem={problem}
      onScope={(value) => { resetRead(); setScopeKey(value); }}
      onView={setView}
      onRecordKind={setRecordKind}
      onAtTime={(value) => { resetRead(); setAtTime(value); }}
      onValidAt={(value) => { resetRead(); setValidAt(value); }}
      onAsKnownAt={(value) => { resetRead(); setAsKnownAt(value); }}
      onRefresh={() => void load()}
      onNext={(kind, cursor) => void nextPage(kind, cursor)}
      onInspect={(object) => void inspect(object)}
      detailPlacement={onOpenDetail ? "inspector" : "inline"}
    />
  );
}

export function MemoryView({
  scopes,
  scopeKey,
  view,
  recordKind,
  atTime,
  validAt,
  asKnownAt,
  entityPage,
  claimPage,
  entityRecordPage,
  claimRecordPage,
  detail,
  problem,
  onScope,
  onView,
  onRecordKind,
  onAtTime,
  onValidAt,
  onAsKnownAt,
  onRefresh,
  onNext,
  onInspect,
  detailPlacement = "inline",
}: {
  scopes: SemanticScope[];
  scopeKey: string;
  view: MemoryViewMode;
  recordKind: ListKind;
  atTime: boolean;
  validAt: string;
  asKnownAt: string;
  entityPage?: SemanticObjectPage;
  claimPage?: SemanticObjectPage;
  entityRecordPage?: SemanticObjectPage;
  claimRecordPage?: SemanticObjectPage;
  detail?: SemanticObjectInspection;
  problem?: string;
  onScope: (value: string) => void;
  onView: (view: MemoryViewMode) => void;
  onRecordKind: (kind: ListKind) => void;
  onAtTime: (value: boolean) => void;
  onValidAt: (value: string) => void;
  onAsKnownAt: (value: string) => void;
  onRefresh: () => void;
  onNext: (kind: ListKind, cursor: string) => void;
  onInspect: (object: SemanticObjectSummary) => void;
  detailPlacement?: "inline" | "inspector";
}) {
  const selectedScope = scopes.find((scope) => scope.scope_key === scopeKey);
  const metadata = claimPage?.metadata ?? entityPage?.metadata;
  const selectedRevision = metadata?.scope_revisions.find((revision) => revision.scope_key === scopeKey)?.revision ?? selectedScope?.revision;
  return (
    <section aria-label="Semantic Memory" className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="border-hair bg-docked flex flex-none flex-wrap items-center gap-x-4 gap-y-2 border-b px-4 py-2.5 sm:px-5">
        <div className="mr-1 flex items-center gap-2">
          <span className="text-teal"><Layers size={14} /></span>
          <div><h2 className="text-body text-[11.5px] font-medium">Knowledge Graph</h2><p className="text-ghost text-[9.5px]">Semantic Memory</p></div>
        </div>
        <label className="text-fainter flex items-center gap-2 text-[10px]">
          Memory Scope
          <select className="border-hair-input bg-app text-body min-w-[190px] rounded-[5px] border px-2.5 py-1.5 font-mono text-[10px]" value={scopeKey} onChange={(event) => onScope(event.target.value)}>
            <option value="">Choose one exact scope…</option>
            {scopes.map((scope) => <option key={scope.scope_key} value={scope.scope_key}>{scope.scope_key}{scope.quarantined ? " — quarantined" : ""}</option>)}
          </select>
        </label>
        {selectedScope && <div className="text-ghost flex items-center gap-2 text-[9.5px]"><span className="border-hair rounded border px-1.5 py-0.5">Exact scope</span><span className="font-mono">revision {selectedRevision}</span></div>}
        <div className="flex-1" />
        <label className="text-faint flex cursor-pointer items-center gap-2 text-[10px]"><input type="checkbox" checked={atTime} onChange={(event) => onAtTime(event.target.checked)} />View at time</label>
        <button type="button" disabled={!scopeKey} onClick={onRefresh} className="text-faint hover:text-body disabled:text-ghost text-[10px]">Refresh</button>
      </div>

      {atTime && <div className="border-hair bg-card flex flex-none flex-wrap items-center gap-4 border-b px-4 py-2 sm:px-5">
        <label className="text-fainter flex items-center gap-2 text-[9.5px]">Valid at<input aria-label="Valid at" type="datetime-local" className="border-hair bg-app text-body rounded border px-2 py-1 font-mono text-[9.5px]" value={validAt} onChange={(event) => onValidAt(event.target.value)} /></label>
        <label className="text-fainter flex items-center gap-2 text-[9.5px]">Known by Evie at<input aria-label="Known by Evie at" type="datetime-local" className="border-hair bg-app text-body rounded border px-2 py-1 font-mono text-[9.5px]" value={asKnownAt} onChange={(event) => onAsKnownAt(event.target.value)} /></label>
        <p className="text-ghost text-[9px]">World time and acceptance time are independent.</p>
      </div>}

      <div className="border-hair flex h-[38px] flex-none items-stretch border-b px-4 sm:px-5">
        {(["graph", "records"] as const).map((item) => <button key={item} type="button" aria-pressed={view === item} onClick={() => onView(item)} className={`${view === item ? "border-teal text-body" : "border-transparent text-faint hover:text-body"} border-b px-3 text-[10.5px]`}>{item === "graph" ? "Graph" : "Records"}</button>)}
        {view === "records" && <div className="border-hair ml-3 flex items-center gap-1 border-l pl-3">
          {(["entity", "claim"] as const).map((kind) => <button key={kind} type="button" aria-pressed={recordKind === kind} onClick={() => onRecordKind(kind)} className={`${recordKind === kind ? "bg-selected text-body" : "text-faint hover:text-body"} rounded px-2 py-1 text-[9.5px]`}>{kind === "entity" ? "Entities" : "Claims"}</button>)}
        </div>}
        <div className="flex-1" />
        {metadata && <div className="text-ghost flex items-center font-mono text-[8.5px]">Valid {formatCompactTime(metadata.valid_at)} · Known {formatCompactTime(metadata.as_known_at)}</div>}
      </div>

      {problem && <p role="alert" className="border-danger-hair bg-danger-bg text-danger-ink border-b px-4 py-2 text-[11px]">{problem}</p>}
      {selectedScope?.quarantined && <p role="status" className="border-amber-hair bg-amber-bg text-amber-ink border-b px-4 py-2 text-[11px]">This scope is quarantined: {selectedScope.quarantine_reason}</p>}

      <div className="min-h-0 flex-1 overflow-auto">
        {!scopeKey && <MemoryEmpty />}
        {scopeKey && view === "graph" && <KnowledgeCanvas entityPage={entityPage} claimPage={claimPage} onInspect={onInspect} />}
        {scopeKey && view === "records" && <RecordList kind={recordKind} page={recordKind === "entity" ? entityRecordPage ?? entityPage : claimRecordPage ?? claimPage} onInspect={onInspect} onNext={onNext} />}
        {detail && detailPlacement === "inline" && <div className="p-5"><MemoryDetail detail={detail} /></div>}
      </div>
    </section>
  );
}

function KnowledgeCanvas({ entityPage, claimPage, onInspect }: { entityPage?: SemanticObjectPage; claimPage?: SemanticObjectPage; onInspect: (object: SemanticObjectSummary) => void }) {
  const graph = useMemo(() => buildKnowledgeGraph(entityPage?.objects ?? [], claimPage?.objects ?? []), [claimPage?.objects, entityPage?.objects]);
  const layout = useMemo(() => layoutKnowledgeGraph(graph), [graph]);
  const byID = useMemo(() => new Map(layout.nodes.map((node) => [node.id, node])), [layout.nodes]);
  if (!entityPage || !claimPage) return <div className="text-fainter flex min-h-full items-center justify-center p-8 text-xs">Reading accepted knowledge…</div>;
  if (layout.nodes.length === 0) return <div className="flex min-h-full items-center justify-center p-8 text-center"><div><Layers size={18} className="text-ghost mx-auto" /><h3 className="text-body mt-3 text-xs font-medium">No accepted knowledge here yet</h3><p className="text-fainter mt-1 text-[10.5px]">This graph shows current, supported Claims in one exact scope.</p></div></div>;
  return <div className="relative min-h-full min-w-full overflow-auto">
    <div className="border-hair bg-docked/90 absolute left-4 top-4 z-20 rounded border px-2.5 py-2 text-[9px] backdrop-blur">
      <p className="text-body">{graph.nodes.filter((node) => node.kind === "entity").length} entities · {graph.edges.length} claims</p>
      <p className="text-ghost mt-1">Claim labels are selectable relationships.</p>
    </div>
    {(entityPage.next_cursor || claimPage.next_cursor) && <div className="border-hair bg-docked/90 text-ghost absolute right-4 top-4 z-20 rounded border px-2 py-1 text-[9px]">Bounded to the first 100 records per kind</div>}
    <div style={{ width: layout.width, height: layout.height }} className="relative mx-auto my-5">
      <svg aria-label="Semantic knowledge relationships" width={layout.width} height={layout.height} className="pointer-events-none absolute inset-0">
        <defs><marker id="memory-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="5" markerHeight="5" orient="auto"><path d="M0 0 8 4 0 8z" fill="currentColor" /></marker></defs>
        {layout.edges.map((edge) => {
          const from = byID.get(edge.from);
          const to = byID.get(edge.to);
          if (!from || !to) return null;
          return <path key={edge.id} d={knowledgeEdgePath(from, to, parallelEdgeOffset(edge))} fill="none" stroke="currentColor" strokeWidth="1" markerEnd="url(#memory-arrow)" className={edge.polarity === "denied" ? "text-amber" : "text-teal"} opacity="0.45" />;
        })}
      </svg>
      {layout.edges.map((edge) => {
        const from = byID.get(edge.from);
        const to = byID.get(edge.to);
        if (!from || !to) return null;
        return <ClaimButton key={edge.id} edge={edge} from={from} to={to} onInspect={onInspect} />;
      })}
      {layout.nodes.map((node) => <KnowledgeNodeButton key={node.id} node={node} onInspect={onInspect} />)}
    </div>
  </div>;
}

function KnowledgeNodeButton({ node, onInspect }: { node: KnowledgeLayoutNode; onInspect: (object: SemanticObjectSummary) => void }) {
  const interactive = Boolean(node.summary);
  return <button
    type="button"
    disabled={!interactive}
    onClick={() => node.summary && onInspect(node.summary)}
    style={{ left: node.x, top: node.y, width: node.width, height: node.height }}
    className={`${node.kind === "entity" ? node.anchor ? "border-teal bg-selected" : "border-hair-input bg-card" : "border-amber-hair bg-amber-card"} absolute z-10 rounded-[9px] border px-3 text-left shadow-[0_10px_28px_rgba(0,0,0,0.22)] enabled:hover:border-teal disabled:cursor-default`}
  >
    <span className={`${node.kind === "entity" ? "text-body" : "text-amber-ink"} block truncate text-[11px] font-medium`}>{node.label}</span>
    <span className="text-ghost mt-1 block truncate text-[9px]">{node.detail}</span>
  </button>;
}

function ClaimButton({ edge, from, to, onInspect }: { edge: KnowledgeEdge; from: KnowledgeLayoutNode; to: KnowledgeLayoutNode; onInspect: (object: SemanticObjectSummary) => void }) {
  const offset = parallelEdgeOffset(edge);
  const self = from.id === to.id;
  const left = self ? from.x + from.width + 18 : (from.x + from.width / 2 + to.x + to.width / 2) / 2 - 58;
  const top = self ? from.y + from.height / 2 - 12 + offset : (from.y + from.height / 2 + to.y + to.height / 2) / 2 - 12 + offset;
  return <button
    type="button"
    aria-label={`Claim: ${from.label} ${edge.label} ${to.label}`}
    onClick={() => onInspect(edge.summary)}
    style={{ left, top, width: 116 }}
    className={`${edge.polarity === "denied" ? "border-amber-hair text-amber-ink" : "border-teal-hair text-teal"} bg-docked hover:bg-selected absolute z-20 truncate rounded-full border px-2 py-1 font-mono text-[8.5px] shadow-[0_4px_16px_rgba(0,0,0,0.3)]`}
  >{edge.polarity === "denied" ? "not " : ""}{edge.label}</button>;
}

function RecordList({ kind, page, onInspect, onNext }: { kind: ListKind; page?: SemanticObjectPage; onInspect: (object: SemanticObjectSummary) => void; onNext: (kind: ListKind, cursor: string) => void }) {
  if (!page) return <div className="text-fainter flex min-h-full items-center justify-center p-8 text-xs">Reading records…</div>;
  return <div className="mx-auto max-w-[880px] p-5 sm:p-7">
    <div className="mb-3 flex items-end"><div><h3 className="text-body text-sm font-medium">{kind === "entity" ? "Entities" : "Claims"}</h3><p className="text-ghost mt-1 text-[10px]">Exact scope · revision {page.metadata.scope_revisions.find((revision) => revision.scope_key === page.metadata.selected_scope)?.revision ?? "—"}</p></div><div className="flex-1" /><span className="text-ghost font-mono text-[9px]">{page.objects.length} shown</span></div>
    <div className="border-hair border-t">
      {page.objects.length === 0 && <p className="text-fainter py-8 text-center text-xs">No records at the selected times.</p>}
      {page.objects.map((object) => <button type="button" key={object.object_id} className="border-hair hover:bg-hover flex w-full items-center gap-4 border-b px-3 py-3 text-left" onClick={() => onInspect(object)}>
        <span className={`${object.object_kind === "claim" ? "border-teal-hair text-teal" : "border-hair-input text-faint"} rounded border px-1.5 py-0.5 font-mono text-[8.5px]`}>{object.object_kind}</span>
        <span className="min-w-0 flex-1"><span className="text-body block truncate text-[11.5px] font-medium">{objectTitle(object)}</span><span className="text-ghost mt-1 block truncate font-mono text-[9px]">{object.object_id}</span></span>
        <span className="text-fainter text-[9.5px]">{object.status}</span>
      </button>)}
    </div>
    {page.next_cursor && <button type="button" className="border-hair text-faint hover:text-body mt-3 rounded border px-3 py-2 text-[10px]" onClick={() => onNext(kind, page.next_cursor!)}>Next page</button>}
  </div>;
}

function MemoryDetail({ detail }: { detail: SemanticObjectInspection }) {
  return <section aria-label="Memory record detail" className="border-hair bg-surface rounded-lg border p-4">
    <h2 className="text-body font-semibold">Record detail</h2>
    <p className="text-fainter mt-1 font-mono text-xs">{detail.object_kind}:{detail.object_id} · {detail.status}</p>
    {detail.entity && <p className="text-muted mt-3">Entity: {detail.entity.canonical_name} ({detail.entity.entity_type})</p>}
    {detail.claim && <div className="text-muted mt-3"><p>Claim: {detail.claim.subject_entity_id} {detail.claim.predicate.token} {claimObject(detail.claim)}</p><p>Valid Time: {formatTime(detail.claim.valid_time.from)} → {formatTime(detail.claim.valid_time.to)} · Transaction Time: {formatTime(detail.claim.transaction_time)}</p></div>}
    <h3 className="text-body mt-4 font-semibold">Evidence</h3>
    {detail.sources.length === 0 && <p className="text-fainter">No Source Links.</p>}
    {detail.sources.map(({ source }) => <article key={source.source_link_id ?? source.event_id} className="border-hair mt-2 border-l pl-3"><p className="text-teal text-[10px]">Source episode · <span className="font-mono">{source.event_id}</span></p><p className="text-muted mt-1">{source.authority} · {source.eligibility}</p><p className="text-fainter font-mono text-xs">scope={source.source_scope_key} · observed={source.observed_at}</p><p className="text-body mt-1">{source.evidence || "Source text unavailable in this scope."}</p></article>)}
    <h3 className="text-body mt-4 font-semibold">Lifecycle</h3>
    <ol className="text-muted mt-2 list-decimal pl-5">{detail.lifecycle.map((state) => <li key={`${state.operation_id}:${state.state}`}>{state.state} · revision {state.scope_revision} · {formatTime(state.transaction_time)}</li>)}</ol>
    {detail.conflicts.length > 0 && <><h3 className="text-body mt-4 font-semibold">Conflicts</h3>{detail.conflicts.map((conflict) => <p className="text-amber mt-1" key={`${conflict.code}:${conflict.claim_ids.join(":")}`}>{conflict.code}: {conflict.claim_ids.join(", ")}</p>)}</>}
    <h3 className="text-body mt-4 font-semibold">Operation history</h3>
    <ol className="text-muted mt-2 list-decimal pl-5">{detail.operations.map((operation) => <li key={operation.operation_id}><span className="font-mono">{operation.operation_id}</span> · {operation.kind} schema v{operation.schema_version} · {formatTime(operation.transaction_time)}</li>)}</ol>
  </section>;
}

function MemoryEmpty() {
  return <div className="flex min-h-full items-center justify-center p-8 text-center"><div><span className="border-hair-strong text-faint mx-auto flex h-10 w-10 items-center justify-center rounded-[10px] border"><Database size={17} /></span><h3 className="text-body mt-4 text-sm font-medium">Choose a Memory Scope</h3><p className="text-fainter mt-2 max-w-[340px] text-xs leading-5">The graph reads one exact scope at a time. Sibling Workspaces and projects are never combined.</p></div></div>;
}

function objectTitle(object: SemanticObjectSummary) {
  if (object.entity) return object.entity.canonical_name;
  if (object.claim) return `${object.subject?.canonical_name ?? shortID(object.claim.subject_entity_id)} ${object.claim.predicate.label} ${object.object_entity?.canonical_name ?? claimObject(object.claim)}`;
  return `${object.object_kind} ${object.object_id}`;
}

function claimObject(claim: { object: { entity_id?: string; literal?: { kind: string; value: string } } }) {
  if (claim.object.entity_id) return shortID(claim.object.entity_id);
  return claim.object.literal?.value ?? "unknown";
}

function knowledgeEdgePath(from: KnowledgeLayoutNode, to: KnowledgeLayoutNode, offset: number) {
  const fromX = from.x + from.width / 2;
  const fromY = from.y + from.height / 2;
  const toX = to.x + to.width / 2;
  const toY = to.y + to.height / 2;
  if (from.id === to.id) {
    const right = from.x + from.width;
    return `M ${right} ${fromY - 8} C ${right + 70} ${fromY - 54 + offset}, ${right + 70} ${fromY + 54 + offset}, ${right} ${fromY + 8}`;
  }
  const bend = Math.min(90, Math.abs(toX - fromX) * 0.2 + 24);
  return `M ${fromX} ${fromY} C ${fromX + bend} ${fromY + offset}, ${toX - bend} ${toY + offset}, ${toX} ${toY}`;
}

function toISOString(value: string) {
  if (!value) return undefined;
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? undefined : date.toISOString();
}

function formatTime(value?: string | null) { return value ? new Date(value).toISOString() : "open"; }
function formatCompactTime(value?: string | null) { return value ? new Date(value).toLocaleString([], { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }) : "open"; }
function shortID(value: string) { return value.length > 12 ? `${value.slice(0, 8)}…` : value; }
function errorMessage(error: unknown, fallback: string) { return error instanceof Error ? error.message : fallback; }
