import { describe, expect, it } from "vitest";
import { applyOpenedSession } from "./useContextSessions";

describe("applyOpenedSession", () => {
  it("commits the selected session and displayed Context Scope together", () => {
    const snapshot = {
      workspaces: [],
      projects: [],
      sessions: [],
      activeSession: { id: "old", title: "", status: "active" as const, createdAt: "", updatedAt: "" },
      activeScope: { kind: "unscoped" as const, displayName: "Unscoped" },
    };
    const opened = {
      session: {
        id: "cairo",
        workspaceId: "workspace-1",
        workspaceRevisionSnapshot: "revision-1",
        title: "",
        status: "active" as const,
        createdAt: "",
        updatedAt: "",
      },
      scope: {
        kind: "workspace" as const,
        displayName: "Cairo's Kitchen",
        workspaceId: "workspace-1",
        workspaceRevision: "revision-1",
      },
    };

    const selected = applyOpenedSession(snapshot, opened);

    expect(selected.activeSession).toEqual(opened.session);
    expect(selected.activeScope).toEqual(opened.scope);
    expect(selected.workspaces).toBe(snapshot.workspaces);
  });
});
