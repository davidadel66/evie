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
import { Layers } from "../ui/Icon";
import type { ContextSessionSnapshot } from "../api/contextSessions";
import { MemoryPresentationProvider, MemoryScopeSelect, useMemoryPresentation, entityLabel, scopeLabel } from "./presentation";
import { MemoryReviewTabs } from "../candidateInbox/MemoryReviewTabs";
import { LatestMemoryRequest } from "./latestRequest";
import { buildKnowledgeGraph, layoutKnowledgeGraph, parallelEdgeOffset, type KnowledgeEdge, type KnowledgeLayoutNode } from "./knowledgeGraph";
import { metadataTimeFilter, pinMemoryRead, sameMemorySnapshot } from "./readSnapshot";

type ListKind = "entity" | "claim";
export type MemoryViewMode = "graph" | "records";

export function Memory({ snapshot, onOpenDetail }: { snapshot?: ContextSessionSnapshot; onOpenDetail?: (detail: SemanticObjectInspection) => void } = {}) {
  return <MemoryPresentationProvider snapshot={snapshot}><MemoryReviewTabs><AcceptedMemory onOpenDetail={onOpenDetail} /></MemoryReviewTabs></MemoryPresentationProvider>;
}

function AcceptedMemory({ onOpenDetail }: { onOpenDetail?: (detail: SemanticObjectInspection) => void } = {}) {
  const { preferredScope } = useMemoryPresentation();
  const [selection, setSelection] = useState<SemanticObjectSummary>();
  const [scopes, setScopes] = useState<SemanticScope[]>([]);
  const [scopeKey, setScopeKey] = useState("");
  const [view, setView] = useState<MemoryViewMode>("records");
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

  const resetRead = useCallback(() => {
    graphRequest.current++;
    detailRequests.current.invalidate();
    setEntityPage(undefined);
    setClaimPage(undefined);
    setEntityRecordPage(undefined);
    setClaimRecordPage(undefined);
    setDetail(undefined);
  }, []);
  const scopeRequests = useRef(new LatestMemoryRequest());
  useEffect(() => {
    resetRead();
    setScopeKey("");
    setScopes([]);
    setProblem("");
    void scopeRequests.current.run(
      () => listMemoryScopes(),
      (result) => {
        setScopes(result.scopes);
        setScopeKey(result.scopes.some((scope) => scope.scope_key === preferredScope) ? preferredScope : result.scopes.find((scope) => scope.scope_key === "global")?.scope_key ?? "");
      },
      (error) => setProblem(errorMessage(error, "Memory scopes are unavailable")),
    );
    const requests = scopeRequests.current;
    return () => requests.invalidate();
  }, [preferredScope, resetRead]);

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
      () => inspectMemoryObject(object.scope_key, object.object_kind, object.object_id, temporal),
      (result) => { setSelection(object); setDetail(result); onOpenDetail?.(result); setProblem(""); },
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
      selection={selection}
      onBack={() => { detailRequests.current.invalidate(); setDetail(undefined); setSelection(undefined); }}
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
  selection,
  onBack,
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
  selection?: SemanticObjectSummary;
  onBack?: () => void;
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
  const { ownerName, names } = useMemoryPresentation();
  const selectedScope = scopes.find((scope) => scope.scope_key === scopeKey);
  const metadata = claimPage?.metadata ?? entityPage?.metadata;
  return (
    <section aria-label="Semantic Memory" className="flex min-h-0 flex-1 flex-col overflow-hidden">
      {detail && detailPlacement === "inline" ? <MemoryDetail detail={detail} selection={selection} onBack={onBack} related={claimPage?.objects ?? []} onInspect={onInspect} /> : <>
        <div className="border-hair flex flex-none flex-wrap items-center gap-3 border-b px-5 py-3 sm:px-7">
          <MemoryScopeSelect scopes={scopes} value={scopeKey} onChange={onScope} />
          <div className="flex-1" />
          <div aria-label="Memory display" className="bg-selected/60 flex rounded-lg p-1">
            {(["records", "graph"] as const).map((item) => <button key={item} type="button" aria-pressed={view === item} onClick={() => onView(item)} className={`${view === item ? "bg-sidebar text-ink shadow-sm" : "text-muted-text hover:text-body"} rounded-md px-3 py-1.5 text-sm`}>{item === "records" ? "List" : "Graph"}</button>)}
          </div>
          <button type="button" disabled={!scopeKey} onClick={onRefresh} className="text-muted-text hover:text-body rounded px-2 py-2 text-sm disabled:opacity-40">Refresh</button>
          <details className="relative">
            <summary className="text-muted-text hover:text-body cursor-pointer rounded px-2 py-2 text-sm">Options</summary>
            <div className="border-hair bg-sidebar absolute right-0 top-full z-30 mt-2 w-72 space-y-4 rounded-lg border p-4 text-sm shadow-lg">
              <label className="flex items-center justify-between gap-3">Show<select aria-label="Memory record type" className="border-hair bg-app rounded border px-2 py-1.5" value={recordKind} onChange={(event) => { onRecordKind(event.target.value as ListKind); onView("records"); }}><option value="claim">Memories</option><option value="entity">People &amp; places</option></select></label>
              <label className="flex items-center gap-2"><input type="checkbox" checked={atTime} onChange={(event) => onAtTime(event.target.checked)} />History</label>
              {atTime && <div className="space-y-3">
                <label className="block">Valid at<input aria-label="Valid at" type="datetime-local" className="border-hair bg-app mt-1 w-full rounded border px-2 py-1" value={validAt} onChange={(event) => onValidAt(event.target.value)} /></label>
                <label className="block">Known by Evie at<input aria-label="Known by Evie at" type="datetime-local" className="border-hair bg-app mt-1 w-full rounded border px-2 py-1" value={asKnownAt} onChange={(event) => onAsKnownAt(event.target.value)} /></label>
              </div>}
              {metadata && <p className="text-muted-text text-xs">Revision {metadata.scope_revisions.find((revision) => revision.scope_key === scopeKey)?.revision ?? "—"}</p>}
            </div>
          </details>
        </div>
        {problem && <p role="alert" className="border-danger-hair bg-danger-bg text-danger-ink border-b px-5 py-3 text-sm">{problem}</p>}
        {selectedScope?.quarantined && <p role="status" className="border-amber-hair bg-amber-bg text-amber-ink border-b px-5 py-3 text-sm">Memory unavailable: {selectedScope.quarantine_reason}</p>}
        {atTime && <div role="status" className="border-hair text-amber-ink flex items-center gap-3 border-b px-7 py-2 text-sm">History view<button type="button" onClick={() => onAtTime(false)} className="underline">Back to current</button></div>}
        <div className="min-h-0 flex-1 overflow-auto">
          {!scopeKey && <MemoryEmpty />}
          {scopeKey && view === "graph" && <KnowledgeCanvas entityPage={entityPage} claimPage={claimPage} onInspect={onInspect} />}
          {scopeKey && view === "records" && <RecordList kind={recordKind} page={recordKind === "entity" ? entityRecordPage ?? entityPage : claimRecordPage ?? claimPage} onInspect={onInspect} onNext={onNext} ownerName={ownerName} title={scopeLabel(scopeKey, names)} />}
        </div>
      </>}
    </section>
  );
}

