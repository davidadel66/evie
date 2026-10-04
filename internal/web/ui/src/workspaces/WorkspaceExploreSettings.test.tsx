import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Workspace } from "../api/contextSessions";
import { WorkspaceHome } from "./Workspaces";
import { exploreSettingFor } from "./exploreSetting";

const base: Workspace = {id: "w", displayName: "Code", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: "", defaultPresetId: "standard", allowedPresetIds: ["standard"]};
const withFolder: Workspace = {...base, folder: {path: "/Users/me/code/evie", revision: 1}, explore: {setting: "default", allowed: true}};
const noFolder: Workspace = {...base, explore: {setting: "default", allowed: false}};

function exploreSection(workspace: Workspace, busy = false): string {
  const html = renderToStaticMarkup(<WorkspaceHome workspace={workspace} sessions={[]} busy={busy} onNewChat={()=>{}} onResume={()=>{}} />);
  const match = html.match(/<section aria-label="Explore workers".*?<\/section>/);
  return match?.[0] ?? "";
}

describe("Workspace explore permission", () => {
  it("is on by default for a Workspace with a folder, and stays changeable during a chat", () => {
    const html = exploreSection(withFolder, true);
    expect(html).toContain("Allow explore workers");
    expect(html).toMatch(/<input[^>]*type="checkbox"[^>]*checked=""/);
    expect(html).not.toMatch(/<input[^>]*type="checkbox"[^>]*disabled/);
    expect(html).toContain("On by default");
    expect(html).toContain("this Workspace&#x27;s folder");
  });
  it("is off by default without a folder and explains the absolute path", () => {
    const html = exploreSection(noFolder);
    expect(html).toContain("Allow explore workers");
    expect(html).not.toMatch(/<input[^>]*type="checkbox"[^>]*checked=""/);
    expect(html).toContain("Off by default");
    expect(html).toContain("absolute path");
  });
  it("shows an explicit choice that differs from the folder default", () => {
    const html = exploreSection({...withFolder, explore: {setting: "off", allowed: false}});
    expect(html).not.toMatch(/<input[^>]*type="checkbox"[^>]*checked=""/);
    expect(html).toContain("Turned off");
    const on = exploreSection({...noFolder, explore: {setting: "on", allowed: true}});
    expect(on).toMatch(/<input[^>]*type="checkbox"[^>]*checked=""/);
    expect(on).toContain("Turned on");
  });
  it("does not offer explore from a Research-only parent", () => {
    const html = exploreSection({...withFolder, defaultPresetId: "research", allowedPresetIds: ["research"], explore: {setting: "default", allowed: false}});
    expect(html).not.toContain("Allow explore workers");
    expect(html).toContain("cannot delegate");
  });
  it("turns on to the folder default, or explicitly without a folder, and off explicitly", () => {
    expect(exploreSettingFor(withFolder, true)).toBe("default");
    expect(exploreSettingFor(noFolder, true)).toBe("on");
    expect(exploreSettingFor(withFolder, false)).toBe("off");
    expect(exploreSettingFor(noFolder, false)).toBe("off");
  });
});
