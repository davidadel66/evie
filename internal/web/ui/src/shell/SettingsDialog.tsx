import { useEffect, useRef, useState } from "react";
import type { ContextSessionSnapshot, StoredSession } from "../api/contextSessions";
import { Dialog } from "../ui/Dialog";
import { Archive, ChevronLeft, ChevronRight, MessageSquare } from "../ui/Icon";
import type { ChatTextSize } from "../ui/textSize";
import { resolveChatFont, type ChatFont } from "../ui/chatFont";

type Props = {
  snapshot?: ContextSessionSnapshot;
  problem?: string | null;
  onRefresh?: () => Promise<void>;
  busy: boolean;
  textSize: ChatTextSize;
  onTextSize: (size: ChatTextSize) => void;
  font: ChatFont;
  onFont: (font: ChatFont) => void;
  onRestore: (session: StoredSession) => Promise<void>;
  onClose: () => void;
};

export function SettingsDialog({ snapshot, problem, onRefresh, busy, textSize, onTextSize, font, onFont, onRestore, onClose }: Props) {
  const [archivedOpen, setArchivedOpen] = useState(false);
  const [error, setError] = useState("");
  const [restoring, setRestoring] = useState<string>();
  const [restoredTitle, setRestoredTitle] = useState("");
  const [refreshing, setRefreshing] = useState(false);
  const [refreshProblem, setRefreshProblem] = useState("");
  const refreshingRef = useRef(false);
  const restoringRef = useRef(false);
  const backButton = useRef<HTMLButtonElement>(null);
  const archivedButton = useRef<HTMLButtonElement>(null);
  const focusAfterChange = useRef<"back" | "archived" | undefined>(undefined);
  useEffect(() => {
    const target = focusAfterChange.current;
    if (!target || restoring || refreshing) return;
    (target === "back" ? backButton : archivedButton).current?.focus();
    focusAfterChange.current = undefined;
  }, [archivedOpen, restoring, refreshing]);
  const showArchived = (open: boolean) => {
    if (restoringRef.current) return;
    focusAfterChange.current = open ? "back" : "archived";
    setArchivedOpen(open);
    setError("");
    setRestoredTitle("");
  };
  const refresh = async () => {
    if (refreshingRef.current || !onRefresh) return;
    refreshingRef.current = true;
    setRefreshing(true);
    setRefreshProblem("");
    try { await onRefresh(); }
    catch (error) { setRefreshProblem(error instanceof Error ? error.message : "Sessions could not be loaded. Try again."); }
    finally {
      refreshingRef.current = false;
      if (backButton.current) focusAfterChange.current = "back";
      setRefreshing(false);
    }
  };
  const restore = async (session: StoredSession) => {
    if (restoringRef.current || busy) return;
    restoringRef.current = true;
    setRestoring(session.id);
    setError("");
    setRestoredTitle("");
    try {
      await onRestore(session);
      setRestoredTitle(session.title.trim() || "Untitled session");
    }
    catch (error) { setError(error instanceof Error ? error.message : "Session could not be restored."); }
    finally {
      restoringRef.current = false;
      focusAfterChange.current = "back";
      setRestoring(undefined);
    }
  };
  return (
    <Dialog title={archivedOpen ? "Archived sessions" : "Settings"} onClose={onClose} busy={!!restoring}>
      {archivedOpen ? <>
        <button ref={backButton} type="button" disabled={!!restoring} onClick={() => showArchived(false)} className="text-muted-text hover:text-ink focus-visible:ring-teal mb-5 flex items-center gap-1 rounded text-xs focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40"><ChevronLeft size={13} /> Settings</button>
        <p className="text-muted-text mb-5 text-xs leading-5">Restore a conversation to bring it back to your workspace.</p>
        <ArchivedSessions snapshot={snapshot} problem={refreshProblem || problem} refreshing={refreshing} onRefresh={onRefresh ? () => void refresh() : undefined} busy={busy || !!restoring} restoring={restoring} onRestore={session => void restore(session)} />
      </> : <>
        <section aria-label="Conversation appearance">
          <h3 className="text-body mb-4 text-sm font-medium">Conversation appearance</h3>
          <label className="text-body flex items-center justify-between gap-4 text-xs">
            Font
            <select value={font} onChange={event => onFont(resolveChatFont(event.target.value))} className="border-hair-input bg-card focus:border-teal rounded-md border px-3 py-2 outline-none">
              <option value="default">IBM Plex Sans</option>
              <option value="system">System</option>
              <option value="serif">Serif</option>
            </select>
          </label>
          <fieldset className="mt-5">
            <legend className="text-body mb-2 text-xs">Text size</legend>
            <div className="flex gap-2">
              {([['compact', 'Small', '13px'], ['default', 'Default', '15px'], ['large', 'Large', '17px']] as const).map(([value, label, size]) => (
                <button key={value} type="button" aria-pressed={textSize === value} onClick={() => onTextSize(value)} className={`${textSize === value ? "border-teal text-ink bg-teal-deep" : "border-hair-input text-muted-text hover:text-body"} focus-visible:ring-teal flex min-w-0 flex-1 flex-col items-center gap-1 rounded-md border px-2 py-2.5 text-xs focus-visible:ring-1 focus-visible:outline-none`}>
                  {label}<span className="text-[10px]">{size}</span>
                </button>
              ))}
            </div>
          </fieldset>
          <p className="border-hair text-body mt-4 rounded-lg border p-4 leading-relaxed" style={{ fontSize: textSize === "compact" ? 13 : textSize === "large" ? 17 : 15, fontFamily: font === "serif" ? "Georgia, serif" : font === "system" ? "system-ui, sans-serif" : "var(--font-sans)" }}>A little space to think, plan, and make progress.</p>
        </section>
        <button ref={archivedButton} type="button" onClick={() => showArchived(true)} className="border-hair text-body hover:text-ink focus-visible:ring-teal mt-6 flex w-full items-center gap-3 border-t pt-5 text-left focus-visible:ring-1 focus-visible:outline-none">
          <Archive size={16} /><span className="flex-1 text-sm">Archived sessions</span><span className="text-muted-text text-xs">{snapshot ? snapshot.archivedSessions?.length ?? 0 : "…"}</span><ChevronRight size={14} />
        </button>
      </>}
      {error && <p role="alert" className="text-danger mt-4 text-xs">{error}</p>}
      {restoredTitle && <p role="status" className="text-muted-text mt-4 text-xs">Restored {restoredTitle}.</p>}
    </Dialog>
  );
}

