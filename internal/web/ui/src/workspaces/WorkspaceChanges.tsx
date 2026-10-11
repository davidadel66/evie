import { useCallback, useEffect, useState } from "react";
import type { Workspace } from "../api/contextSessions";
import { changesAPI, type Change, type ChangeDetail, type ChangesAPI, type ChangeList as ChangeListing, type Check, type ComparedFile } from "../api/changes";
import { ago } from "../ui/time";
import {
  changeNumber, changeOrigin, changeStatus, checksSummary, diffBar, groupChanges, lineCounts,
  type ChangeAction, type ChangeGroup, type ChangeTone,
} from "./changeView";
import {
  browserStorage, isViewed, markViewed, parsePatch, readLayout, readViewed, splitPath, splitRows, wordSpans, writeLayout, writeViewed,
  type DiffLayout, type DiffLine, type Hunk, type ViewedMarks,
} from "./diffView";

// The Changes tab (changes.spec.md story 6a): the Workspace's branches of
// work grouped by what they need from the owner, and a review page for one
// of them with its checks, where it came from, Evie's notes and its diff,
// side by side or unified, and the one action that moves it on.

const dot: Record<ChangeTone, string> = { amber: "bg-amber", teal: "bg-teal", danger: "bg-danger", draft: "bg-fainter", merged: "bg-ok-gutter", closed: "bg-ghost" };
const ink: Record<ChangeTone, string> = { amber: "text-amber", teal: "text-teal", danger: "text-danger", draft: "text-muted-text", merged: "text-muted-text", closed: "text-muted-text" };
const checkDot = (check: Check) => check.state === "failed" ? "bg-danger" : check.state === "pending" || check.state === "running" ? "bg-teal" : "bg-ok";
const checkWord = (check: Check) => check.state === "failed" ? "Failed" : check.state === "pending" || check.state === "running" ? "Running" : check.state === "neutral" ? "Skipped" : "Passed";
const checkInk = (check: Check) => check.state === "failed" ? "text-danger" : check.state === "pending" || check.state === "running" ? "text-teal" : "text-ok";
const summaryInk = { muted: "text-muted-text", teal: "text-teal", danger: "text-danger", none: "text-fainter" };

function DiffBar({additions, deletions}: {additions?: number; deletions?: number}) {
  return <span aria-hidden="true" className="flex gap-0.5">
    {diffBar(additions, deletions).map((segment, i) => <span key={i} className={`h-1 w-[9px] rounded-[1px] ${segment === "add" ? "bg-ok" : segment === "del" ? "bg-danger" : "bg-hair-strong"}`} />)}
  </span>;
}

/** The tab's list: each group of Changes with its rows. */
export function ChangeList({groups, kind, now, onSelect}: {groups: ChangeGroup[]; kind: "local" | "github" | "none"; now: Date; onSelect: (change: Change) => void}) {
  const columns = "grid grid-cols-[minmax(0,1fr)_84px_92px_64px] gap-4";
  if (groups.length === 0) {
    return <p className="border-hair text-muted-text border-t px-2.5 py-4 text-[12.5px]">No Changes yet. When Evie builds something in this Workspace, it shows up here for you to review.</p>;
  }
  return <>
    <div className={`${columns} text-fainter px-2.5 pb-2 text-[11px]`}><span>Change</span><span>Checks</span><span>Diff</span><span className="text-right">Updated</span></div>
    {groups.map((group) => <section key={group.kind} aria-label={group.label} className="mb-6">
      <h3 className={`${group.kind === "review" ? "text-amber" : group.kind === "progress" ? "text-muted-text" : "text-faint"} flex items-baseline gap-2 px-2.5 pb-2 text-[11px] font-medium`}>
        {group.label}<span className="text-fainter font-normal">{group.changes.length}</span>
      </h3>
      <ul className="border-hair border-t">
        {group.changes.map((change) => {
          const status = changeStatus(change, kind, now);
          const summary = checksSummary(change.checks);
          const origin = changeOrigin(change);
          return <li key={change.id}>
            <button type="button" onClick={() => onSelect(change)} className={`${columns} border-hair hover:bg-hover w-full items-center border-b px-2.5 py-3 text-left`}>
              <span className="flex min-w-0 flex-col gap-1">
                <span className="flex min-w-0 items-center gap-2">
                  <span aria-hidden="true" className={`h-2 w-2 flex-none rounded-full ${dot[status.tone]}`} />
                  <span className="text-ink truncate text-[13.5px]">{change.title}</span>
                </span>
                <span className="text-faint flex min-w-0 items-baseline gap-2 pl-4 text-xs">
                  <span className={`${ink[status.tone]} flex-none`}>{status.label}</span>
                  <span className="flex-none font-mono">{changeNumber(change)}</span>
                  <span className="truncate font-mono">{change.branch}</span>
                  {origin && <span className="flex-none">{origin}</span>}
                </span>
              </span>
              <span className="flex flex-col gap-1">
                {(change.checks?.length ?? 0) > 0 && <span aria-hidden="true" className="flex gap-1">{change.checks!.map((check) => <span key={check.name} className={`h-[7px] w-[7px] rounded-full ${checkDot(check)}`} />)}</span>}
                <span className={`${summaryInk[summary.tone]} text-[11.5px]`}>{summary.label}</span>
              </span>
              <span className="flex flex-col gap-1">
                <span className="text-muted-text font-mono text-xs">{lineCounts(change.insertions, change.deletions)}</span>
                <DiffBar additions={change.insertions} deletions={change.deletions} />
              </span>
              <span className="text-faint text-right text-[11.5px]">{ago(change.updated_at, now)}</span>
            </button>
          </li>;
        })}
      </ul>
    </section>)}
  </>;
}

