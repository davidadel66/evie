import { afterEach, describe, expect, it, vi } from "vitest";
import { streamChat } from "./stream";
import {
  listContextSessions,
  registerWorkspace,
  selectContextSession,
} from "./contextSessions";

describe("Workspace frontend path", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("registers, explicitly enters, converses, and resumes one Workspace session", async () => {
    const paths: string[] = [];
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const path = String(input);
      paths.push(path);
      if (path === "/api/workspaces/register") {
        return jsonResponse({
          id: "workspace-1",
          displayName: "Cairo's Kitchen",
          currentRevisionId: "revision-1",
        });
      }
      if (path === "/api/context-sessions/list") {
        return jsonResponse({
          workspaces: [],
          projects: [],
          sessions: [{ id: "session-1", workspaceId: "workspace-1" }],
        });
      }
      if (path === "/api/chat") {
        return new Response(
          'event: delta\ndata: {"text":"inside Cairo"}\n\n' +
            'event: turn_done\ndata: {}\n\n',
          { status: 200, headers: { "Content-Type": "text/event-stream" } },
        );
      }
      return jsonResponse({
        session: { id: "session-1", workspaceId: "workspace-1" },
        scope: {
          kind: "workspace",
          displayName: "Cairo's Kitchen",
          workspaceId: "workspace-1",
          workspaceRevision: "revision-1",
        },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const workspace = await registerWorkspace("Cairo's Kitchen");
    const entered = await selectContextSession({
      workspaceId: workspace.id,
      workspaceRevision: workspace.currentRevisionId,
    });
    const events: string[] = [];
    await streamChat("prepare dinner", (event) => events.push(event.type));
    const snapshot = await listContextSessions();
    const resumed = await selectContextSession({ sessionId: snapshot.sessions[0].id });

    expect(entered.scope).toEqual(resumed.scope);
    expect(events).toEqual(["delta", "turn_done"]);
    expect(paths).toEqual([
      "/api/workspaces/register",
      "/api/context-sessions/select",
      "/api/chat",
      "/api/context-sessions/list",
      "/api/context-sessions/select",
    ]);
  });
});

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}
