import { afterEach, describe, expect, it, vi } from "vitest";
import { listContextSessions, registerWorkspace, selectContextSession } from "./contextSessions";

describe("Context Session API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("registers a Workspace and explicitly selects its rendered revision", async () => {
    const responses = [
      { id: "workspace-1", displayName: "Cairo's Kitchen", currentRevisionId: "revision-1" },
      { session: { id: "session-1", workspaceId: "workspace-1" }, scope: { kind: "workspace", displayName: "Cairo's Kitchen" } },
    ];
    const fetchMock = vi.fn(async () => new Response(JSON.stringify(responses.shift()), {
      status: 200, headers: { "Content-Type": "application/json" },
    }));
    vi.stubGlobal("fetch", fetchMock);

    const workspace = await registerWorkspace("Cairo's Kitchen");
    await selectContextSession({ workspaceId: workspace.id, workspaceRevision: workspace.currentRevisionId });

    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/workspaces/register", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ displayName: "Cairo's Kitchen" }),
    });
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/context-sessions/select", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ workspaceId: "workspace-1", workspaceRevision: "revision-1" }),
    });
  });

  it("lists choices and resumes only the selected session identity", async () => {
    const snapshot = { workspaces: [], projects: [], sessions: [] };
    const fetchMock = vi.fn(async (path: string) => new Response(JSON.stringify(
      path.endsWith("/list") ? snapshot : { session: { id: "session-1" }, scope: { kind: "unscoped", displayName: "Unscoped" } },
    ), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listContextSessions()).resolves.toEqual(snapshot);
    await selectContextSession({ sessionId: "session-1" });
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/context-sessions/select", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ sessionId: "session-1" }),
    });
  });
});
