import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StoredSession, Workspace } from "../api/contextSessions";
import { Sidebar } from "../shell/Sidebar";
import { chatMark, combineMarks, loadReadState, markRead, markText, type ChatMark } from "./chatMarks";

describe("chat markers", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("bolds a chat whose latest turn finished after it was last viewed", () => {
    const read = { baseline: 1_000, read: { seen: 5_000 } };
    expect(chatMark({ sessionId: "seen", finishedAt: 4_000 }, undefined, read, "seen").unread).toBe(false);
    expect(chatMark({ sessionId: "seen", finishedAt: 6_000 }, undefined, read, "seen").unread).toBe(true);
    // A chat never viewed counts from when this browser started keeping track.
    expect(chatMark(undefined, new Date(500).toISOString(), read, "old").unread).toBe(false);
    expect(chatMark(undefined, new Date(2_000).toISOString(), read, "new").unread).toBe(true);
    // A running chat shows its spinner, not bold.
    expect(chatMark({ sessionId: "seen", running: true, finishedAt: 6_000 }, undefined, read, "seen")).toEqual({ running: true, waiting: false, unread: false });
    expect(chatMark({ sessionId: "seen", running: true, waiting: 1 }, undefined, read, "seen").waiting).toBe(true);
  });

  it("starts read state at first use and keeps it in this browser", () => {
    const store = new Map<string, string>();
    vi.stubGlobal("localStorage", { getItem: (key: string) => store.get(key) ?? null, setItem: (key: string, value: string) => store.set(key, value) });
    const fresh = loadReadState(7_000);
    expect(fresh).toEqual({ baseline: 7_000, read: {} });
    const marked = markRead(fresh, "chat", 9_000);
    expect(markRead(marked, "chat", 8_000)).toBe(marked);
    expect(loadReadState(99_000)).toEqual({ baseline: 7_000, read: { chat: 9_000 } });
  });

  it("summarizes hidden chats and names each marker for screen readers", () => {
    const marks: ChatMark[] = [{ running: true, waiting: false, unread: false }, { running: false, waiting: true, unread: true }];
    expect(combineMarks(marks)).toEqual({ running: true, waiting: true, unread: true });
    expect(markText(combineMarks(marks))).toBe("working, waiting for your approval, new reply");
  });

  it("shows a spinner, an approval dot and a bold title in the sidebar", () => {
    const workspace: Workspace = { id: "w", displayName: "Demo", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: "" };
    const session = (id: string, title: string): StoredSession => ({ id, title, workspaceId: "w", status: "active", createdAt: "", updatedAt: "" } as StoredSession);
    const sessions = [session("a", "Running chat"), session("b", "Waiting chat"), session("c", "Unread chat"), session("d", "Open chat")];
    const marks: Record<string, ChatMark> = {
      a: { running: true, waiting: false, unread: false },
      b: { running: true, waiting: true, unread: false },
      c: { running: false, waiting: false, unread: true },
      d: { running: false, waiting: false, unread: true },
    };
    const render = (collapsed: string[]) => {
      localStorage.setItem("evie.sidebarCollapsedWorkspaces", JSON.stringify(collapsed));
      return renderToStaticMarkup(<Sidebar snapshot={{ workspaces: [workspace], projects: [], sessions, archivedSessions: [], activeSession: sessions[3] }} destination="chat" busy={false} marks={marks} mobileOpen={false}
        onSettings={() => {}} onCreateWorkspace={() => {}} onArchive={() => {}} onCloseMobile={() => {}} onData={() => {}} onWorkspace={() => {}} onNewWorkspaceChat={() => {}} onSession={() => {}} />);
    };
    const store = new Map<string, string>();
    vi.stubGlobal("localStorage", { getItem: (key: string) => store.get(key) ?? null, setItem: (key: string, value: string) => store.set(key, value) });
    const open = render([]);
    expect(open.match(/animate-spin/g)).toHaveLength(2);
    expect(open).toContain(", working, waiting for your approval</span>");
    expect(open).toMatch(/font-semibold">Unread chat</);
    // The chat on screen is read whatever its record says.
    expect(open).not.toMatch(/font-semibold">Open chat</);
    const collapsed = render(["w"]);
    expect(collapsed).not.toContain("Running chat");
    expect(collapsed).toContain(", other chats: working, waiting for your approval, new reply");
    expect(collapsed).toMatch(/font-semibold">Demo</);
  });
});