/** One line's text, with its changed part marked when it has a paired line. */
function LineText({line, pair}: {line: DiffLine; pair?: DiffLine}) {
  const spans = pair && line.kind !== "context" ? (line.kind === "del" ? wordSpans(line.text, pair.text)?.before : wordSpans(pair.text, line.text)?.after) : undefined;
  if (!spans) return <>{line.text || " "}</>;
  return <>{spans[0]}<mark className={`${line.kind === "del" ? "bg-danger/35" : "bg-ok/35"} text-inherit rounded-[2px]`}>{spans[1]}</mark>{spans[2]}</>;
}

const lineBg = (line?: DiffLine) => !line ? "bg-panel" : line.kind === "add" ? "bg-ok/10" : line.kind === "del" ? "bg-danger/10" : "";
const gutter = (line?: DiffLine) => !line ? "bg-panel" : line.kind === "add" ? "bg-ok/15 text-ok-ink" : line.kind === "del" ? "bg-danger/15 text-danger-ink" : "text-ghost";
const lineInk = (line?: DiffLine) => line && line.kind === "context" ? "text-diff-context" : "text-body";

function HunkView({hunk, layout}: {hunk: Hunk; layout: DiffLayout}) {
  const band = <div className="bg-card text-faint px-3 py-1 font-mono text-[11.5px]">{hunk.header}</div>;
  if (layout === "unified") {
    return <div>{band}
      {hunk.lines.map((line, i) => line.kind === "note"
        ? <div key={i} className="bg-card text-faint px-3 py-1 font-mono text-[11.5px] italic">{line.text}</div>
        : <div key={i} className={`grid grid-cols-[44px_44px_1fr] ${lineBg(line)}`}>
          <span className={`${gutter(line)} px-2 text-right select-none`}>{line.old ?? ""}</span>
          <span className={`${gutter(line)} px-2 text-right select-none`}>{line.new ?? ""}</span>
          <span className={`${lineInk(line)} pr-3 pl-2 whitespace-pre`}>{line.kind === "add" ? "+" : line.kind === "del" ? "−" : " "}<LineText line={line} /></span>
        </div>)}
    </div>;
  }
  return <div>{band}
    {splitRows(hunk).map((row, i) => row.note
      ? <div key={i} className="bg-card text-faint px-3 py-1 font-mono text-[11.5px] italic">{row.note}</div>
      : <div key={i} className="grid grid-cols-[44px_minmax(0,1fr)_44px_minmax(0,1fr)]">
        <span className={`${gutter(row.left)} px-2 text-right select-none`}>{row.left?.old ?? ""}</span>
        <span className={`${lineBg(row.left)} ${lineInk(row.left)} overflow-hidden pr-3 pl-2 whitespace-pre`}>{row.left && <LineText line={row.left} pair={row.right !== row.left ? row.right : undefined} />}</span>
        <span className={`${gutter(row.right)} border-hair border-l px-2 text-right select-none`}>{row.right?.new ?? ""}</span>
        <span className={`${lineBg(row.right)} ${lineInk(row.right)} overflow-hidden pr-3 pl-2 whitespace-pre`}>{row.right && <LineText line={row.right} pair={row.right !== row.left ? row.left : undefined} />}</span>
      </div>)}
  </div>;
}

