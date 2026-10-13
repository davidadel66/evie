import { describeSource, type CustomTab } from "../api/workspaceTabs";

/**
 * Settings → Tabs lists every folder under .evie/tabs/ with whether it
 * shows or what is wrong with it (workspace-tabs.spec.md). A tab with a
 * problem never shows in the header. A tab that declares data reads it only
 * after the owner allows that exact list (story 2); a changed list asks
 * again.
 */
export function WorkspaceTabsSettings({folder, tabs, problem, busy, actionProblem, onRefresh, onApprove, onWithdraw}: {
  folder?: string;
  tabs?: CustomTab[];
  problem?: string;
  /** The tab whose data choice is being saved. */
  busy?: string;
  actionProblem?: string;
  onRefresh?: () => void;
  onApprove?: (tab: CustomTab) => void;
  onWithdraw?: (tab: CustomTab) => void;
}) {
  return <section aria-label="Tabs" className="border-hair mt-7 border-b pb-7">
    <div className="mb-3 flex items-center gap-3">
      <h2 className="text-body flex-1 text-sm font-medium">Tabs</h2>
      {folder && onRefresh && <button type="button" onClick={onRefresh} className="text-muted-text hover:text-body text-xs">Refresh</button>}
    </div>
    {!folder
      ? <p className="text-muted-text max-w-[620px] text-xs leading-5">Give this workspace a folder and it can have its own tabs: each folder in <code>.evie/tabs/</code> with a <code>tab.json</code> and an <code>index.html</code> becomes a tab.</p>
      : problem
        ? <p role="alert" className="text-danger text-xs">{problem}</p>
        : !tabs
          ? <p className="text-muted-text text-xs">Loading tabs…</p>
          : tabs.length === 0
            ? <p className="text-muted-text max-w-[620px] text-xs leading-5">No tabs yet. Each folder in <code>{folder}/.evie/tabs/</code> with a <code>tab.json</code> and an <code>index.html</code> becomes a tab.</p>
            : <ul className="divide-hair divide-y">
              {tabs.map((tab) => <li key={tab.id} className="py-2.5 text-xs">
                <div className="flex items-start gap-3">
                  <span className="text-body w-40 flex-none truncate font-medium" title={tab.title}>{tab.title}</span>
                  <code className="text-faint w-32 flex-none truncate">{tab.id}</code>
                  {tab.problem ? <span className="text-amber-ink min-w-0 flex-1">{tab.problem}</span> : <span className="text-muted-text flex-1">Shown</span>}
                </div>
                {(tab.recentProblems?.length ?? 0) > 0 && <ul aria-label={`Recent problems in ${tab.title}`} className="mt-1.5 ml-[172px] space-y-0.5">
                  {tab.recentProblems?.map((problem, i) => <li key={i} className="text-amber-ink truncate" title={problem.message}>{problem.kind}: {problem.message}</li>)}
                </ul>}
                {!tab.problem && (tab.data?.length ?? 0) > 0 && <div className="mt-2 ml-[172px] flex flex-wrap items-center gap-x-3 gap-y-1">
                  <span className="text-muted-text min-w-0">Reads {(tab.data ?? []).map(describeSource).join(", ")}.</span>
                  {tab.approved
                    ? <><span className="text-ok-ink">Allowed</span>
                      <button type="button" disabled={busy === tab.id} onClick={() => onWithdraw?.(tab)} className="text-muted-text hover:text-body underline underline-offset-2 disabled:opacity-40">Withdraw</button></>
                    : <button type="button" disabled={busy === tab.id} onClick={() => onApprove?.(tab)}
                      className="border-teal-hair text-teal-hover hover:bg-teal-deep rounded-[6px] border px-2.5 py-1 disabled:opacity-40">Allow this data</button>}
                </div>}
              </li>)}
            </ul>}
    {actionProblem && <p role="alert" className="text-danger mt-3 text-xs">{actionProblem}</p>}
  </section>;
}
