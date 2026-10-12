import type { WorkflowCapStatus } from "../api/workflows";
import { reviewText, waitingText, workflowSpendLine, workspaceTabs, type WorkspaceTab } from "./workspaceActivity";

// The Workspace header under the top bar (workspace-activity.spec.md): the
// tabs, badges for runs and Changes waiting on the owner, and today's
// workflow spend.
// The top bar's breadcrumb already names the Workspace.

export function WorkspaceHeader({tab, waiting, review = 0, showChanges = false, caps, onTab}: {
  tab: WorkspaceTab;
  waiting: number;
  /** Changes waiting on the owner (changes.spec.md story 6a). */
  review?: number;
  /** The Changes tab shows only for a Workspace with a Repository. */
  showChanges?: boolean;
  caps?: WorkflowCapStatus;
  onTab: (tab: WorkspaceTab) => void;
}) {
  return <div className="border-hair bg-topbar flex h-[42px] flex-none items-stretch gap-6 border-b px-6 sm:px-10">
    <div role="tablist" aria-label="Workspace sections" className="flex min-w-0 items-stretch gap-[22px]">
      {workspaceTabs.filter(({id}) => id !== "changes" || showChanges).map(({id, label}) => {
        const selected = tab === id;
        const badge = id === "runs" ? waiting : id === "changes" ? review : 0;
        return <button key={id} type="button" role="tab" id={`workspace-tab-${id}`} aria-selected={selected} aria-controls="workspace-tab-panel"
          onClick={() => onTab(id)}
          className={`${selected ? "border-teal text-ink" : "text-muted-text hover:text-body border-transparent"} focus-visible:text-ink -mb-px flex flex-none items-center gap-[7px] border-b-2 px-0.5 text-[13px] font-medium focus-visible:outline-none`}>
          {label}
          {badge > 0 && <>
            <span aria-hidden="true" title="Waiting on you"
              className="border-amber-hair-strong bg-amber-bg text-amber-ink min-w-4 rounded-full border px-[5px] text-center text-[10.5px] leading-[14px]">{badge}</span>
            <span className="sr-only">, {id === "changes" ? reviewText(badge) : waitingText(badge)}</span>
          </>}
        </button>;
      })}
    </div>
    <div className="flex-1" />
    {caps && <div className={`${caps.reached ? "text-danger" : "text-faint"} hidden items-center text-[11.5px] sm:flex`}>{workflowSpendLine(caps)}</div>}
  </div>;
}
