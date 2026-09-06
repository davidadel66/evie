import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { WorkspaceHome, Workspaces } from "./Workspaces";

const workspace = {
  id: "workspace-1",
  displayName: "Cairo's Kitchen",
  state: "active" as const,
  currentRevisionId: "revision-1",
  createdAt: "",
  updatedAt: "",
};

const session = {
  id: "session-1",
  workspaceId: workspace.id,
  title: "Dinner prep",
  status: "active" as const,
  createdAt: "",
  updatedAt: "",
};

describe("Workspaces", () => {
  it("keeps workspace pages separate from their conversations", () => {
    const html = renderToStaticMarkup(
      <Workspaces
        snapshot={{ workspaces: [workspace], projects: [], sessions: [session] }}
        busy={false}
        problem={null}
        onRegister={() => undefined}
        onOpenWorkspace={() => undefined}
        onNewWorkspaceChat={() => undefined}
        onNewProjectChat={() => undefined}
        onResume={() => undefined}
        onNewUnscopedChat={() => undefined}
      />,
    );
    expect(html).toContain("Durable places");
    expect(html).toContain("Cairo&#x27;s Kitchen");
    expect(html).toContain("Dinner prep");
    expect(html).toContain("Create workspace");
  });

  it("shows a workspace home with its own chat list", () => {
    const html = renderToStaticMarkup(
      <WorkspaceHome workspace={workspace} sessions={[session]} busy={false} onNewChat={() => undefined} onResume={() => undefined} />,
    );
    expect(html).toContain("Workspace");
    expect(html).toContain("isolated memory");
    expect(html).toContain("Dinner prep");
  });
});
