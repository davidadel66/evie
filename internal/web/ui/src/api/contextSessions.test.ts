import { afterEach, describe, expect, it, vi } from "vitest";
import { archiveSession, restoreSession, chooseWorkspaceFolder, listContextSessions, registerWorkspace, selectContextSession } from "./contextSessions";

describe("Context Session API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("opens the native folder chooser with a cancellable empty request", async () => {
    const fetchMock = vi.fn(async () => Response.json({ path: "/Users/david/a folder ", cancelled: false }));
    vi.stubGlobal("fetch", fetchMock);
    const controller = new AbortController();
    await expect(chooseWorkspaceFolder(controller.signal)).resolves.toEqual({ path: "/Users/david/a folder ", cancelled: false });
    expect(fetchMock).toHaveBeenCalledExactlyOnceWith("/api/workspaces/choose-folder", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: "{}", signal: controller.signal,
    });
  });

  it("treats picker cancellation as a result and reports unavailable pickers", async () => {
    const fetchMock = vi.fn(async () => Response.json({ path: "", cancelled: true }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(chooseWorkspaceFolder()).resolves.toEqual({ path: "", cancelled: true });
    fetchMock.mockImplementation(async () => Response.json({ error: "The folder picker is available on macOS." }, { status: 501 }));
    await expect(chooseWorkspaceFolder()).rejects.toThrow("available on macOS");
  });

  it("registers a Workspace and explicitly selects its rendered revision", async () => {
    const responses = [
      { id: "workspace-1", displayName: "Cairo's Kitchen", currentRevisionId: "revision-1" },
      { session: { id: "session-1", workspaceId: "workspace-1" }, scope: { kind: "workspace", displayName: "Cairo's Kitchen" } },
    ];
    const fetchMock = vi.fn(async () => new Response(JSON.stringify(responses.shift()), {
      status: 200, headers: { "Content-Type": "application/json" },
    }));
    vi.stubGlobal("fetch", fetchMock);

    const workspace = await registerWorkspace({ displayName: "Cairo's Kitchen" });
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

  it("creates with chosen access and folder options without opening a conversation", async () => {
    const options = { displayName: "Research", presetId: "research", folderPath: "/tmp/research", createFolder: true };
    const fetchMock = vi.fn(async (_path: string, _init: RequestInit) => Response.json({ id: "workspace-2", ...options }));
    vi.stubGlobal("fetch", fetchMock);
    await registerWorkspace(options);
    expect(fetchMock).toHaveBeenCalledExactlyOnceWith("/api/workspaces/register", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(options),
    });
  });

  it("archives and restores only the requested session and surfaces failure", async () => {
    const fetchMock = vi.fn(async (_path: string, _init: RequestInit) => Response.json({ session: { id: "one", status: "closed" } }));
    vi.stubGlobal("fetch", fetchMock);
    await archiveSession("one");
    await restoreSession("one");
    expect(fetchMock.mock.calls.map(call => call[0])).toEqual(["/api/context-sessions/archive", "/api/context-sessions/restore"]);
    for (const call of fetchMock.mock.calls) expect(call[1].body).toBe(JSON.stringify({ sessionId: "one" }));
    fetchMock.mockImplementation(async () => Response.json({ error: "Finish the current turn" }, { status: 409 }));
    await expect(archiveSession("one")).rejects.toThrow("Finish the current turn");
  });
});
