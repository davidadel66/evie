import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Sidebar } from "./Sidebar";
import type { StoredSession, Workspace } from "../api/contextSessions";

const workspace: Workspace = {id: "one", displayName: "Cairo", state: "active", currentRevisionId: "revision-2", createdAt: "", updatedAt: ""};
function render(workspaces: Workspace[], busy = false, collapsed = false, waiting: Record<string, number> = {}, onDeleteWorkspace?: (workspace: Workspace) => void) {
  return renderToStaticMarkup(<Sidebar snapshot={{workspaces, projects: [], sessions: [], archivedSessions: []}} destination="chat" busy={busy} collapsed={collapsed} waiting={waiting} mobileOpen={false} onSettings={() => {}} onCreateWorkspace={() => {}} onArchive={() => {}} onDeleteWorkspace={onDeleteWorkspace} onCloseMobile={() => {}} onData={() => {}} onSystem={() => {}} onWorkspace={() => {}} onNewWorkspaceChat={() => {}} onNewChat={() => {}} onSession={() => {}} />);
}

describe("workspace session shortcuts", () => {
  it("gives each workspace an independent new-session button", () => {
    const html = render([workspace, {...workspace, id: "two", displayName: "Personal"}]);
    expect(html).toContain('aria-label="New session in Cairo"');
    expect(html).toContain('aria-label="New session in Personal"');
    expect(html).toContain('aria-label="Create workspace"');
    expect(html).toContain("Settings");
    expect(html).toContain("Data");
    expect(html.match(/>Workspaces</g)).toHaveLength(1);
    expect(html).not.toContain("No scope selected");
    expect(html).not.toContain("Choose a workspace to begin");
    expect(html).not.toMatch(/<button[^>]*>(?:(?!<\/button>)[\s\S])*<button/);
    expect(html).not.toContain('disabled=""');
  });

  it("badges a workspace with the runs waiting on the owner", () => {
    const html = render([workspace, {...workspace, id: "two", displayName: "Personal"}], false, false, {one: 2});
    expect(html).toMatch(/Cairo<\/span><span aria-hidden="true" title="Waiting on you"[^>]*>2<\/span><span class="sr-only">, 2 runs wait on you<\/span>/);
    expect(html.match(/Waiting on you/g)).toHaveLength(1);
  });

  it("disables session creation while busy or when a workspace is unavailable", () => {
    for (const [entry, busy] of [[workspace, true], [{...workspace, currentRevisionId: ""}, false]] as const) {
      const html = render([entry], busy);
      expect(html).toMatch(/<button[^>]*aria-label="New session in Cairo"[^>]*disabled=""/);
    }
  });

  it("offers delete on each workspace and leaves a deleted (archived) one out", () => {
    const html = render([workspace, {...workspace, id: "two", displayName: "Old project", state: "archived"}], false, false, {}, () => {});
    expect(html).toMatch(/<button[^>]*aria-label="Delete Cairo"[^>]*title="Delete workspace"/);
    expect(html).not.toContain("Old project");
    expect(render([{...workspace, state: "archived"}])).toContain("Create your first workspace");
    expect(render([workspace])).not.toContain("Delete Cairo");
  });

  it("keeps a collapse control in desktop navigation", () => {
    const html = render([workspace]);
    const button = html.match(/<button[^>]*aria-label="Close navigation"[^>]*>/)?.[0];
    expect(button).toBeDefined();
    expect(button).not.toContain("md:hidden");
    expect(render([workspace], false, true)).toMatch(/<aside[^>]*md:hidden/);
  });

  it("starts at the smaller default width with a keyboard-resizable edge", () => {
    const html = render([workspace]);
    expect(html).toContain("width:240px");
    expect(html).toMatch(/role="separator" aria-label="Resize sidebar" aria-orientation="vertical" aria-valuenow="240" aria-valuemin="200" aria-valuemax="360" tabindex="0"/);
  });
});

describe("destination views", () => {
  const renderAt = (destination: "chat" | "system") => renderToStaticMarkup(<Sidebar snapshot={{workspaces: [workspace], projects: [], sessions: [], archivedSessions: []}} destination={destination} busy={false} mobileOpen={false}
    onSettings={() => {}} onCreateWorkspace={() => {}} onArchive={() => {}} onCloseMobile={() => {}} onData={() => {}} onSystem={() => {}} onWorkspace={() => {}} onNewWorkspaceChat={() => {}} onNewChat={() => {}} onSession={() => {}} />);

  it("lists System directly under Data, above the Workspaces", () => {
    const html = renderAt("chat");
    const data = html.indexOf(">Data<"), system = html.indexOf(">System<"), workspaces = html.indexOf(">Workspaces<");
    expect(data).toBeGreaterThan(-1);
    expect(system).toBeGreaterThan(data);
    expect(workspaces).toBeGreaterThan(system);
    expect(html.slice(data, system).match(/<button/g)).toHaveLength(1);
  });

  it("marks System as the current page only while it is open", () => {
    const current = /<button[^>]*aria-current="page"[^>]*>(?:(?!<\/button>)[\s\S])*>System</;
    expect(renderAt("system")).toMatch(current);
    expect(renderAt("chat")).not.toMatch(current);
  });
});