function KnowledgeCanvas({ entityPage, claimPage, onInspect }: { entityPage?: SemanticObjectPage; claimPage?: SemanticObjectPage; onInspect: (object: SemanticObjectSummary) => void }) {
  const { ownerName } = useMemoryPresentation();
  const graph = useMemo(() => buildKnowledgeGraph(entityPage?.objects ?? [], claimPage?.objects ?? [], ownerName), [claimPage?.objects, entityPage?.objects, ownerName]);
  const layout = useMemo(() => layoutKnowledgeGraph(graph), [graph]);
  const byID = useMemo(() => new Map(layout.nodes.map((node) => [node.id, node])), [layout.nodes]);
  if (!entityPage || !claimPage) return <div className="text-fainter flex min-h-full items-center justify-center p-8 text-xs">Loading memories…</div>;
  if (layout.nodes.length === 0) return <div className="flex min-h-full items-center justify-center p-8 text-center"><div><Layers size={18} className="text-ghost mx-auto" /><h3 className="text-body mt-3 text-xs font-medium">No memories yet</h3></div></div>;
  return <div className="relative min-h-full min-w-full overflow-auto">
    <div className="border-hair bg-docked/90 absolute left-4 top-4 z-20 rounded border px-2.5 py-2 text-[9px] backdrop-blur">
      <p className="text-body">{graph.edges.length} {graph.edges.length === 1 ? "memory" : "memories"}</p>
    </div>
    {(entityPage.next_cursor || claimPage.next_cursor) && <div className="border-hair bg-docked/90 text-ghost absolute right-4 top-4 z-20 rounded border px-2 py-1 text-[9px]">Partial graph · more in List</div>}
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
    title={node.label}
    aria-label={`Open ${node.label}`}
    onClick={() => node.summary && onInspect(node.summary)}
    style={{ left: node.x, top: node.y, width: node.width, height: node.height }}
    className={`${node.kind === "entity" ? node.anchor ? "border-teal bg-selected" : "border-hair-input bg-card" : "border-amber-hair bg-amber-card"} absolute z-10 rounded-[9px] border px-3 text-left shadow-[0_10px_28px_rgba(0,0,0,0.22)] enabled:hover:border-teal disabled:cursor-default`}
  >
    <span className={`${node.kind === "entity" ? "text-body" : "text-amber-ink"} block line-clamp-2 text-[12px] font-medium`}>{node.label}</span>
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
    title={`${from.label} ${edge.polarity === "denied" ? "does not: " : ""}${edge.label} ${to.label}`}
    aria-label={`Claim: ${from.label} ${edge.polarity === "denied" ? "not " : ""}${edge.label} ${to.label}`}
    onClick={() => onInspect(edge.summary)}
    style={{ left, top, width: 116 }}
    className={`${edge.polarity === "denied" ? "border-amber-hair text-amber-ink" : "border-teal-hair text-teal"} bg-docked hover:bg-selected absolute z-20 truncate rounded-full border px-2 py-1 font-mono text-[8.5px] shadow-[0_4px_16px_rgba(0,0,0,0.3)]`}
  >{edge.polarity === "denied" ? "not " : ""}{edge.label}</button>;
}