export function ArchivedSessions({ snapshot, problem, refreshing, onRefresh, busy, restoring, onRestore }: { snapshot?: ContextSessionSnapshot; problem?: string | null; refreshing?: boolean; onRefresh?: () => void; busy: boolean; restoring?: string; onRestore: (session: StoredSession) => void }) {
  if (!snapshot) return problem ? <div className="py-6">
    <p role="alert" className="text-danger-ink text-xs leading-5">{problem}</p>
    {onRefresh && <button type="button" disabled={refreshing} onClick={onRefresh} className="border-hair-input text-body focus-visible:ring-teal mt-3 rounded-md border px-3 py-2 text-xs focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40">{refreshing ? "Retrying…" : "Retry"}</button>}
  </div> : <p role="status" className="text-muted-text py-6 text-sm">Loading sessions…</p>;
  const sessions = snapshot.archivedSessions ?? [];
  if (!sessions.length) return <p className="text-muted-text border-hair rounded-lg border border-dashed px-5 py-8 text-center text-sm">No archived sessions.</p>;
  return <ul className="divide-hair divide-y">
    {sessions.map(session => {
      const scope = session.workspaceId ? snapshot.workspaces.find(workspace => workspace.id === session.workspaceId)?.displayName ?? "Workspace" : session.projectId ? snapshot.projects.find(project => project.id === session.projectId)?.displayName ?? "Project" : "Unscoped";
      return <li key={session.id} className="flex items-center gap-3 py-3">
        <MessageSquare size={14} className="text-faint" />
        <div className="min-w-0 flex-1"><p className="text-body truncate text-xs" title={session.title}>{session.title.trim() || "Untitled session"}</p><p className="text-muted-text mt-1 truncate text-[11px]">{scope}</p></div>
        <button type="button" disabled={busy} onClick={() => onRestore(session)} aria-label={`Restore ${session.title.trim() || "Untitled session"}`} className="border-hair-input text-body hover:border-teal focus-visible:ring-teal rounded-md border px-3 py-1.5 text-xs focus-visible:ring-1 focus-visible:outline-none disabled:opacity-40">{restoring === session.id ? "Restoring…" : "Restore"}</button>
      </li>;
    })}
  </ul>;
}
