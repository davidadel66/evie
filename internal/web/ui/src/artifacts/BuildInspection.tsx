import { useEffect, useState } from "react";
import { changesAPI, type ChangeDetail, type ChangesAPI } from "../api/changes";
import { changeStatus, lineCounts } from "../workspaces/changeView";
import { browserStorage, isViewed, markViewed, readViewed, writeViewed, type ViewedMarks } from "../workspaces/diffView";
import { FileDiff } from "../workspaces/WorkspaceChanges";
import type { BuildRef } from "./chatBuilds";

// A chat's build in the Inspector (changes.spec.md story 6b): the Change it
// made, its files and their diffs, and "Open in Changes", where the owner
// reviews and merges it. Nothing is approved here.

/** One build's Change as the Inspector shows it. */
export function BuildView({build, detail, problem, viewed, now, onOpenChange, onViewed}: {
  build: BuildRef;
  detail?: ChangeDetail;
  problem?: string;
  viewed: ViewedMarks;
  now: Date;
  /** Opens the Change in its Workspace's Changes tab; absent outside a Workspace. */
  onOpenChange?: () => void;
  onViewed: (path: string, viewed: boolean) => void;
}) {
  const change = detail?.change;
  const status = change && detail ? changeStatus(change, detail.repository.kind, now) : undefined;
  return <section aria-label={`Build ${build.branch}`} className="border-hair mb-5 border-b pb-5 last:border-b-0">
    <div className="flex items-start gap-3">
      <div className="min-w-0 flex-1">
        <h2 className="text-ink text-[15px] font-medium">{change?.title ?? build.branch}</h2>
        <p className="text-faint mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
          {status && <span className="text-muted-text">{status.label}</span>}
          <span className="font-mono">{build.branch}{change ? ` → ${change.base}` : ""}</span>
          <span>{lineCounts(change?.insertions ?? build.insertions, change?.deletions ?? build.deletions)}</span>
        </p>
      </div>
      {onOpenChange && <button type="button" onClick={onOpenChange}
        className="bg-teal text-primary-foreground hover:bg-teal-hover flex-none rounded-[7px] px-3 py-[6px] text-xs font-semibold">Open in Changes</button>}
    </div>
    {problem && <p className="text-muted-text mt-3 text-xs">{problem}</p>}
    <div className="mt-4">
      {detail
        ? detail.files.length === 0
          ? <p className="text-fainter text-xs">{detail.problem ?? "No files changed."}</p>
          : detail.files.map((file) => <FileDiff key={file.path} file={file} layout="unified" viewed={isViewed(viewed, file)} onViewed={(value) => onViewed(file.path, value)} />)
        : <ul>{build.files.map((file) => <li key={file.path} className="flex gap-2 py-[3px] text-xs">
          <span className="text-body min-w-0 flex-1 truncate font-mono">{file.path}</span>
          <span className="text-fainter flex-none">{lineCounts(file.added, file.deleted)}</span>
        </li>)}</ul>}
    </div>
  </section>;
}

function BuildSection({build, workspaceId, api, onOpenChange}: {build: BuildRef; workspaceId?: string; api: ChangesAPI; onOpenChange?: (workspaceId: string, changeId: string) => void}) {
  const [detail, setDetail] = useState<ChangeDetail>();
  const [problem, setProblem] = useState<string>();
  const [viewed, setViewed] = useState<ViewedMarks>(() => workspaceId ? readViewed(browserStorage(), workspaceId, build.changeId) : {});
  useEffect(() => {
    if (!workspaceId) return;
    let live = true;
    api.show(workspaceId, build.changeId).then((value) => { if (live) setDetail(value); },
      (error) => { if (live) setProblem(error instanceof Error ? error.message : String(error)); });
    return () => { live = false; };
  }, [api, workspaceId, build.changeId]);
  return <BuildView build={build} detail={detail} viewed={viewed} now={new Date()}
    problem={!workspaceId ? "Diffs and review live in a Workspace's Changes tab; this chat is not in a Workspace." : problem}
    onOpenChange={workspaceId && onOpenChange ? () => onOpenChange(workspaceId, build.changeId) : undefined}
    onViewed={(path, value) => {
      const file = detail?.files.find((candidate) => candidate.path === path);
      if (!file || !workspaceId) return;
      const next = markViewed(viewed, file, value);
      setViewed(next);
      writeViewed(browserStorage(), workspaceId, build.changeId, next);
    }} />;
}

/** The builds one chat row made, each with its Change. */
export function BuildInspection({builds, workspaceId, api = changesAPI, onOpenChange}: {
  builds: BuildRef[];
  workspaceId?: string;
  api?: ChangesAPI;
  onOpenChange?: (workspaceId: string, changeId: string) => void;
}) {
  return <div className="px-5 pt-4 pb-8">
    {builds.map((build) => <BuildSection key={build.changeId} build={build} workspaceId={workspaceId} api={api} onOpenChange={onOpenChange} />)}
  </div>;
}
