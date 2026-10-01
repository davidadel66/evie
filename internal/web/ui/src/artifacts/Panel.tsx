import { InstructionsView } from "../folder/Instructions";
import type { InstructionTarget } from "../api/repositoryInstructions";
import { useEffect, useRef } from "react";
import type { ContextScope, Workspace } from "../api/contextSessions";
import type { SemanticObjectInspection } from "../api/memory";
import { Cross, Database, FileIcon, Folder, Layers, Wrench } from "../ui/Icon";
import type { ToolItem } from "../chat/activityModel";
import { ToolInspection } from "./ToolInspection";
import { Diff } from "../chat/Diff";
import { FilePath, FileViewer } from "./FileViewer";
import type { FileInspection as InspectedFile } from "./fileInspection";
import { MemoryEvidence } from "./MemoryEvidence";
import { targetTitle, memoryTitle, claimObject } from "./inspectorTitle";

export type InspectorTarget =
 | ({kind:"repository-instructions"} & InstructionTarget)
  | { kind: "file"; file: InspectedFile }
  | { kind: "tool"; tool: ToolItem }
  | { kind: "memory"; detail: SemanticObjectInspection }
  | { kind: "memory-evidence"; sessionId: string; snapshotId: string; memoryTools?: ToolItem[] }
  | { kind: "workspace"; workspace: Workspace }
  | { kind: "scope"; scope: ContextScope }
  | { kind: "file-diff"; path: string; oldText: string; newText: string; isNew: boolean; state: string }
  | { kind: "data" }
  | { kind: "empty" };

type Props = {
  target: InspectorTarget;
  focused: boolean;
  onClose: () => void;
};

export function Panel({ target, focused, onClose }: Props) {
  const closeButton = useRef<HTMLButtonElement>(null);
  const selectionKey = target.kind === "file" ? target.file.key : target.kind === "tool" ? target.tool.key : target.kind === "memory-evidence" ? `${target.sessionId}:${target.snapshotId}` : undefined;
  useEffect(() => {
    if (selectionKey) closeButton.current?.focus();
  }, [selectionKey]);
  return (
    <aside
      aria-label="Inspector"
      onKeyDown={(event) => {
        if (!selectionKey || event.key !== "Tab" || !window.matchMedia(target.kind === "file" ? "(max-width: 639px)" : "(max-width: 767px)").matches) return;
        const controls = [...event.currentTarget.querySelectorAll<HTMLElement>('button, summary, a[href], [tabindex="0"]')].filter((node) => node.tabIndex >= 0 && !node.matches(":disabled") && node.getClientRects().length > 0);
        const destination = event.shiftKey && document.activeElement === controls[0] ? controls.at(-1) : !event.shiftKey && document.activeElement === controls.at(-1) ? controls[0] : undefined;
        if (destination) { event.preventDefault(); destination.focus(); }
      }}
      className={`${focused ? "min-w-0 flex-1" : target.kind === "file" ? "w-[min(720px,48vw)] max-sm:absolute max-sm:inset-0 max-sm:z-30 max-sm:w-full" : "w-[min(460px,42vw)] max-md:absolute max-md:inset-0 max-md:z-30 max-md:w-full"} ${target.kind === "file" ? "bg-editor border-editor-hair" : "bg-inspector border-hair"} flex min-h-0 flex-none flex-col border-l`}
    >
      <header className="border-hair flex h-[44px] flex-none items-center gap-2 border-b px-3">
        {target.kind === "file" ? <FilePath file={target.file} /> : <>
          <span className="text-faint"><TargetIcon target={target} /></span>
          <span className="text-body min-w-0 flex-1 truncate text-xs font-medium">{targetTitle(target)}</span>
          <span className="text-ghost border-hair-strong rounded border px-2 py-0.5 font-mono text-[9.5px]">Inspector</span>
        </>}
        <button ref={closeButton} type="button" onClick={onClose} aria-label="Close inspector" className="text-faint hover:text-body focus-visible:ring-teal rounded p-1.5 focus-visible:ring-1 focus-visible:outline-none">
          <Cross size={13} />
        </button>
      </header>

      <div className={target.kind === "file" ? "flex min-h-0 flex-1 flex-col" : "min-h-0 flex-1 overflow-y-auto"}>
        <InspectorContent target={target} />
      </div>
    </aside>
  );
}

