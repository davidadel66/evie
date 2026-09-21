import { describe, expect, it } from "vitest";
import { applyOpenedSession, applySessionArchive } from "./useContextSessions";

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

describe("session archive state", () => {
  const first = { id: "first", title: "Keep history", status: "active" as const, createdAt: "", updatedAt: "" };
  const second = { ...first, id: "second" };
  const snapshot = { workspaces: [], projects: [], sessions: [first, second], activeSession: first, activeScope: { kind: "unscoped" as const, displayName: "Unscoped" } };

  it("moves a selected session into the archive and clears its scope together", () => {
    const closed = { ...first, status: "closed" as const };
    const result = applySessionArchive(snapshot, closed);
    expect(result.sessions).toEqual([second]);
    expect(result.archivedSessions).toEqual([closed]);
    expect(result.activeSession).toBeUndefined();
    expect(result.activeScope).toBeUndefined();
  });

  it("preserves another selection and restores without silently opening a chat", () => {
    const closed = { ...second, status: "closed" as const };
    const archived = applySessionArchive(snapshot, closed);
    expect(archived.activeSession).toEqual(first);
    const restored = applySessionArchive(archived, second);
    expect(restored.sessions).toEqual([first, second]);
    expect(restored.archivedSessions).toEqual([]);
    expect(restored.activeSession).toEqual(first);
    expect(applySessionArchive(restored, second).sessions).toHaveLength(2);
  });
});
