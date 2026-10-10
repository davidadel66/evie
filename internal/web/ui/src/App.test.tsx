import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { WorkbenchBar, type WorkbenchView } from "./App";
import { selectionForScope } from "./shell/scopeSelection";

describe("WorkbenchBar", () => {
  const base = {chatLabel: "Dinner prep", inspectorOpen: true, onOpenNavigation: () => undefined, onChat: () => undefined, onToggleInspector: () => undefined};

  it("names the open chat with one inspector toggle and no tabs", () => {
    const html = renderToStaticMarkup(<WorkbenchBar {...base} view={{id: "chat", kind: "chat"}} />);
    expect(html).toContain("Dinner prep");
    expect(html).toContain('aria-current="page"');
    expect(html).not.toContain('role="tab');
    expect(html).toContain('aria-label="Toggle inspector"');
    expect(html).toContain('aria-pressed="true"');
    expect(html).not.toContain("Focus inspector");
  });

  it("shows another view as a breadcrumb that leads back to the chat", () => {
    const view: WorkbenchView = {id: "workspace:one", kind: "workspace", workspaceId: "one", label: "Cairo's Kitchen"};
    const html = renderToStaticMarkup(<WorkbenchBar {...base} view={view} />);
    expect(html).toMatch(/<button[^>]*title="Back to Dinner prep"[^>]*>Dinner prep<\/button>/);
    expect(html).toMatch(/aria-current="page"[^>]*>Cairo&#x27;s Kitchen</);
    expect(renderToStaticMarkup(<WorkbenchBar {...base} view={{id: "data", kind: "data"}} />)).toMatch(/aria-current="page"[^>]*>Data</);
  });

  it("offers the Inspector toggle only in chat, and the notifications icon on every view", () => {
    const views: WorkbenchView[] = [{id: "chat", kind: "chat"}, {id: "data", kind: "data"}, {id: "workspaces", kind: "workspaces"},
      {id: "workspace:one", kind: "workspace", workspaceId: "one", label: "Cairo's Kitchen", tab: "settings"}];
    for (const view of views) {
      const html = renderToStaticMarkup(<WorkbenchBar {...base} view={view} notifications={<button type="button" aria-label="Notifications" />} />);
      expect(html).toContain('aria-label="Notifications"');
      expect(html.includes('aria-label="Toggle inspector"')).toBe(view.kind === "chat");
    }
  });
});

describe("selectionForScope", () => {
  it("fails closed when a scoped context is missing its durable identity", () => {
    expect(selectionForScope({ kind: "workspace", displayName: "Broken" })).toBeUndefined();
    expect(selectionForScope({ kind: "project", displayName: "Broken" })).toBeUndefined();
  });

  it("preserves exact workspace revision selection", () => {
    expect(selectionForScope({
      kind: "workspace",
      displayName: "Cairo's Kitchen",
      workspaceId: "workspace-1",
      workspaceRevision: "revision-4",
    })).toEqual({ workspaceId: "workspace-1", workspaceRevision: "revision-4" });
  });
});
