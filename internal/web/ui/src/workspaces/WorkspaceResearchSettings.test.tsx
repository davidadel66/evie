import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { WorkspaceHome } from "./Workspaces";

const workspace = {id: "w", displayName: "Study", state: "active" as const, currentRevisionId: "r", createdAt: "", updatedAt: "", defaultPresetId: "standard", allowedPresetIds: ["standard", "research"]};

describe("Workspace research permission", () => {
  it("keeps revocation available while a chat is running", () => {
    const html = renderToStaticMarkup(<WorkspaceHome workspace={workspace} sessions={[]} busy onNewChat={()=>{}} onResume={()=>{}} />);
    expect(html).toContain("Allow research delegation");
    expect(html).toMatch(/<input[^>]*type="checkbox"[^>]*checked=""/);
    expect(html).not.toMatch(/<input[^>]*type="checkbox"[^>]*disabled/);
    expect(html).toContain("new chats");
    expect(html).toContain("stops active research");
  });
  it("does not offer delegation from a Research-only parent", () => {
    const html = renderToStaticMarkup(<WorkspaceHome workspace={{...workspace, defaultPresetId: "research", allowedPresetIds: ["research"]}} sessions={[]} busy={false} onNewChat={()=>{}} onResume={()=>{}} />);
    expect(html).not.toContain("Allow research delegation");
    expect(html).toContain("cannot delegate");
  });
});
