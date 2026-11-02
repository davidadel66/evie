import { describe, expect, it } from "vitest";
import type { ContextSessionSnapshot, StoredSession } from "../api/contextSessions";
import { isPlainChat, reusableChat, sessionScopeLabel } from "./plainChats";

const chat = (scope: Partial<StoredSession> = {}): StoredSession => ({id: "s", title: "", status: "active", createdAt: "", updatedAt: "", ...scope});
const snapshot: ContextSessionSnapshot = {
  workspaces: [{id: "w", displayName: "Finance", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: ""}],
  projects: [{id: "p", displayName: "evie", canonicalRoot: "/code/evie", archived: false, createdAt: "", updatedAt: ""}],
  sessions: [],
};

describe("chats with no workspace (ui-chat.spec.md, plain chats amendment)", () => {
  it("are chats with neither a Workspace nor a project", () => {
    expect(isPlainChat(chat())).toBe(true);
    expect(isPlainChat(chat({workspaceId: "w"}))).toBe(false);
    expect(isPlainChat(chat({projectId: "p"}))).toBe(false);
  });

  it("name their scope No workspace, and others their Workspace or project", () => {
    expect(sessionScopeLabel(chat(), snapshot)).toBe("No workspace");
    expect(sessionScopeLabel(chat({workspaceId: "w"}), snapshot)).toBe("Finance");
    expect(sessionScopeLabel(chat({projectId: "p"}), snapshot)).toBe("evie");
    expect(sessionScopeLabel(chat({workspaceId: "gone"}), snapshot)).toBe("Workspace");
  });

  it("reuse the open chat for New chat only while it has no Workspace and nothing in it", () => {
    expect(reusableChat(chat(), 0, false)).toBe(true);
    expect(reusableChat(undefined, 0, false)).toBe(false);
    expect(reusableChat(chat({title: "Weekend plans"}), 0, false)).toBe(false);
    expect(reusableChat(chat(), 2, false)).toBe(false);
    expect(reusableChat(chat(), 0, true)).toBe(false);
    expect(reusableChat(chat({workspaceId: "w"}), 0, false)).toBe(false);
  });
});
