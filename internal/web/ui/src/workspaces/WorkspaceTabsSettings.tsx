import type { CustomTab } from "../api/workspaceTabs";

/**
 * Settings → Tabs lists every folder under .evie/tabs/ with whether it
 * shows or what is wrong with it (workspace-tabs.spec.md). A tab with a
 * problem never shows in the header.
 */
export function WorkspaceTabsSettings({folder, tabs, problem, onRefresh}: {folder?: string; tabs?: CustomTab[]; problem?: string; onRefresh?: () => void}) {
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
              {tabs.map((tab) => <li key={tab.id} className="flex items-start gap-3 py-2.5 text-xs">
                <span className="text-body w-40 flex-none truncate font-medium" title={tab.title}>{tab.title}</span>
                <code className="text-faint w-32 flex-none truncate">{tab.id}</code>
                {tab.problem ? <span className="text-amber-ink min-w-0 flex-1">{tab.problem}</span> : <span className="text-muted-text flex-1">Shown</span>}
              </li>)}
            </ul>}
  </section>;
}
