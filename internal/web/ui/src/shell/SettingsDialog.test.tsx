import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ArchivedSessions, SettingsDialog } from "./SettingsDialog";
import { resolveChatFont } from "../ui/chatFont";

describe("Settings", () => {
  it("exposes font, text size, and archives together", () => {
    const html = renderToStaticMarkup(<SettingsDialog busy={false} textSize="large" onTextSize={() => {}} font="serif" onFont={() => {}} onRestore={async () => {}} onClose={() => {}} />);
    expect(html).toContain("Conversation appearance");
    expect(html).toContain("Text size");
    expect(html).toContain("Archived sessions");
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

  it("shows failed session loading inside the popup with a retry action", () => {
    const html = renderToStaticMarkup(<ArchivedSessions busy={false} problem="Sessions are unavailable." onRefresh={() => {}} onRestore={() => {}} />);
    expect(html).toContain('role="alert"');
    expect(html).toContain("Sessions are unavailable.");
    expect(html).toContain("Retry");
    expect(html).not.toContain("Loading sessions…");
    expect(html).not.toContain("No archived sessions.");
  });
});