describe("collapsible workspaces", () => {
  const personal: Workspace = {...workspace, id: "two", displayName: "Personal"};
  const chat = (id: string, workspaceId: string, title: string): StoredSession => ({id, workspaceId, title, status: "active", createdAt: "", updatedAt: ""});
  const sessions = [chat("a", "one", "Cairo menu"), chat("b", "two", "Personal budget")];
  const renderWithChats = (waiting: Record<string, number> = {}) => renderToStaticMarkup(<Sidebar snapshot={{workspaces: [workspace, personal], projects: [], sessions, archivedSessions: []}} destination="chat" busy={false} waiting={waiting} mobileOpen={false}
    onSettings={() => {}} onCreateWorkspace={() => {}} onArchive={() => {}} onCloseMobile={() => {}} onData={() => {}} onSystem={() => {}} onWorkspace={() => {}} onNewWorkspaceChat={() => {}} onNewChat={() => {}} onSession={() => {}} />);
  afterEach(() => { vi.unstubAllGlobals(); });

  it("starts every workspace expanded with its chats under a labelled toggle", () => {
    const html = renderWithChats();
    expect(html).toMatch(/<button[^>]*aria-expanded="true" aria-controls="workspace-sessions-one" aria-label="Collapse Cairo"/);
    expect(html).toMatch(/<button[^>]*aria-expanded="true" aria-controls="workspace-sessions-two" aria-label="Collapse Personal"/);
    expect(html).toContain('id="workspace-sessions-one"');
    expect(html).toContain("Cairo menu");
    expect(html).toContain("Personal budget");
  });

  it("hides a collapsed workspace's chats, remembered per browser, and keeps its waiting badge", () => {
    vi.stubGlobal("localStorage", { getItem: (key: string) => key === "evie.sidebarCollapsedWorkspaces" ? JSON.stringify(["one"]) : null, setItem: () => {} });
    const html = renderWithChats({one: 2});
    expect(html).toMatch(/<button[^>]*aria-expanded="false" aria-controls="workspace-sessions-one" aria-label="Expand Cairo"/);
    expect(html).not.toContain("Cairo menu");
    expect(html).not.toContain('id="workspace-sessions-one"');
    expect(html).toContain("Personal budget");
    expect(html).toContain("2 runs wait on you");
  });

  it("falls back to expanded when browser storage is unreadable", () => {
    vi.stubGlobal("localStorage", { getItem: () => { throw new Error("blocked"); }, setItem: () => { throw new Error("blocked"); } });
    expect(renderWithChats()).toContain("Cairo menu");
    vi.stubGlobal("localStorage", { getItem: () => "{not json", setItem: () => {} });
    expect(renderWithChats()).toContain("Cairo menu");
  });
});

// ui-chat.spec.md, plain chats amendment (2026-11-02): a chat needs no
// Workspace, and chats without one are listed apart from Workspace chats.
describe("chats without a workspace", () => {
  const chat = (id: string, title: string, scope: Partial<StoredSession> = {}): StoredSession => ({id, title, status: "active", createdAt: "", updatedAt: "", ...scope});
  const renderChats = (sessions: StoredSession[], busy = false) => renderToStaticMarkup(<Sidebar snapshot={{workspaces: [workspace], projects: [], sessions, archivedSessions: []}} destination="chat" busy={busy} mobileOpen={false}
    onSettings={() => {}} onCreateWorkspace={() => {}} onArchive={() => {}} onCloseMobile={() => {}} onData={() => {}} onSystem={() => {}} onWorkspace={() => {}} onNewWorkspaceChat={() => {}} onNewChat={() => {}} onSession={() => {}} />);

  it("starts a chat with no workspace from the top of the navigation", () => {
    const html = renderChats([]);
    const newChat = html.indexOf(">New chat<");
    expect(newChat).toBeGreaterThan(-1);
    expect(newChat).toBeLessThan(html.indexOf(">Data<"));
    expect(html).not.toMatch(/<button[^>]*disabled=""[^>]*>(?:(?!<\/button>)[\s\S])*>New chat</);
    expect(renderChats([], true)).toMatch(/<button[^>]*disabled=""[^>]*>(?:(?!<\/button>)[\s\S])*>New chat</);
  });

  it("lists chats without a workspace under Chats, below the Workspaces", () => {
    const html = renderChats([
      chat("w", "Cairo menu", {workspaceId: "one"}),
      chat("p", "Weekend plans"),
      chat("q", "", {}),
      chat("f", "Repo notes", {projectId: "project-1"}),
      chat("c", "Closed chat", {status: "closed"}),
    ]);
    const chats = html.indexOf(">Chats<");
    expect(chats).toBeGreaterThan(html.indexOf(">Workspaces<"));
    expect(html.indexOf("Cairo menu")).toBeLessThan(chats);
    expect(html.indexOf("Weekend plans")).toBeGreaterThan(chats);
    expect(html.indexOf("Untitled session")).toBeGreaterThan(chats);
    expect(html).toContain('aria-label="Archive Weekend plans"');
    expect(html).not.toContain("Repo notes");
    expect(html).not.toContain("Closed chat");
  });

  it("marks a chat under Chats as running, waiting or unread, as in a Workspace", () => {
    const html = renderToStaticMarkup(<Sidebar snapshot={{workspaces: [], projects: [], sessions: [chat("p", "Weekend plans")], archivedSessions: []}} destination="chat" busy={false} mobileOpen={false}
      marks={{p: {running: true, waiting: true, unread: true}}}
      onSettings={() => {}} onCreateWorkspace={() => {}} onArchive={() => {}} onCloseMobile={() => {}} onData={() => {}} onSystem={() => {}} onWorkspace={() => {}} onNewWorkspaceChat={() => {}} onNewChat={() => {}} onSession={() => {}} />);
    const row = html.slice(html.indexOf(">Chats<"));
    expect(row).toContain("animate-spin");
    expect(row).toContain('title="Waiting for your approval"');
    expect(row).toMatch(/font-semibold">Weekend plans</);
  });

  it("leaves Chats out until there is one", () => {
    expect(renderChats([chat("w", "Cairo menu", {workspaceId: "one"})])).not.toContain(">Chats<");
  });
});
