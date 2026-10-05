import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Workspace } from "../api/contextSessions";
import type { WorkflowBuiltin } from "../api/workflowBuiltins";
import { WorkspaceHome } from "./Workspaces";
import { BuiltinWorkflowList } from "./WorkspaceBuiltinWorkflows";

const workspace: Workspace = {id: "w", displayName: "Research", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: "", defaultPresetId: "standard", allowedPresetIds: ["standard", "research"]};

const deepResearch: WorkflowBuiltin = {
  name: "deep-research", description: "Research one question from up to five search angles.", inputs: ["question (string, required)"],
  workers: ["research"], max_tokens: 6_000_000, executable_hash: "sha256:" + "a".repeat(64), version: 1, enabled: true,
  workspace: {workspace_id: "w", source: "builtin", version: 1},
};

describe("Built-in workflows", () => {
  it("is part of the Workspace's Workflows section", () => {
    const html = renderToStaticMarkup(<WorkspaceHome workspace={workspace} sessions={[]} busy={false} onNewChat={()=>{}} onResume={()=>{}} />);
    expect(html).toContain('aria-label="Built-in workflows"');
    expect(html).toContain("without a proposal or approval");
  });

  it("lists each built-in with an on switch, its bounds and what this Workspace runs", () => {
    const html = renderToStaticMarkup(<BuiltinWorkflowList builtins={[deepResearch]} workspaceId="w" busy={false} onToggle={()=>{}} />);
    expect(html).toContain("deep-research");
    expect(html).toContain("Built in");
    expect(html).toMatch(/<input[^>]*type="checkbox"[^>]*role="switch"[^>]*checked=""/);
    expect(html).toContain("On for every Workspace");
    expect(html).toContain("research workers, at most 6,000,000 tokens a run");
    expect(html).toContain("This Workspace runs the built-in.");
    expect(html).toContain("evie workflows customize deep-research --workspace w");
    expect(html).not.toContain("--template");
  });

  it("says why it cannot start here, and when it is off or overridden", () => {
    const blocked = {...deepResearch, workspace: {workspace_id: "w", source: "builtin", blocked: ["agent steps run research workers, which this Workspace does not allow"]}};
    let html = renderToStaticMarkup(<BuiltinWorkflowList builtins={[blocked]} workspaceId="w" busy={false} onToggle={()=>{}} />);
    expect(html).toContain("Cannot start here yet: agent steps run research workers");
    const off = {...deepResearch, enabled: false, control: {seq: 1, name: "deep-research", enabled: false, reason: "trying my own", surface: "web", recorded_at: ""}};
    html = renderToStaticMarkup(<BuiltinWorkflowList builtins={[off]} workspaceId="w" busy={false} onToggle={()=>{}} />);
    expect(html).toContain("Off for every Workspace (trying my own)");
    expect(html).not.toMatch(/checked=""/);
    const overridden = {...deepResearch, workspace: {workspace_id: "w", source: "global", version: 2}};
    html = renderToStaticMarkup(<BuiltinWorkflowList builtins={[overridden]} workspaceId="w" busy={false} onToggle={()=>{}} />);
    expect(html).toContain("This Workspace runs the Global approved copy version 2.");
    html = renderToStaticMarkup(<BuiltinWorkflowList builtins={[]} workspaceId="w" busy={false} onToggle={()=>{}} />);
    expect(html).toContain("No workflows are built into this Evie.");
  });
});
