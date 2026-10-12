import { useState } from "react";
import type { Workspace } from "../api/contextSessions";
import { Dialog } from "../ui/Dialog";

// Deleting a Workspace archives it (workspaces.spec.md keeps hard erasure
// separate): it leaves the sidebar and takes no new chats or runs, and its
// conversations and memories stay until it is restored from Settings.
export function DeleteWorkspaceDialog({ workspace, onDelete, onClose }: {
  workspace: Workspace;
  onDelete: (workspace: Workspace) => Promise<void>;
  onClose: () => void;
}) {
  const [deleting, setDeleting] = useState(false);
  const [problem, setProblem] = useState("");
  const remove = async () => {
    setDeleting(true);
    setProblem("");
    try {
      await onDelete(workspace);
      onClose();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "The workspace could not be deleted.");
      setDeleting(false);
    }
  };
  return (
    <Dialog title={`Delete ${workspace.displayName}?`} busy={deleting} onClose={onClose}>
      <p className="text-body text-sm leading-6">It leaves the sidebar, and no new chats or workflow runs can start in it.</p>
      <p className="text-muted-text mt-2 text-xs leading-5">Its conversations and memories are kept. Restore it any time from Settings → Archived workspaces.</p>
      {problem && <p role="alert" className="text-danger mt-4 text-xs">{problem}</p>}
      <div className="mt-6 flex justify-end gap-2">
        <button type="button" data-dialog-autofocus disabled={deleting} onClick={onClose} className="border-hair-input text-body hover:bg-hover focus-visible:ring-teal rounded-md border px-4 py-2 text-xs focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40">Cancel</button>
        <button type="button" disabled={deleting} onClick={() => void remove()} className="border-danger-hair bg-danger-bg text-danger-ink focus-visible:ring-teal rounded-md border px-4 py-2 text-xs font-medium focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40">{deleting ? "Deleting…" : "Delete workspace"}</button>
      </div>
    </Dialog>
  );
}
