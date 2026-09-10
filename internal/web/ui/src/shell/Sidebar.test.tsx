import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Sidebar } from "./Sidebar";
import type { Workspace } from "../api/contextSessions";

const workspace: Workspace = {id: "one", displayName: "Cairo", state: "active", currentRevisionId: "revision-2", createdAt: "", updatedAt: ""};
function render(workspaces: Workspace[], busy = false, collapsed = false) {
  return renderToStaticMarkup(<Sidebar snapshot={{workspaces, projects: [], sessions: []}} destination="chat" busy={busy} collapsed={collapsed} mobileOpen={false} textSize="default" onTextSize={() => {}} onCloseMobile={() => {}} onNewChat={() => {}} onData={() => {}} onWorkspaces={() => {}} onWorkspace={() => {}} onNewWorkspaceChat={() => {}} onSession={() => {}} />);
}

describe("workspace session shortcuts", () => {
  it("gives each workspace an independent new-session button", () => {
    const html = render([workspace, {...workspace, id: "two", displayName: "Personal"}]);
    expect(html).toContain('aria-label="New session in Cairo"');
    expect(html).toContain('aria-label="New session in Personal"');
    expect(html).toContain('aria-label="Create workspace"');
    expect(html).not.toMatch(/<button[^>]*>(?:(?!<\/button>)[\s\S])*<button/);
    expect(html).not.toContain('disabled=""');
  });

  it("disables session creation while busy or when a workspace is unavailable", () => {
    for (const [entry, busy] of [[workspace, true], [{...workspace, state: "archived"}, false], [{...workspace, currentRevisionId: ""}, false]] as const) {
      const html = render([entry], busy);
      expect(html).toMatch(/<button[^>]*aria-label="New session in Cairo"[^>]*disabled=""/);
    }
  });

  it("keeps a collapse control in desktop navigation", () => {
    const html = render([workspace]);
    const button = html.match(/<button[^>]*aria-label="Close navigation"[^>]*>/)?.[0];
    expect(button).toBeDefined();
    expect(button).not.toContain("md:hidden");
    expect(render([workspace], false, true)).toMatch(/<aside[^>]*md:hidden/);
  });
});
