import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Workspace } from "../api/contextSessions";
import { WorkspaceHome } from "./Workspaces";

const base: Workspace = {id: "w", displayName: "Code", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: "", defaultPresetId: "standard", allowedPresetIds: ["standard"],
  folder: {path: "/Users/me/code/evie", revision: 1}, explore: {setting: "default", allowed: true}, build: {setting: "on", allowed: true}};

function verifySection(workspace: Workspace): string {
  const html = renderToStaticMarkup(<WorkspaceHome tab="settings" workspace={workspace} sessions={[]} busy={false} onNewChat={()=>{}} onResume={()=>{}} />);
  const match = html.match(/<section aria-label="Merge checks".*?<\/section>/);
  return match?.[0] ?? "";
}

describe("Workspace merge checks", () => {
  it("says merges cannot be proposed without a verify command", () => {
    const html = verifySection(base);
    expect(html).toContain("Verify command");
    expect(html).toContain("Setup command (optional, runs with network)");
    expect(html).toContain("No verify command is set");
    expect(html).toContain("Suggest from repository instructions");
    expect(html).toContain("approval every time");
  });
  it("shows the owner's saved commands", () => {
    const html = verifySection({...base, verify: {setup: "npm ci", verify: "./scripts/verify-change.sh", revision: 2}});
    expect(html).toContain('value="./scripts/verify-change.sh"');
    expect(html).toContain('value="npm ci"');
    expect(html).not.toContain("No verify command is set");
  });
  it("cannot suggest without a folder or change an archived Workspace", () => {
    expect(verifySection({...base, folder: undefined})).toMatch(/<button[^>]*disabled=""[^>]*>Suggest from repository instructions/);
    expect(verifySection({...base, state: "archived"})).toMatch(/<input[^>]*disabled=""/);
  });
});
