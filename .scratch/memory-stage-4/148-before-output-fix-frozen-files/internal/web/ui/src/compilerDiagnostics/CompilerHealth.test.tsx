import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { CompilerHealthView } from "./CompilerHealth";
import { CompilerDiagnosticsController, type DiagnosticsState } from "./controller";
import { diagnosticAPI, diagnosticPage, diagnosticState } from "./fixtures.test-support";
import { MemoryReviewTabs } from "../candidateInbox/MemoryReviewTabs";
function render(state: DiagnosticsState = diagnosticState) { return renderToStaticMarkup(<CompilerHealthView state={state} controller={new CompilerDiagnosticsController(diagnosticAPI())}/>); }

describe("compiler health disclosure", () => {
 it("shows failed gaps beside later success with exact counts and separate incomplete timing", () => {
  const html = render();
  for (const text of ["earlier-gap", "later-success", "attempts_exhausted", "Selected new events: 2", "Completed new events: 0", "failed or cancelled gap remains unresolved", "Sequence bounds", "Incomplete", "Unavailable / incomplete", "Queue wait", "Inference", "Validation / resolution", "Database completion", "revision 7", "Next diagnostic page", "Next session page"]) expect(html.toLowerCase()).toContain(text.toLowerCase());
  expect(html).not.toContain("Accept this"); expect(html).not.toContain("Reject this");
 });
 it("labels partial indexing and only renders documented counters", () => {
  const html = render({ ...diagnosticState, page: { ...diagnosticPage, indexing: true, counts: { candidates_unresolved: 3, secret_sql_path: 99 } } });
  expect(html).toContain("these counts are partial"); expect(html).toContain("Unresolved inbox candidates"); expect(html).not.toContain("secret_sql_path");
 });
 it("separates elapsed inbox age, recorded review lineage and active human time", () => {
  const html = render({ ...diagnosticState, view: "candidates", page: { ...diagnosticPage, view: "candidates", candidates: [{ ref: { candidate_id: "candidate", interpretation_revision: 2, review_revision: 0 }, job_id: "job", generation_id: "generation", review_state: "unresolved", published_at_unix_ms: 199000, decided_at_unix_ms: null, edited: true }] } });
  for (const text of ["Elapsed inbox age at snapshot: 1.000 s", "Active measured review time", "unavailable here", "Approval rate is not accuracy", "Interpretation revision: 2", "Edited: Yes"]) expect(html).toContain(text);
 });
 it("does not count a suppressed unresolved extraction as active inbox age", () => {
  const html = render({ ...diagnosticState, view: "candidates", page: { ...diagnosticPage, view: "candidates", candidates: [{ ref: { candidate_id: "repeat", interpretation_revision: 0, review_revision: 0 }, job_id: "job", generation_id: "generation", review_state: "unresolved", published_at_unix_ms: 199000, decided_at_unix_ms: null, edited: false, equivalent_to: "owner-decision" }] } });
  expect(html).toContain("Suppressed repeat; review origin: owner-decision"); expect(html).not.toContain("Elapsed inbox age at snapshot");
 });
 it("renders terminal persistence and response finalization independently, including zero and missing observations", () => {
  const html = render({ ...diagnosticState, view: "foreground", page: { ...diagnosticPage, view: "foreground", foreground: [{ root_id: "root", started_at_unix_ms: 0, terminal_committed_at_unix_ms: 10, terminal_commit_nanos: 0, response_finalized_at_unix_ms: null, response_finalization_nanos: null, outcome: "incomplete" }] } });
  for (const text of ["Terminal event commit", "0.000 ms", "Response finalization", "Unavailable / incomplete", "not request duration or first-token time"]) expect(html).toContain(text);
 });
 it("does not equate selection cursors, activation identities or availability with completed work", () => {
  const activation = render({ ...diagnosticState, view: "activations", page: { ...diagnosticPage, view: "activations", activations: [{ activation_id: "activation", selector: { source_scope: "global", destination: "global" }, generation_id: "retained-generation", revision: 3, after_position: 4, through_position: 8, work_paused: true }] } });
  expect(activation).toContain("do not establish completed coverage"); expect(activation).toContain("does not establish current runtime availability"); expect(activation).toContain("Work: Paused");
  const history = render({ ...diagnosticState, view: "history", page: { ...diagnosticPage, view: "history" } }); expect(history).toContain("discovery, not completed coverage");
  const selection = render({ ...diagnosticState, view: "selection", generation: "retained-generation", page: { ...diagnosticPage, view: "selection", selection: [{ event_id: "old-event", sequence: 1, membership: "outside_selection" }] } }); expect(selection).toContain("Exact generation ID"); expect(selection).toContain("outside_selection");
 });
 it("shows deferred roots and selected units before a job exists", () => {
  const units = render({ ...diagnosticState, view: "selections", page: { ...diagnosticPage, view: "selections", selections: [{ selection_id: "unit", generation_id: "generation", first_sequence: 8, last_sequence: 12, state: "selected_unmaterialized", selected_new_events: 2 }] } });
  expect(units).toContain("selected_unmaterialized"); expect(units).toContain("Job: Not materialized"); expect(units).toContain("Selected new events: 2");
  const roots = render({ ...diagnosticState, view: "live_roots", page: { ...diagnosticPage, view: "live_roots", live_roots: [{ activation_id: "activation", root_id: "deferred-root", first_sequence: 8, last_sequence: 12, state: "deferred_live", reason: "live_turn" }] } });
  expect(roots).toContain("deferred_live"); expect(roots).toContain("pending extension is a separate obligation");
 });
 it("preserves accepted memory as the default and makes health separately available", () => {
  const html = renderToStaticMarkup(<MemoryReviewTabs><p>Existing graph</p></MemoryReviewTabs>); expect(html).toContain("Existing graph"); expect(html).toContain("Compiler health"); expect(html).not.toContain("Scoped operational counts");
 });
});