/** One file's card: its path, counts and "Viewed" mark, then its hunks unless viewed. */
export function FileDiff({file, layout, viewed, onViewed}: {file: ComparedFile; layout: DiffLayout; viewed: boolean; onViewed: (viewed: boolean) => void}) {
  const {dir, name} = splitPath(file.path);
  const hunks = parsePatch(file.patch);
  const why = file.excluded ? "Review withholds this file: it may hold secrets."
    : file.binary ? "Binary file."
    : file.truncated ? "Too large to show here."
    : hunks.length === 0 ? (file.status === "renamed" ? `Renamed from ${file.old_path}, with no changes inside.` : "No changes to show.")
    : "";
  return <section id={`change-file-${file.path}`} aria-label={file.path} className="border-hair bg-code mb-4 overflow-hidden rounded-lg border">
    <div className="bg-card border-hair sticky top-0 z-[1] flex items-center gap-3 border-b px-3 py-2">
      <span className="min-w-0 flex-1 truncate font-mono text-xs" title={file.path}>
        <span className="text-faint">{dir}</span><span className="text-ink">{name}</span>
        {file.old_path && <span className="text-faint"> ← {file.old_path}</span>}
      </span>
      <span className="text-muted-text flex-none font-mono text-[11px]">{lineCounts(file.additions, file.deletions)}</span>
      <label className="text-muted-text flex flex-none cursor-pointer items-center gap-1.5 text-[11.5px]">
        <input type="checkbox" checked={viewed} onChange={(event) => onViewed(event.target.checked)} className="accent-teal h-[11px] w-[11px]" />
        Viewed
      </label>
    </div>
    {!viewed && (why
      ? <p className="text-faint px-3 py-3 text-xs">{why}</p>
      : <div className="overflow-x-auto font-mono text-xs leading-[1.7]"><div className="min-w-[560px]">{hunks.map((hunk, i) => <HunkView key={i} hunk={hunk} layout={layout} />)}</div></div>)}
  </section>;
}

/** The review page's header action: the primary button for the Change's state. */
function ActionButton({action, busy, onAction}: {action: ChangeAction; busy: boolean; onAction: (action: ChangeAction) => void}) {
  return <button type="button" disabled={busy || action.disabled} onClick={() => onAction(action)}
    className={`${action.muted ? "bg-selected text-body hover:text-ink" : "bg-teal text-primary-foreground hover:bg-teal-hover"} rounded-[7px] px-3.5 py-[7px] text-[12.5px] font-semibold disabled:opacity-40`}>
    {action.label}
  </button>;
}

