import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { customTabURL } from "../api/workspaceTabs";
import { MoreTabs, WorkspaceHeader } from "./WorkspaceHeader";
import { WorkspaceTabFrame } from "./WorkspaceTabFrame";
import { WorkspaceTabsSettings } from "./WorkspaceTabsSettings";
import { customTabID, customTabKey, fitCustomTabs } from "./workspaceTabs";

describe("fitting a Workspace's own tabs into the header", () => {
  const base = { fixed: 200, more: 50, gap: 20, selected: -1 };
  it("shows every tab when they fit, or before they are measured", () => {
    expect(fitCustomTabs({ ...base, custom: [60, 60], available: 400 })).toEqual([0, 1]);
    expect(fitCustomTabs({ ...base, fixed: 0, more: 0, custom: [0, 0, 0], available: 0 })).toEqual([0, 1, 2]);
  });
  it("keeps the tabs that fit beside More and moves the rest into it", () => {
    // 200 fixed + More 70 leaves 130: one 60 tab plus gap fits, the next does not.
    expect(fitCustomTabs({ ...base, custom: [60, 60, 60], available: 400 })).toEqual([0]);
    expect(fitCustomTabs({ ...base, custom: [60, 60, 60], available: 260 })).toEqual([]);
  });
  it("always shows the selected tab, giving up others to the right for it", () => {
    expect(fitCustomTabs({ ...base, custom: [60, 60, 60], available: 400, selected: 2 })).toEqual([2]);
    expect(fitCustomTabs({ ...base, custom: [60, 60, 60], available: 260, selected: 1 })).toEqual([1]);
  });
  it("keeps a custom tab apart from a built-in one of the same name", () => {
    expect(customTabKey("runs")).toBe("tab:runs");
    expect(customTabID("tab:runs")).toBe("runs");
    expect(customTabID("runs")).toBeUndefined();
  });
});

describe("the Workspace header with its own tabs", () => {
  const custom = [{ id: customTabKey("reports"), label: "Reports" }, { id: customTabKey("budget"), label: "Budget" }];
  it("shows built-ins, then the Workspace's tabs, then Settings", () => {
    const html = renderToStaticMarkup(<WorkspaceHeader tab="runs" waiting={0} customTabs={custom} onTab={() => {}} />);
    const order = [...html.matchAll(/role="tab" id="workspace-tab-([^"]+)"/g)].map((match) => match[1]);
    expect(order).toEqual(["runs", "workflows", "tab:reports", "tab:budget", "settings"]);
    expect(html).not.toContain(">Reload<");
  });
  it("offers Reload while one of its own tabs is selected", () => {
    const html = renderToStaticMarkup(<WorkspaceHeader tab={customTabKey("reports")} waiting={0} customTabs={custom} onTab={() => {}} onReload={() => {}} />);
    expect(html).toMatch(/id="workspace-tab-tab:reports" aria-selected="true"/);
    expect(html).toContain(">Reload<");
  });
  it("puts tabs that do not fit in a closed More menu", () => {
    const html = renderToStaticMarkup(<MoreTabs tabs={custom} onTab={() => {}} />);
    expect(html).toMatch(/aria-haspopup="menu" aria-expanded="false"/);
    expect(html).toContain("More");
    expect(html).not.toContain("Budget");
  });
});

describe("a Workspace tab's page", () => {
  it("is framed sealed: scripts only, no same-origin, at its own folder's URL", () => {
    const html = renderToStaticMarkup(<WorkspaceTabFrame workspaceId="ws 1" tab={{ id: "reports", title: "Reports", order: 0 }} />);
    expect(html).toContain('sandbox="allow-scripts"');
    expect(html).toContain(`src="${customTabURL("ws 1", "reports")}"`);
    expect(customTabURL("ws 1", "reports")).toBe("/workspace-tabs/ws%201/reports/");
    expect(html).toContain('title="Reports"');
  });
});

describe("Settings → Tabs", () => {
  it("explains that a folder comes first", () => {
    expect(renderToStaticMarkup(<WorkspaceTabsSettings />)).toContain("Give this workspace a folder");
  });
  it("lists every tab with whether it shows or what is wrong", () => {
    const html = renderToStaticMarkup(<WorkspaceTabsSettings folder="/code/finance" onRefresh={() => {}} tabs={[
      { id: "reports", title: "Reports", order: 1 }, { id: "broken", title: "broken", order: 0, problem: "index.html is missing" },
    ]} />);
    for (const text of ["Reports", "Shown", "broken", "index.html is missing", "Refresh"]) expect(html).toContain(text);
    expect(renderToStaticMarkup(<WorkspaceTabsSettings folder="/code/finance" tabs={[]} />)).toContain("/code/finance/.evie/tabs/");
    expect(renderToStaticMarkup(<WorkspaceTabsSettings folder="/code/finance" problem="the Workspace folder's .evie/tabs cannot be read" />)).toContain('role="alert"');
  });
});
