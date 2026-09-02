import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ContextChooser } from "./ContextChooser";

describe("ContextChooser", () => {
  it("shows explicit Workspace registration, entry, resume, and unscoped choices", () => {
    const html = renderToStaticMarkup(<ContextChooser
      snapshot={{
        workspaces: [{ id: "workspace-1", displayName: "Cairo's Kitchen", state: "active", currentRevisionId: "revision-1", createdAt: "", updatedAt: "" }],
        projects: [],
        sessions: [{ id: "session-1", workspaceId: "workspace-1", workspaceRevisionSnapshot: "revision-1", title: "Dinner prep", status: "active", createdAt: "", updatedAt: "", activityAt: "" }],
      }}
      busy={false}
      problem={null}
      onRegister={() => undefined}
      onSelect={() => undefined}
    />);
    for (const expected of ["Choose a Context Scope", "Cairo&#x27;s Kitchen", "New session", "Dinner prep", "Register Workspace", "New unscoped session"]) {
      expect(html).toContain(expected);
    }
  });

  it("displays the immutable active Context Scope", () => {
    const html = renderToStaticMarkup(<ContextChooser
      snapshot={{ workspaces: [], projects: [], sessions: [], activeScope: { kind: "workspace", displayName: "Cairo's Kitchen", workspaceId: "workspace-1", workspaceRevision: "revision-1" } }}
      busy={false}
      problem={null}
      onRegister={() => undefined}
      onSelect={() => undefined}
    />);
    expect(html).toContain("Context Scope: Workspace — Cairo&#x27;s Kitchen");
    expect(html).toContain("Switch session");
  });
});