/** The review page for one Change. */
export function ChangeReview({detail, now, busy, message, problem, viewed, layout, sessionTitles = {}, confirmClose,
  onBack, onAction, onClose, onConfirmClose, onViewed, onLayout, onOpenRun, onOpenSession}: {
  detail: ChangeDetail;
  now: Date;
  busy: boolean;
  /** What the last action did. */
  message?: string;
  /** Why the last action did nothing. */
  problem?: string;
  viewed: ViewedMarks;
  layout: DiffLayout;
  sessionTitles?: Record<string, string>;
  /** Close was asked for and awaits confirmation. */
  confirmClose: boolean;
  onBack: () => void;
  onAction: (action: ChangeAction) => void;
  onClose: () => void;
  onConfirmClose: (confirmed: boolean) => void;
  onViewed: (file: ComparedFile, viewed: boolean) => void;
  onLayout: (layout: DiffLayout) => void;
  onOpenRun?: (runId: string) => void;
  onOpenSession?: (sessionId: string) => void;
}) {
  const {change, repository, files} = detail;
  const status = changeStatus(change, repository.kind, now);
  const finished = change.state === "merged" || change.state === "closed";
  const seen = files.filter((file) => isViewed(viewed, file)).length;
  const notes = (change.notes ?? "").split(/\n\s*\n/).map((paragraph) => paragraph.trim()).filter(Boolean);
  const pull = change.pull_request;
  return <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
    <header className="border-hair border-b px-6 pt-3.5 pb-4 sm:px-7">
      <button type="button" onClick={onBack} className="text-muted-text hover:text-body text-xs">← Changes</button>
      <div className="mt-2 flex flex-wrap items-start gap-x-6 gap-y-3">
        <div className="min-w-0 flex-1">
          <h2 className="text-ink text-lg font-medium">
            {change.title}{" "}
            {pull ? <a href={pull.url} target="_blank" rel="noreferrer" className="text-faint hover:text-teal font-mono text-[13px] font-normal">#{pull.number}</a>
              : <span className="text-faint font-mono text-[13px] font-normal">Draft</span>}
          </h2>
          <div className="text-faint mt-1.5 flex flex-wrap items-center gap-x-3.5 gap-y-1 text-xs">
            <span className="flex items-center gap-1.5"><span aria-hidden="true" className={`h-2 w-2 rounded-full ${dot[status.tone]}`} /><span className={ink[status.tone]}>{status.label}</span></span>
            <span className="font-mono">{change.branch} → {change.base}</span>
            <span>{lineCounts(change.insertions, change.deletions)}</span>
            <span>{files.length === 1 ? "1 file" : `${files.length} files`}</span>
            <span>Updated {ago(change.updated_at, now).toLowerCase()}</span>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2.5">
          <span className="text-faint text-xs">{status.note}</span>
          {!finished && !confirmClose && <button type="button" disabled={busy} onClick={onClose}
            className="border-hair-input text-body hover:text-ink rounded-[7px] border px-3.5 py-[7px] text-[12.5px] font-semibold disabled:opacity-40">Close</button>}
          {status.action && !confirmClose && <ActionButton action={status.action} busy={busy} onAction={onAction} />}
          {confirmClose && <span role="group" aria-label="Close this Change" className="flex items-center gap-2">
            <span className="text-body text-xs">{change.author === "evie" ? "Close it and remove Evie's branch and worktree?" : "Close this pull request on GitHub?"}</span>
            <button type="button" disabled={busy} onClick={() => onConfirmClose(true)} className="bg-danger text-primary-foreground rounded-[7px] px-3 py-[6px] text-xs font-semibold disabled:opacity-40">Close it</button>
            <button type="button" disabled={busy} onClick={() => onConfirmClose(false)} className="text-muted-text hover:text-body px-2 text-xs">Keep it</button>
          </span>}
        </div>
      </div>
      {problem && <p role="alert" className="text-danger mt-3 text-xs">{problem}</p>}
      {message && <p role="status" className="text-muted-text mt-3 text-xs">{message}</p>}
    </header>
    <div className="flex flex-col lg:flex-row">
      <aside className="border-hair flex flex-col gap-6 border-b px-3.5 pt-[18px] pb-6 lg:w-[264px] lg:flex-none lg:border-r lg:border-b-0 lg:pb-8">
        <section>
          <h3 className="text-faint mb-2 flex text-[11px] font-medium"><span className="flex-1">Files</span>{files.length > 0 && <span className="font-normal">{seen} of {files.length} viewed</span>}</h3>
          {files.length === 0 && <p className="text-fainter text-xs">{detail.problem ?? "No files changed."}</p>}
          <ul>{files.map((file) => {
            const done = isViewed(viewed, file);
            return <li key={file.path} className="flex items-center gap-2 py-[3px]">
              <input type="checkbox" aria-label={`Viewed ${file.path}`} checked={done} onChange={(event) => onViewed(file, event.target.checked)} className="accent-teal h-3 w-3 flex-none" />
              <a href={`#change-file-${file.path}`} title={file.path} className={`${done ? "text-faint" : "text-body"} hover:text-ink min-w-0 flex-1 truncate font-mono text-xs`}>{splitPath(file.path).name}</a>
              <span className="text-fainter flex-none text-[11px]">{lineCounts(file.additions, file.deletions)}</span>
            </li>;
          })}</ul>
        </section>
        <section>
          <h3 className="text-faint mb-2 text-[11px] font-medium">Checks</h3>
          {(change.checks?.length ?? 0) === 0
            ? <p className="text-fainter text-xs">{repository.kind === "github" ? (pull ? "No checks ran on this pull request." : "Nothing runs until the branch is pushed.") : "Verify runs before it merges."}</p>
            : <ul>{change.checks!.map((check) => <li key={check.name} className="flex items-center gap-2 py-[3px] text-xs">
              <span aria-hidden="true" className={`h-[7px] w-[7px] flex-none rounded-full ${checkDot(check)}`} />
              {check.url ? <a href={check.url} target="_blank" rel="noreferrer" className="text-body hover:text-ink min-w-0 flex-1 truncate font-mono">{check.name}</a>
                : <span className="text-body min-w-0 flex-1 truncate font-mono">{check.name}</span>}
              <span className={checkInk(check)}>{checkWord(check)}</span>
            </li>)}</ul>}
        </section>
        <section>
          <h3 className="text-faint mb-2 text-[11px] font-medium">Came from</h3>
          {change.origin?.run_id ? <button type="button" onClick={() => onOpenRun?.(change.origin!.run_id!)} className="text-teal hover:text-teal-hover font-mono text-xs">
            {change.origin.workflow} › {change.origin.step} ›</button>
            : change.origin?.session_id ? <button type="button" onClick={() => onOpenSession?.(change.origin!.session_id!)} className="text-teal hover:text-teal-hover text-left text-xs">
              Chat: {sessionTitles[change.origin.session_id]?.trim() || "Untitled session"} ›</button>
            : <p className="text-muted-text text-xs">{pull ? <>Opened on GitHub by <span className="font-mono">{pull.author}</span></> : "Your own branch"}</p>}
        </section>
        <section>
          <h3 className="text-faint mb-2 text-[11px] font-medium">Worktree</h3>
          {change.worktree ? <p className="text-muted-text font-mono text-[11.5px] break-all">{change.worktree}</p>
            : <p className="text-fainter text-xs">{change.author === "evie" ? "Removed." : "None: this branch is the owner's."}</p>}
        </section>
      </aside>
      <div className="min-w-0 flex-1 px-4 pt-5 pb-20 sm:px-6">
        <section aria-label={change.author === "evie" ? "Evie's notes" : "Description"} className="border-hair bg-card mb-6 rounded-lg border px-[18px] py-4">
          <h3 className="text-faint mb-2 text-[11px] font-medium">{change.author === "evie" ? "Evie’s notes" : "Description"}</h3>
          {notes.length === 0 ? <p className="text-fainter text-[13px]">No notes.</p>
            : notes.map((paragraph, i) => <p key={i} className="text-body mb-2.5 max-w-[76ch] text-[13.5px] leading-[1.6] whitespace-pre-wrap last:mb-0">{paragraph}</p>)}
        </section>
        <div className="mb-4 flex items-center gap-3">
          <div role="radiogroup" aria-label="Diff layout" className="border-hair bg-code flex rounded-[7px] border p-0.5">
            {(["split", "unified"] as const).map((value) => <button key={value} type="button" role="radio" aria-checked={layout === value} onClick={() => onLayout(value)}
              className={`${layout === value ? "bg-selected text-ink" : "text-muted-text hover:text-body"} rounded-[5px] px-2.5 py-1 text-xs`}>{value === "split" ? "Side by side" : "Unified"}</button>)}
          </div>
          {detail.truncated && <span className="text-faint text-xs">Some files are too many to show here.</span>}
        </div>
        {detail.problem && files.length === 0 && <p className="text-muted-text text-xs">{detail.problem}</p>}
        {files.map((file) => <FileDiff key={file.path} file={file} layout={layout} viewed={isViewed(viewed, file)} onViewed={(value) => onViewed(file, value)} />)}
      </div>
    </div>
  </div>;
}

