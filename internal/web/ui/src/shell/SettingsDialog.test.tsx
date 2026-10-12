import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ArchivedSessions, ArchivedWorkspaces, SettingsDialog } from "./SettingsDialog";
import { DeleteWorkspaceDialog } from "./DeleteWorkspaceDialog";
import { resolveChatFont } from "../ui/chatFont";

describe("Settings", () => {
  it("exposes font, text size, and archives together", () => {
    const html = renderToStaticMarkup(<SettingsDialog busy={false} textSize="large" onTextSize={() => {}} font="serif" onFont={() => {}} onRestore={async () => {}} onClose={() => {}} />);
    expect(html).toContain("Conversation appearance");
    expect(html).toContain("Text size");
    expect(html).toContain("Archived sessions");
    expect(html).toContain("Archived workspaces");
    expect(html).toContain('value="serif" selected=""');
    expect(resolveChatFont("unexpected")).toBe("default");
    expect(html).toContain("…");
  });

  it("shows archived conversation titles with workspace context and a restore action", () => {
    const html = renderToStaticMarkup(<ArchivedSessions busy snapshot={{ workspaces: [{ id: "one", displayName: "Interview prep", currentRevisionId: "r", state: "active", createdAt: "", updatedAt: "" }], projects: [], sessions: [], archivedSessions: [{ id: "s", workspaceId: "one", title: "Practice questions", status: "closed", createdAt: "", updatedAt: "" }] }} onRestore={() => {}} />);
    expect(html).toContain("Interview prep");
    expect(html).toContain("Practice questions");
    expect(html).toMatch(/<button[^>]*disabled=""[^>]*aria-label="Restore Practice questions"/);
  });

  it("lists archived workspaces with the conversations they keep and a restore action", () => {
    const workspace = { currentRevisionId: "r", createdAt: "", updatedAt: "" };
    const snapshot = {
      workspaces: [{ ...workspace, id: "one", displayName: "Interview prep", state: "active" as const }, { ...workspace, id: "two", displayName: "evie-it-merge", state: "archived" as const }],
      projects: [], archivedSessions: [],
      sessions: [{ id: "s", workspaceId: "two", title: "Delegate two builds", status: "active" as const, createdAt: "", updatedAt: "" }],
    };
    const html = renderToStaticMarkup(<ArchivedWorkspaces busy={false} snapshot={snapshot} onRestore={() => {}} />);
    expect(html).toContain("evie-it-merge");
    expect(html).toContain("1 conversation kept");
    expect(html).not.toContain("Interview prep");
    expect(html).toMatch(/<button[^>]*aria-label="Restore evie-it-merge"/);
    expect(renderToStaticMarkup(<ArchivedWorkspaces busy={false} snapshot={{ ...snapshot, workspaces: [snapshot.workspaces[0]] }} onRestore={() => {}} />)).toContain("No archived workspaces.");
  });

  it("asks before deleting a workspace and says what is kept", () => {
    const html = renderToStaticMarkup(<DeleteWorkspaceDialog workspace={{ id: "two", displayName: "evie-it-merge", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: "" }} onDelete={async () => {}} onClose={() => {}} />);
    for (const text of ["Delete evie-it-merge?", "no new chats or workflow runs can start in it", "Its conversations and memories are kept", "Settings → Archived workspaces", "Cancel", "Delete workspace"]) expect(html).toContain(text);
  });

  it("shows failed session loading inside the popup with a retry action", () => {
    const html = renderToStaticMarkup(<ArchivedSessions busy={false} problem="Sessions are unavailable." onRefresh={() => {}} onRestore={() => {}} />);
    expect(html).toContain('role="alert"');
    expect(html).toContain("Sessions are unavailable.");
    expect(html).toContain("Retry");
    expect(html).not.toContain("Loading sessions…");
    expect(html).not.toContain("No archived sessions.");
  });
});