export function InspectorContent({target}: {target: InspectorTarget}) {
  return <>
        {target.kind === "file" && <FileViewer key={target.file.key} file={target.file} />}
        {target.kind === "tool" && <ToolInspection key={target.tool.key} tool={target.tool} />}
        {target.kind === "memory" && <MemoryInspection detail={target.detail} />}
        {target.kind === "memory-evidence" && <MemoryEvidence key={`${target.sessionId}:${target.snapshotId}`} sessionId={target.sessionId} snapshotId={target.snapshotId} memoryTools={target.memoryTools} />}
        {target.kind === "repository-instructions" && <InstructionsView target={target}/>}
        {target.kind === "workspace" && <WorkspaceInspection workspace={target.workspace} />}
        {target.kind === "scope" && <ScopeInspection scope={target.scope} />}
        {target.kind === "file-diff" && <FileInspection target={target} />}
        {target.kind === "data" && <DataInspection />}
        {target.kind === "empty" && <EmptyInspection />}
  </>;
}

function FileInspection({ target }: { target: Extract<InspectorTarget, { kind: "file-diff" }> }) {
  return (
    <div>
      <div className="p-5 pb-4">
        <InspectorLabel>Complete file change</InspectorLabel>
        <h2 className="text-ink mt-2 break-all font-mono text-[12px] font-medium">{target.path}</h2>
        <p className="text-fainter mt-1 text-[10.5px]">Approval {target.state}</p>
      </div>
      <Diff oldText={target.oldText} newText={target.newText} isNew={target.isNew} />
    </div>
  );
}

function MemoryInspection({ detail }: { detail: SemanticObjectInspection }) {
  return (
    <div className="p-5">
      <InspectorLabel>Memory record</InspectorLabel>
      <h2 className="text-ink mt-2 text-[16px] font-semibold">{memoryTitle(detail)}</h2>
      <MetaLine>{detail.object_kind}:{detail.object_id}</MetaLine>
      <Definition label="Scope" value={detail.scope.scope_key} />
      <Definition label="Status" value={detail.status} />
      {detail.entity && <Definition label="Entity type" value={detail.entity.entity_type} />}
      {detail.claim && (
        <>
          <Definition label="Predicate" value={`${detail.claim.predicate.label} · v${detail.claim.predicate.version}`} />
          <Definition label="Value" value={claimObject(detail.claim)} />
          <Definition label="Valid time" value={`${formatTime(detail.claim.valid_time.from)} – ${formatTime(detail.claim.valid_time.to)}`} />
          <Definition label="Transaction time" value={formatTime(detail.claim.transaction_time)} />
        </>
      )}

      <SectionTitle title="Evidence" count={detail.sources.length} />
      {detail.sources.length === 0 && <EmptyLine>No source links</EmptyLine>}
      {detail.sources.map(({ source }) => (
        <div key={source.source_link_id ?? source.event_id} className="border-hair mt-3 border-l pl-3">
          <p className="text-teal text-[10px]">Source episode · <span className="font-mono">{source.event_id}</span></p>
          <p className="text-body text-xs leading-5">{source.evidence || (source.authority === "evie_proposed" ? "Evie proposed this value; it is not in your words." : "Source text is not available in this scope.")}</p>
          <p className="text-fainter mt-2 font-mono text-[10px] leading-4">{source.authority} · {source.eligibility}<br />{source.source_scope_key}<br />{formatTime(source.observed_at)}</p>
        </div>
      ))}

      <SectionTitle title="Lifecycle" count={detail.lifecycle.length} />
      {detail.lifecycle.map((state) => (
        <div key={`${state.operation_id}:${state.state}`} className="border-hair flex gap-3 border-b py-2.5">
          <span className="text-teal text-xs">{state.state}</span>
          <span className="text-fainter ml-auto text-right font-mono text-[10px]">revision {state.scope_revision}<br />{formatTime(state.transaction_time)}</span>
        </div>
      ))}

      {detail.conflicts.length > 0 && (
        <>
          <SectionTitle title="Conflicts" count={detail.conflicts.length} />
          {detail.conflicts.map((conflict) => <p key={`${conflict.code}:${conflict.claim_ids.join(":")}`} className="text-amber mt-2 text-xs">{conflict.code}: {conflict.claim_ids.join(", ")}</p>)}
        </>
      )}

      <SectionTitle title="Operations" count={detail.operations.length} />
      {detail.operations.map((operation) => (
        <div key={operation.operation_id} className="border-hair border-b py-3">
          <div className="text-body text-xs">{operation.kind}</div>
          <div className="text-fainter mt-1 break-all font-mono text-[9.5px] leading-4">{operation.operation_id}<br />schema v{operation.schema_version} · {formatTime(operation.transaction_time)}</div>
        </div>
      ))}
    </div>
  );
}