const storage = browserStorage;

/** The Changes tab: the list, or one Change's review page when one is open. */
export function WorkspaceChanges({workspace, listing, selected, sessionTitles, api = changesAPI, onSelect, onChanged, onOpenRun, onOpenSession}: {
  workspace: Workspace;
  listing?: ChangeListing;
  /** The Change open on its review page. */
  selected?: string;
  sessionTitles?: Record<string, string>;
  api?: ChangesAPI;
  onSelect: (changeId?: string) => void;
  /** Reads the list again after an action. */
  onChanged: () => void;
  onOpenRun?: (runId: string) => void;
  onOpenSession?: (sessionId: string) => void;
}) {
  const now = new Date();
  const [detail, setDetail] = useState<ChangeDetail>();
  const [loadProblem, setLoadProblem] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string>();
  const [problem, setProblem] = useState<string>();
  const [confirmClose, setConfirmClose] = useState(false);
  const [viewed, setViewed] = useState<ViewedMarks>({});
  const [layout, setLayout] = useState<DiffLayout>(() => readLayout(storage()));

  // Opening the tab asks GitHub for the pull requests now.
  useEffect(() => {
    if (listing?.repository.kind !== "github") return;
    void api.refresh(workspace.id).then(onChanged, () => undefined);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspace.id, listing?.repository.kind]);

  const load = useCallback(async () => {
    if (!selected) return;
    try {
      setDetail(await api.show(workspace.id, selected));
      setLoadProblem(undefined);
    } catch (error) {
      setLoadProblem(error instanceof Error ? error.message : String(error));
    }
  }, [api, workspace.id, selected]);

  useEffect(() => {
    setDetail(undefined);
    setMessage(undefined);
    setProblem(undefined);
    setConfirmClose(false);
    setViewed(selected ? readViewed(storage(), workspace.id, selected) : {});
    void load();
  }, [load, selected, workspace.id]);

  useEffect(() => {
    if (!selected) return;
    const onEscape = (event: KeyboardEvent) => { if (event.key === "Escape" && !confirmClose) onSelect(undefined); };
    window.addEventListener("keydown", onEscape);
    return () => window.removeEventListener("keydown", onEscape);
  }, [selected, confirmClose, onSelect]);

  const act = async (run: () => Promise<string | undefined>) => {
    setBusy(true);
    setProblem(undefined);
    setMessage(undefined);
    try {
      setMessage(await run());
    } catch (error) {
      setProblem(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
      onChanged();
      await load();
    }
  };

  if (!selected) {
    const kind = listing?.repository.kind ?? "none";
    return <section aria-label="Changes" className="min-h-0 flex-1 overflow-y-auto px-6 pt-6 pb-10 sm:px-10">
      {!listing ? <p role="status" className="text-muted-text text-xs">Loading Changes…</p>
        : <ChangeList groups={groupChanges(listing.changes, kind, now)} kind={kind} now={now} onSelect={(change) => onSelect(change.id)} />}
    </section>;
  }
  if (!detail) {
    return <section className="min-h-0 flex-1 px-6 pt-4 sm:px-7">
      <button type="button" onClick={() => onSelect(undefined)} className="text-muted-text hover:text-body text-xs">← Changes</button>
      {loadProblem ? <p role="alert" className="text-danger mt-4 text-xs">{loadProblem}</p> : <p role="status" className="text-muted-text mt-4 text-xs">Loading the Change…</p>}
    </section>;
  }
  const change = detail.change;
  return <ChangeReview detail={detail} now={now} busy={busy} message={message} problem={problem} viewed={viewed} layout={layout}
    sessionTitles={sessionTitles} confirmClose={confirmClose}
    onBack={() => onSelect(undefined)}
    onAction={(action) => void act(async () => {
      switch (action.kind) {
        case "merge": {
          const merged = await api.merge(workspace.id, change.id);
          return merged.notes?.join(" ") || "Merged.";
        }
        case "arm":
          await api.arm(workspace.id, change.id);
          return "It merges itself once its checks pass.";
        case "disarm":
          await api.disarm(workspace.id, change.id);
          return "It no longer merges by itself.";
        case "push": {
          const pushed = await api.push(workspace.id, change.branch);
          return `Pushed and opened pull request #${pushed.pull_request}.`;
        }
      }
    })}
    onClose={() => setConfirmClose(true)}
    onConfirmClose={(confirmed) => {
      setConfirmClose(false);
      if (confirmed) void act(async () => (await api.close(workspace.id, change.id)).notes?.join(" ") || "Closed.");
    }}
    onViewed={(file, value) => {
      const next = markViewed(viewed, file, value);
      setViewed(next);
      writeViewed(storage(), workspace.id, change.id, next);
    }}
    onLayout={(next) => { setLayout(next); writeLayout(storage(), next); }}
    onOpenRun={onOpenRun} onOpenSession={onOpenSession} />;
}
