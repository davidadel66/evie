from pathlib import Path
p=Path('internal/web/ui/src/memory/Memory.tsx');s=p.read_text()
s=s.replace('import { Database, Layers } from "../ui/Icon";', 'import { Layers } from "../ui/Icon";\nimport type { ContextSessionSnapshot } from "../api/contextSessions";\nimport { MemoryPresentationProvider, MemoryScopeSelect, useMemoryPresentation, entityLabel, scopeLabel } from "./presentation";')
a=s.index('export function Memory(');b=s.index('\nfunction AcceptedMemory',a)
s=s[:a]+'''export function Memory({ snapshot, onOpenDetail }: { snapshot?: ContextSessionSnapshot; onOpenDetail?: (detail: SemanticObjectInspection) => void } = {}) {
  return <MemoryPresentationProvider snapshot={snapshot}><MemoryReviewTabs><AcceptedMemory onOpenDetail={onOpenDetail} /></MemoryReviewTabs></MemoryPresentationProvider>;
}
'''+s[b:]
s=s.replace('  const [scopes, setScopes]', '  const { preferredScope } = useMemoryPresentation();\n  const [selection, setSelection] = useState<SemanticObjectSummary>();\n  const [scopes, setScopes]',1).replace('useState<MemoryViewMode>("graph")','useState<MemoryViewMode>("records")',1)
s=s.replace('.then((result) => { setScopes(result.scopes); setProblem(""); })','.then((result) => { setScopes(result.scopes); setScopeKey(result.scopes.some((scope) => scope.scope_key === preferredScope) ? preferredScope : result.scopes.find((scope) => scope.scope_key === "global")?.scope_key ?? ""); setProblem(""); })',1).replace('  }, []);','  }, [preferredScope]);',1)
s=s.replace('() => inspectMemoryObject(scopeKey, object.object_kind, object.object_id, temporal),','() => inspectMemoryObject(object.scope_key, object.object_kind, object.object_id, temporal),').replace('(result) => { setDetail(result); onOpenDetail?.(result); setProblem(""); },','(result) => { setSelection(object); setDetail(result); onOpenDetail?.(result); setProblem(""); },')
s=s.replace('      detail={detail}\n','      detail={detail}\n      selection={selection}\n      onBack={() => { detailRequests.current.invalidate(); setDetail(undefined); setSelection(undefined); }}\n',1)
s=s.replace('  detail,\n  problem','  detail,\n  selection,\n  onBack,\n  problem',1)
s=s.replace('  detail?: SemanticObjectInspection;\n','  detail?: SemanticObjectInspection;\n  selection?: SemanticObjectSummary;\n  onBack?: () => void;\n',1)
a=s.index('  const selectedScope =',s.index('export function MemoryView'));b=s.index('\nfunction KnowledgeCanvas',a)
s=s[:a]+'''  const { ownerName, names } = useMemoryPresentation();
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
''' +s[b:]
s=s.replace('  const graph = useMemo(() => buildKnowledgeGraph(entityPage?.objects ?? [], claimPage?.objects ?? []), [claimPage?.objects, entityPage?.objects]);','  const { ownerName } = useMemoryPresentation();\n  const graph = useMemo(() => buildKnowledgeGraph(entityPage?.objects ?? [], claimPage?.objects ?? [], ownerName), [claimPage?.objects, entityPage?.objects, ownerName]);')
s=s.replace('Reading accepted knowledge…','Loading memories…').replace('No accepted knowledge here yet','No memories yet')
s=s.replace('<p className="text-fainter mt-1 text-[10.5px]">This graph shows current, supported Claims in one exact scope.</p>','')
s=s.replace('{graph.nodes.filter((node) => node.kind === "entity").length} entities · {graph.edges.length} claims','{graph.edges.length} {graph.edges.length === 1 ? "memory" : "memories"}')
s=s.replace('      <p className="text-ghost mt-1">Claim labels are selectable relationships.</p>\n','').replace('Bounded to the first 100 records per kind','Partial graph · more in List')
s=s.replace('    disabled={!interactive}\n','    disabled={!interactive}\n    title={node.label}\n    aria-label={`Open ${node.label}`}\n')
s=s.replace('block truncate text-[11px] font-medium','block line-clamp-2 text-[12px] font-medium')
s=s.replace('    aria-label={`Claim: ${from.label} ${edge.label} ${to.label}`}','    title={`${from.label} ${edge.polarity === "denied" ? "does not: " : ""}${edge.label} ${to.label}`}\n    aria-label={`Claim: ${from.label} ${edge.label} ${to.label}`}')
a=s.index('function RecordList(');b=s.index('\nfunction objectTitle(',a)
s=s[:a]+'''function RecordList({ kind, page, onInspect, onNext, ownerName = "You", title }: { kind: ListKind; page?: SemanticObjectPage; onInspect: (object: SemanticObjectSummary) => void; onNext: (kind: ListKind, cursor: string) => void; ownerName?: string; title?: string }) {
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
  const title = detail.entity ? entityLabel(detail.entity, ownerName) : detail.claim ? `${detail.claim.polarity === "denied" ? "Not: " : ""}${claimObject(detail.claim)}` : "Memory";
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
      {detail.entity && <div className="mt-8">{related.filter((object) => object.claim?.subject_entity_id === detail.entity?.entity_id).map((object) => <button type="button" key={object.object_id} onClick={() => onInspect(object)} className="border-hair text-body hover:bg-hover flex w-full items-center justify-between gap-4 border-b py-4 text-left text-sm">{objectTitle(object)}<span aria-hidden="true">›</span></button>)}</div>}
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
''' +s[b:]
s=s.replace('  if (object.claim) return `${object.claim.predicate.label}: ${claimObject(object.claim)}`;','  if (object.claim) return `${object.claim.polarity === "denied" ? "Not: " : ""}${claimObject(object.claim)}`;')
# formatCompactTime is no longer shown by default.
a=s.find('\nfunction formatCompactTime(')
if a!=-1:
 b=s.find('\nfunction ',a+1)
 s=s[:a]+(s[b:] if b!=-1 else '')
p.write_text(s)
