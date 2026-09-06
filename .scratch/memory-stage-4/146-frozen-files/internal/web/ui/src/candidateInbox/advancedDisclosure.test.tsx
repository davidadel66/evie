import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ReviewEffects, EditLineage, IdentityLineage, CorrectionLineage } from "./ReviewDisclosure";
import { AdvancedReviewForms } from "./AdvancedReviewForms";
import { BatchReview } from "./BatchReview";
import { CandidateInboxController, type InboxState } from "./controller";
import { batchPreview, batchResult, chosenCandidate, correctionPreview, identityCandidate, identityOptions, identityPreview, temporalOptions } from "./advanced.test-support";
import { candidate, fakeAPI, preview } from "./fixtures.test-support";
import type { EditRevision } from "../api/candidateReview";
const state: InboxState = { scope: "global", busy: false, scopesBusy: false, problem: "", notice: "", reason: "" };
function contains(html: string, texts: string[]) { for (const text of texts) expect(html).toContain(text); }
function controller() { return new CandidateInboxController(fakeAPI()); }

describe("complete advanced owner disclosure", () => {
 it("offers explicit edit, identity and correction history kinds within the shared revision bounds", () => {
  const detail = { ...candidate, ref: { ...candidate.ref, interpretation_revision: 3, review_revision: 3 } };
  const html = renderToStaticMarkup(<AdvancedReviewForms state={{ ...state, detail }} controller={controller()}/>);
  contains(html, ["Inspect interpretation history", "share one revision sequence", 'value="edit"', 'value="identity"', 'value="correction"', "Owner edit", "Identity choice", "Correction choice", 'max="3"', 'step="1"', "Load immutable history revision"]);
  expect(html).not.toContain("Inspect earlier edit revision");
 });
 it("renders readable immutable identity choices, earlier alternatives, parent and audit", () => {
  const identity = { ...chosenCandidate.identity!, choices: { ...chosenCandidate.identity!.choices, subject: { create: false, entity_id: "maya-2" } } };
  contains(renderToStaticMarkup(<IdentityLineage identity={identity}/>), ["Recorded identity choice", "Interpretation 1, from revision 0", "review revision 1", "identity-audit", "Reuse Entity maya-2", "Create the sourced global Predicate definition", "maya-1", "maya-2", "Maya 1", "Maya 2", "Designer", "Physician", "Chosen", "not new factual evidence or approval", "Complete immutable identity revision"]);
 });
 it("renders a historical correction without implying a current lifecycle transition or invented time", () => {
  contains(renderToStaticMarkup(<CorrectionLineage correction={correctionPreview.effect!.correction!.revision}/>), ["Recorded correction choice", "temporal-audit", "Chosen earlier claim: old-tea", "earlier statement was wrong", "effective time: Unknown", "Lifecycle at this revision: active", "Offered meanings: error, changed", "Complete immutable correction revision"]);
 });
 it("never displays a historical result under a different selected kind or revision", () => {
  const identity = { ...chosenCandidate.identity!, audit_id: "historical-protected-audit" };
  const detail = { ...candidate, ref: { ...candidate.ref, interpretation_revision: 3, review_revision: 3 } };
  const html = renderToStaticMarkup(<AdvancedReviewForms state={{ ...state, detail, history: { kind: "identity", value: identity } }} controller={controller()}/>);
  expect(html).not.toContain("historical-protected-audit");
 });
 it("renders distinct same-name alternatives, Alias/context and explicit new global Predicate choice without preselection", () => {
  const html = renderToStaticMarkup(<AdvancedReviewForms state={{ ...state, detail: identityCandidate, identityOptions }} controller={controller()}/>);
  contains(html,["maya-1","maya-2","Maya 1","Maya 2","Designer","Physician","Create distinct Entity","new global Predicate","Enjoys","enjoys","cardinality many","Record these identity choices"]); expect(html).not.toContain('checked=""');
 });
 it("shows chosen new Entity, Predicate and Alias effects before acceptance", () => {
  contains(renderToStaticMarkup(<ReviewEffects preview={identityPreview}/>),["Create distinct Entity","new-maya","Create global Predicate definition","new-enjoys","Create Alias","alias-maya","Two people share this name","not authority","owner_statement","none","Complete canonical disclosure"]);
 });
 it("discloses error versus changed, old lifecycle/polarity and before/after temporal bounds", () => {
  contains(renderToStaticMarkup(<ReviewEffects preview={correctionPreview}/>),["Earlier statement was an error","old-tea","affirmed tea","active","superseded","Effective time: Unknown","Old validity before","Old validity after","Replacement validity"]);
  const form = renderToStaticMarkup(<AdvancedReviewForms state={{ ...state, detail: { ...candidate, candidate: { ...candidate.candidate, proposal: { ...candidate.candidate.proposal, temporal: { meaning: "assertion", correction: { modes: ["error","changed"], effective_time: null } } } } }, temporalOptions }} controller={controller()}/>);
  contains(form,["earlier statement was wrong","real-world state changed"]); expect(form).not.toContain('checked=""');
 });
 it("preserves original contracted clock authority and an unzoned display", () => {
  const source = { ...candidate.candidate.support![0], evidence: "2026-09-05 10:30:00", authority: "tool_observation", actor: "tool", source_type: "tool_succeeded", observation: { contract: "local-clock-display-v1", root_id: "root-clock", execution_id: "execution-clock", call_id: "call-clock", ancestry_sha256: "ancestry-hash" } };
  const clock = { ...preview, version: "owner-review-preview-v4", candidates: [{ ...candidate, candidate: { ...candidate.candidate, support: [candidate.candidate.support![0], source] } }], effect: { ...preview.effect!, version: "owner-review-effect-v4" } };
  contains(renderToStaticMarkup(<ReviewEffects preview={clock}/>),["tool_observation","tool_succeeded","local-clock-display-v1","2026-09-05 10:30:00","no inferred timezone","root-clock","execution-clock","ancestry-hash","owner_statement"]);
 });
 it("shows original and edited meaning, both evidence sets, reason and immutable parent audit", () => {
  const before = { proposal: candidate.candidate.proposal, support: candidate.candidate.support!, context: candidate.candidate.context!, identity: null, temporal: null };
  const edit: EditRevision = { candidate_id: "candidate-1", revision: 3, parent_revision: 2, review_revision: 4, audit_id: "edit-audit", before, after: { ...before, proposal: { ...before.proposal, proposition: { ...before.proposal.proposition, object: { literal: { kind: "text", value: "green tea" } } } } }, reason: "Owner correction of extraction" };
  contains(renderToStaticMarkup(<EditLineage edit={edit}/>),["Before edit","After edit","café","green tea","I prefer café.","Recorded.","revision 2","edit-audit","Owner correction of extraction","not new factual evidence"]);
 });
 it("renders compound members, dependencies, record lifecycle changes and mixed durable outcomes", () => {
  contains(renderToStaticMarkup(<BatchReview state={{ ...state, batchPreview, batchResult }} controller={controller()}/>),["all member changes succeed together","Member 1","Member 2","candidate-2 subject uses the exact subject created by candidate-1","Complete semantic record changes","new-maya","state_event","superseded","Independent successful groups may commit","never retried automatically","together: accepted","independent: failed","source_ineligible","new complete group","Confirm all displayed group actions"]);
 });
 it("distinguishes earlier recorded decisions from the failed current group", () => {
  const prior = batchResult.groups[0].result!; const priorResult = { ...batchResult, groups: [{ group_id: "together", outcome: "failed", failure_code: "already_resolved", result: null, prior_resolutions: [prior] }] };
  contains(renderToStaticMarkup(<BatchReview state={{ ...state, batchResult: priorResult }} controller={controller()}/>),["together: failed","Earlier recorded decision: accept","not a successful change by the current failed group","Earlier operation"]);
 });
 it("fails closed on an unknown nested compound effect", () => {
  const unknown = structuredClone(batchPreview); unknown.groups[0].preview.effect!.members![1].version = "owner-review-effect-v99";
  const html = renderToStaticMarkup(<BatchReview state={{ ...state, batchPreview: unknown }} controller={controller()}/>); expect(html).toContain("Additional review is required"); expect(html).toMatch(/disabled=""[^>]*>Confirm all displayed group actions/);
 });
 it("does not display source or choices from a redacted candidate", () => {
  expect(renderToStaticMarkup(<AdvancedReviewForms state={{ ...state, detail: { ...identityCandidate, redacted: true }, identityOptions, temporalOptions }} controller={controller()}/>)).toBe("");
 });
});