function WorkspaceInspection({ workspace }: { workspace: Workspace }) {
  return (
    <div className="p-5">
      <InspectorLabel>Workspace boundary</InspectorLabel>
      <h2 className="text-ink mt-2 text-[16px] font-semibold">{workspace.displayName}</h2>
      <p className="text-muted-text mt-3 text-xs leading-5">Sessions in this workspace can use global memory, this workspace&apos;s memory, and their own session memory. Other workspace and project scopes remain excluded.</p>
      <Definition label="State" value={workspace.state} />
      <Definition label="Revision" value={workspace.currentRevisionId} mono />
      <Definition label="Workspace ID" value={workspace.id} mono />
      <SectionTitle title="Inspector surfaces" />
      <EmptyLine>Workspace memory, resources, access, and workflows will open here as their APIs become available.</EmptyLine>
    </div>
  );
}

function ScopeInspection({ scope }: { scope: ContextScope }) {
  return (
    <div className="p-5">
      <InspectorLabel>Conversation scope</InspectorLabel>
      <h2 className="text-ink mt-2 text-[16px] font-semibold">{scope.displayName}</h2>
      <p className="text-muted-text mt-3 text-xs leading-5">This scope was selected when the session was created and cannot change during the conversation.</p>
      <Definition label="Context" value={scope.kind === "workspace" ? "Workspace" : scope.kind === "project" ? "Filesystem project" : "Unscoped"} />
      {scope.workspaceRevision && <Definition label="Pinned revision" value={scope.workspaceRevision} mono />}
      {scope.projectRoot && <Definition label="Root snapshot" value={scope.projectRoot} mono />}
    </div>
  );
}

function DataInspection() {
  return (
    <div className="flex min-h-full flex-col items-center justify-center px-8 py-14 text-center">
      <span className="border-hair-strong text-faint flex h-10 w-10 items-center justify-center rounded-[10px] border"><Database size={18} /></span>
      <h2 className="text-body mt-4 text-sm font-medium">Inspect one object</h2>
      <p className="text-fainter mt-2 max-w-[280px] text-xs leading-5">Choose a Claim or Entity to inspect its exact value, evidence, lifecycle, and operation history here.</p>
    </div>
  );
}

function EmptyInspection() {
  return (
    <div className="flex min-h-full flex-col items-center justify-center px-8 py-14 text-center">
      <span className="border-hair-strong text-faint flex h-10 w-10 items-center justify-center rounded-[10px] border"><FileIcon size={18} /></span>
      <h2 className="text-body mt-4 text-sm font-medium">Nothing open</h2>
      <p className="text-fainter mt-2 max-w-[280px] text-xs leading-5">Files, diffs, previews, database rows, and memory records open in this pane without replacing your work.</p>
    </div>
  );
}

function Definition({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="border-hair mt-4 grid grid-cols-[100px_minmax(0,1fr)] gap-3 border-b pb-3">
      <div className="text-fainter text-[11px]">{label}</div>
      <div className={`${mono ? "font-mono text-[10px] break-all" : "text-xs"} text-body text-right`}>{value}</div>
    </div>
  );
}

function SectionTitle({ title, count }: { title: string; count?: number }) {
  return <h3 className="text-body mt-7 flex items-center gap-2 text-xs font-semibold">{title}{count !== undefined && <span className="text-ghost font-mono text-[10px]">{count}</span>}</h3>;
}

function InspectorLabel({ children }: { children: React.ReactNode }) {
  return <div className="text-teal flex items-center gap-2 text-[11px]"><Layers size={12} />{children}</div>;
}

function EmptyLine({ children }: { children: React.ReactNode }) {
  return <p className="text-fainter mt-2 text-xs leading-5">{children}</p>;
}

function MetaLine({ children }: { children: React.ReactNode }) {
  return <p className="text-fainter mt-1 break-all font-mono text-[10px]">{children}</p>;
}

function TargetIcon({ target }: { target: InspectorTarget }) {
  if (target.kind === "tool") return <Wrench size={13} />;
  if (target.kind === "memory" || target.kind === "memory-evidence" || target.kind === "data") return <Database size={13} />;
  if (target.kind === "workspace" || target.kind === "scope") return <Folder size={13} />;
  return <FileIcon size={13} />;
}



function formatTime(value?: string | null) {
  return value ? new Date(value).toLocaleString() : "Open";
}
