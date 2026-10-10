import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Workspace } from "../api/contextSessions";
import type { WorkflowCard, WorkflowDefinition } from "../api/workflows";
import { WorkspaceHome } from "./Workspaces";
import { WorkflowApprovalCard, WorkflowList } from "./WorkspaceWorkflows";

const workspace: Workspace = {id: "w", displayName: "Notes", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: "", defaultPresetId: "standard", allowedPresetIds: ["standard"]};

const hash = "sha256:" + "a".repeat(64);
const authority = "sha256:" + "b".repeat(64);

const active: WorkflowDefinition = {
  workspace_id: "w", name: "notes-digest", status: "active", pending: 1,
  current: {workspace_id: "w", name: "notes-digest", version: 1, proposal_id: "p1", commit: "c".repeat(40), executable_hash: hash, authority_hash: authority,
    authority: [], workers: [], approver: {actor: "owner", surface: "cli"}, approved_at: "2026-10-04T12:00:00Z"},
};

const blocked: WorkflowDefinition = {...active, name: "tips", status: "blocked", pending: 0, blocked: ["Workspace Access revision 3 revoked part of its authority"]};

const card: WorkflowCard = {
  proposal: {id: "p2", workspace_id: "w", name: "notes-digest", commit: "d".repeat(40), executable_hash: "sha256:" + "e".repeat(64), authority_hash: authority,
    simulation: {status: "succeeded", attempts: 4, tokens: 300, previews: [{}], notice: "Effects suppressed"}},
  current: active.current, current_status: "active", expected_version: 1, needs_approval: true,
  diff: {from_hash: hash, to_hash: "sha256:" + "e".repeat(64), executable_changed: true, documentation_changed: false,
    executable: [{section: "Limits", removed: ["max duration 30m"], added: ["max duration 1h"]}],
    authority: ["~ notes.append on conn-notes: max calls per run 1 -> 2"]},
  requirements: {authority: [{capability: "notes.append", connection: "conn-notes", resources: ["notebook:digest"], max_calls_per_run: 2}], authority_hash: authority, workers: [],
    schedule: {cron: "0 7 * * 1-5", timezone: "America/New_York", catch_up: "skip", inputs: []}},
  patch: "",
};

describe("Workspace workflows", () => {
  it("is a section of the Workspace home", () => {
    const html = renderToStaticMarkup(<WorkspaceHome tab="workflows" workspace={workspace} sessions={[]} busy={false} onNewChat={()=>{}} onResume={()=>{}} />);
    expect(html).toContain('aria-label="Workflows"');
    expect(html).toContain("evie workflows propose");
    // Built-in workflows need no "add from template" step (2026-10-04).
    expect(html).not.toContain("--template");
    expect(html).toContain('aria-label="Built-in workflows"');
  });

  it("lists each definition with its status, version and why it is blocked", () => {
    const html = renderToStaticMarkup(<WorkflowList definitions={[active, blocked]} busy={false} onReview={()=>{}} onDeactivate={()=>{}} />);
    expect(html).toContain("notes-digest");
    expect(html).toContain("Active");
    expect(html).toContain("version 1");
    expect(html).toContain("1 proposal waiting");
    expect(html).toContain("Blocked");
    expect(html).toContain("Workspace Access revision 3 revoked");
    expect(html).toContain("Review proposal");
  });

  it("shows an active version's schedule, and that it is not running when the workflow is not active", () => {
    const schedule = {cron: "0 7 * * *", timezone: "America/New_York", catch_up: "each_missed", max_runs: 3, inputs: ["day"]};
    const scheduled: WorkflowDefinition = {...active, current: {...active.current!, schedule}};
    const html = renderToStaticMarkup(<WorkflowList definitions={[scheduled, {...scheduled, name: "tips", status: "blocked"}]} busy={false} onReview={()=>{}} onDeactivate={()=>{}} />);
    expect(html).toContain("Schedule: 0 7 * * * America/New_York, catch-up each_missed");
    expect(html).toContain("(not running: the workflow is not active)");
  });

  it("shows exactly what approval binds and changes", () => {
    const html = renderToStaticMarkup(<WorkflowApprovalCard card={card} busy={false} onApprove={()=>{}} onClose={()=>{}} />);
    expect(html).toContain(card.proposal.executable_hash);
    expect(html).toContain(authority);
    expect(html).toContain("max duration 1h");
    expect(html).toContain("~ notes.append on conn-notes: max calls per run 1 -&gt; 2");
    expect(html).toContain("notes.append on conn-notes: notebook:digest, at most 2 calls per run");
    expect(html).toContain("also activates this schedule");
    expect(html).toContain("Dry run: succeeded");
    expect(html).toMatch(/<button[^>]*>Approve version 2<\/button>/);
  });

  it("cannot approve while approval would be refused, or when nothing needs approval", () => {
    const refused = renderToStaticMarkup(<WorkflowApprovalCard card={{...card, refusals: ["agent steps run research workers, which this Workspace does not allow"]}} busy={false} onApprove={()=>{}} onClose={()=>{}} />);
    expect(refused).toContain("research workers");
    expect(refused).toMatch(/<button[^>]*disabled=""[^>]*>Approve version 2<\/button>/);
    const docs = renderToStaticMarkup(<WorkflowApprovalCard card={{...card, needs_approval: false, proposal: {...card.proposal, documents_version: 1}}} busy={false} onApprove={()=>{}} onClose={()=>{}} />);
    expect(docs).toContain("No approval needed");
    expect(docs).not.toContain("Approve version");
  });
});
