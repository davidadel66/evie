import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Workspace } from "../api/contextSessions";
import { WorkspaceHome } from "./Workspaces";

const base: Workspace = {id: "w", displayName: "Code", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: "", defaultPresetId: "standard", allowedPresetIds: ["standard"],
  folder: {path: "/Users/me/code/evie", revision: 1}, explore: {setting: "default", allowed: true}};

function buildSection(workspace: Workspace, busy = false): string {
  const html = renderToStaticMarkup(<WorkspaceHome workspace={workspace} sessions={[]} busy={busy} onNewChat={()=>{}} onResume={()=>{}} />);
  const match = html.match(/<section aria-label="Build workers".*?<\/section>/);
  return match?.[0] ?? "";
}

describe("Workspace build permission", () => {
  it("is off by default even with a folder, and stays changeable during a chat", () => {
    const html = buildSection({...base, build: {setting: "default", allowed: false}}, true);
    expect(html).toContain("Allow build workers");
    expect(html).not.toMatch(/<input[^>]*type="checkbox"[^>]*checked=""/);
    expect(html).not.toMatch(/<input[^>]*type="checkbox"[^>]*disabled/);
    expect(html).toContain("Off by default");
    expect(html).toContain("worktree");
  });
  it("shows the owner's explicit on", () => {
    const html = buildSection({...base, build: {setting: "on", allowed: true}});
    expect(html).toMatch(/<input[^>]*type="checkbox"[^>]*checked=""/);
    expect(html).toContain("Turned on");
  });
  it("is off for a Workspace from before build workers", () => {
    const html = buildSection(base);
    expect(html).toContain("Allow build workers");
    expect(html).not.toMatch(/<input[^>]*type="checkbox"[^>]*checked=""/);
  });
  it("does not offer build from a Research-only parent", () => {
    const html = buildSection({...base, defaultPresetId: "research", allowedPresetIds: ["research"], build: {setting: "default", allowed: false}});
    expect(html).not.toContain("Allow build workers");
    expect(html).toContain("cannot delegate");
  });
});
