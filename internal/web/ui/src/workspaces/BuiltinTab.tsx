import { Spending } from "../data/Spending";

/**
 * BuiltinTab shows one of Evie's own views as a Workspace tab, named by
 * "builtin" in the tab's tab.json (workspace-tabs decisions, 2026-10-12):
 * the same view, data and buttons as under Data, not a sealed page.
 */
export function BuiltinTab({view}: {view: string}) {
  if (view === "spending") return <div className="flex min-h-0 min-w-0 flex-1 flex-col"><Spending /></div>;
  return <p className="text-muted-text m-auto text-sm">This version of Evie has no built-in view named {view}.</p>;
}