function RecordList({ kind, page, onInspect, onNext, ownerName = "You", title }: { kind: ListKind; page?: SemanticObjectPage; onInspect: (object: SemanticObjectSummary) => void; onNext: (kind: ListKind, cursor: string) => void; ownerName?: string; title?: string }) {
  if (!page) return <div role="status" className="text-muted-text p-8 text-sm">Loading memories…</div>;
  return <div className="mx-auto max-w-[960px] px-5 py-7 sm:px-7">
    <div className="mb-6 flex items-baseline gap-3"><h2 className="text-ink text-lg font-semibold">{kind === "entity" ? "People & places" : title || "Memories"}</h2><span className="text-muted-text text-sm">{page.objects.length}{page.next_cursor ? "+" : ""}</span></div>
    <div className="border-hair border-t">
      {page.objects.length === 0 && <p className="text-muted-text py-12 text-sm">{kind === "entity" ? "No people or places yet." : "No memories here yet."}</p>}
      {page.objects.map((object) => <button type="button" key={object.object_id} className="border-hair hover:bg-hover focus-visible:ring-teal flex w-full items-center gap-5 border-b py-5 pr-3 text-left focus-visible:ring-2 focus-visible:outline-none" onClick={() => onInspect(object)}>
        <span className="min-w-0 flex-1"><span className="text-body block text-[15px] leading-6 font-medium">{object.entity ? entityLabel(object.entity, ownerName) : objectTitle(object)}</span>
        {object.claim && <span className="text-muted-text mt-1 block text-sm">{object.subject ? entityLabel(object.subject, ownerName) : "Person"} · {object.claim.predicate.label}</span>}</span>
        {object.status !== "active" && <span className="text-amber-ink text-xs">{object.status}</span>}
        <span aria-hidden="true" className="text-muted-text text-lg">›</span>
      </button>)}
    </div>
    {page.next_cursor && <button type="button" className="border-hair text-body hover:bg-hover mt-5 rounded border px-4 py-2 text-sm" onClick={() => onNext(kind, page.next_cursor!)}>Next page</button>}
  </div>;
}

