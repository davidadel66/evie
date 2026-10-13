import { useCallback, useEffect, useState } from "react";
import { workspaceTabsAPI, type CustomTab, type WorkspaceTabsAPI } from "../api/workspaceTabs";
import type { WorkspaceTab } from "./workspaceActivity";

/** A custom tab's place in the Workspace view; the prefix keeps a folder named "runs" apart from Runs. */
export function customTabKey(id: string): WorkspaceTab {
  return `tab:${id}`;
}

export function customTabID(tab: WorkspaceTab | undefined): string | undefined {
  return tab?.startsWith("tab:") ? tab.slice("tab:".length) : undefined;
}

/**
 * A Workspace's own tabs, read when it opens or its folder changes. Tabs
 * with a problem are listed for Settings → Tabs but never shown as tabs.
 */
export function useWorkspaceTabs(workspaceId: string, folder: string | undefined, api: WorkspaceTabsAPI = workspaceTabsAPI) {
  const [tabs, setTabs] = useState<CustomTab[]>();
  const [problem, setProblem] = useState("");
  const reload = useCallback(() => {
    api.list(workspaceId).then(
      (list) => { setTabs(list.tabs); setProblem(""); },
      (error: unknown) => setProblem(error instanceof Error ? error.message : "The Workspace's tabs could not be read."),
    );
  }, [api, workspaceId]);
  useEffect(() => {
    setTabs(undefined);
    setProblem("");
    reload();
  }, [reload, folder]);
  return { tabs, shown: (tabs ?? []).filter((tab) => !tab.problem), problem, reload };
}

/**
 * fitCustomTabs picks which custom tabs show in the header; the rest go in
 * More. Widths are in pixels; fixed is the built-in tabs and Settings with
 * the gaps between them, and each custom tab or the More button adds its
 * width plus one gap. The selected tab always shows. A row not laid out yet
 * (no available width) shows everything.
 */
export function fitCustomTabs({ custom, fixed, more, gap, available, selected }: {
  custom: number[]; fixed: number; more: number; gap: number; available: number; selected: number;
}): number[] {
  const all = custom.map((_, i) => i);
  if (available <= 0 || fixed + custom.reduce((sum, width) => sum + width + gap, 0) <= available) return all;
  let room = available - fixed - (more + gap);
  const visible: number[] = [];
  for (const i of all) {
    if (custom[i] + gap > room) break;
    visible.push(i);
    room -= custom[i] + gap;
  }
  if (selected >= 0 && !visible.includes(selected)) {
    while (visible.length > 0 && room < custom[selected] + gap) room += custom[visible.pop()!] + gap;
    visible.push(selected);
    visible.sort((a, b) => a - b);
  }
  return visible;
}
