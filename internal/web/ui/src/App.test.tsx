import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { WorkbenchBar, type WorkbenchView } from "./App";
import { selectionForScope } from "./shell/scopeSelection";

describe("WorkbenchBar", () => {
  it("renders open work as semantic tabs with inspector controls", () => {
    const views: WorkbenchView[] = [
      { id: "chat", kind: "chat" },
      { id: "data", kind: "data" },
      { id: "workspace:one", kind: "workspace", workspaceId: "one", label: "Cairo's Kitchen" },
    ];
    const html = renderToStaticMarkup(
      <WorkbenchBar
        views={views}
        activeViewId="workspace:one"
        chatLabel="Dinner prep"
        inspectorOpen
        inspectorFocused={false}
        onOpenNavigation={() => undefined}
        onActivate={() => undefined}
        onClose={() => undefined}
        onToggleInspector={() => undefined}
        onToggleInspectorFocus={() => undefined}
      />,
    );
    for (const label of ["Dinner prep", "Data", "Cairo&#x27;s Kitchen"]) expect(html).toContain(label);
    expect(html).toContain('role="tablist"');
    expect(html).toContain('aria-selected="true"');
    expect(html).toContain('aria-label="Toggle inspector"');
    expect(html).toContain('aria-label="Focus inspector"');
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