function MemoryDetail({ detail, selection, onBack, related, onInspect }: { detail: SemanticObjectInspection; selection?: SemanticObjectSummary; onBack?: () => void; related: SemanticObjectSummary[]; onInspect: (object: SemanticObjectSummary) => void }) {
  const { ownerName, names } = useMemoryPresentation();
  const title = detail.entity ? entityLabel(detail.entity, ownerName) : detail.claim ? `${detail.claim.polarity === "denied" ? "Not: " : ""}${selection?.object_entity ? entityLabel(selection.object_entity, ownerName) : claimObject(detail.claim)}` : "Memory";
  const subject = selection?.subject;
  return <section aria-label="Memory detail" className="min-h-0 flex-1 overflow-auto">
    <div className="mx-auto max-w-[860px] px-5 py-6 sm:px-8">
      <button type="button" onClick={onBack} className="text-muted-text hover:text-body mb-8 rounded py-1 text-sm">‹ Back to memories</button>
      <h1 className="text-ink max-w-[65ch] text-2xl leading-snug font-semibold">{title}</h1>
      <div className="text-muted-text mt-4 flex flex-wrap items-center gap-3 text-sm">
        {subject && <button type="button" className="text-teal hover:underline" onClick={() => onInspect({ object_kind: "entity", object_id: subject.entity_id, scope_key: subject.scope_key, status: "active", entity: subject })}>{entityLabel(subject, ownerName)}</button>}
        <span>{scopeLabel(detail.scope.scope_key, names)}</span>
        {detail.status !== "active" && <span className="text-amber-ink">{detail.status}</span>}
      </div>
      {detail.claim && <p className="text-muted-text mt-3 text-sm">{detail.claim.predicate.label}</p>}
      {detail.sources.length > 0 && <section className="mt-9"><h2 className="text-body text-sm font-medium">Source</h2>{detail.sources.map(({ source }) => <blockquote key={source.source_link_id ?? source.event_id} className="border-teal-hair mt-4 border-l-2 pl-5">
        <p className="text-body whitespace-pre-wrap text-[15px] leading-7">{source.evidence || "Source unavailable."}</p>
        <footer className="text-muted-text mt-3 text-xs">{source.authority === "owner_statement" ? ownerName : source.authority} · {formatTime(source.observed_at)}</footer>
      </blockquote>)}</section>}
      {detail.entity && <div className="mt-8">{related.filter((object) => object.claim?.subject_entity_id === detail.entity?.entity_id).map((object) => <button type="button" key={object.object_id} onClick={() => onInspect(object)} className="border-hair text-body hover:bg-hover flex w-full items-center justify-between gap-4 border-b py-4 text-left text-sm"><span><span className="block">{objectTitle(object)}</span><span className="text-muted-text mt-1 block text-xs">{object.claim?.predicate.label} · {scopeLabel(object.scope_key, names)}</span></span><span aria-hidden="true">›</span></button>)}</div>}
      {detail.conflicts.length > 0 && <section className="text-amber-ink mt-8"><h2 className="font-medium">Conflicting memories</h2>{detail.conflicts.map((conflict) => <p key={`${conflict.code}:${conflict.claim_ids.join(":")}`} className="mt-2 text-sm">{conflict.code}: {conflict.claim_ids.join(", ")}</p>)}</section>}
      <details className="border-hair text-muted-text mt-10 border-t pt-5 text-xs"><summary className="cursor-pointer py-1 text-sm">Details &amp; history</summary>
        <dl className="mt-4 space-y-3 break-all"><div><dt>Record</dt><dd>{detail.object_kind}:{detail.object_id}</dd></div><div><dt>Scope</dt><dd>{detail.scope.scope_key}</dd></div><div><dt>Status</dt><dd>{detail.status}</dd></div>
        {detail.entity && <div><dt>Canonical identity</dt><dd>{detail.entity.canonical_name} · {detail.entity.entity_type}</dd></div>}
        {detail.claim && <><div><dt>Valid time</dt><dd>{formatTime(detail.claim.valid_time.from)} – {formatTime(detail.claim.valid_time.to)}</dd></div><div><dt>Saved</dt><dd>{formatTime(detail.claim.transaction_time)}</dd></div></>}
        </dl>
        <h3 className="mt-6 font-medium">Source records</h3>{detail.sources.map(({ source }) => <p className="mt-3 break-all" key={source.source_link_id ?? source.event_id}>{source.event_id}<br />{source.source_scope_key}<br />{source.authority} · {source.eligibility}<br />{source.evidence_sha256}</p>)}
        <h3 className="mt-6 font-medium">History</h3>{detail.lifecycle.map((state) => <p className="mt-2" key={`${state.operation_id}:${state.state}`}>{state.state} · revision {state.scope_revision} · {formatTime(state.transaction_time)}</p>)}
        <h3 className="mt-6 font-medium">Operations</h3>{detail.operations.map((operation) => <p className="mt-2 break-all" key={operation.operation_id}>{operation.kind} · {formatTime(operation.transaction_time)}<br />{operation.operation_id}</p>)}
      </details>
    </div>
  </section>;
}

function MemoryEmpty() {
  return <div className="text-muted-text p-8 text-sm">Open a conversation to view its memories.</div>;
}

function objectTitle(object: SemanticObjectSummary) {
  if (object.entity) return object.entity.canonical_name;
  if (object.claim) return `${object.claim.polarity === "denied" ? "Not: " : ""}${object.object_entity?.canonical_name ?? claimObject(object.claim)}`;
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

function formatTime(value?: string | null) { return value ? new Date(value).toLocaleString() : "Unknown"; }
function shortID(value: string) { return value.length > 12 ? `${value.slice(0, 8)}…` : value; }
function errorMessage(error: unknown, fallback: string) { return error instanceof Error ? error.message : fallback; }
