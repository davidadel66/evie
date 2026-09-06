import { describe, expect, it, vi } from "vitest";
import { CandidateReviewError, type BatchPreview, type EditRevision, type IdentityOptions, type IdentityRevision, type OwnerCandidate, type TemporalRevision } from "../api/candidateReview";
import { CandidateInboxController } from "./controller";
import { pendingDecisionJournal } from "./pendingDecision";
import { supportedPreview } from "./previewSupport";
import { candidate, fakeAPI, preview } from "./fixtures.test-support";
import { batchPreview, batchRequest, batchResult, chosenCandidate, correctionPreview, identityCandidate, identityOptions, identityPreview, temporalOptions } from "./advanced.test-support";
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((yes) => { resolve = yes; }); return { promise, resolve }; }
async function ready(controller: CandidateInboxController) { await controller.selectScope("global"); await controller.inspect(candidate.ref.candidate_id); }

describe("advanced interpretation and exact approval", () => {
 it("loads both same-name alternatives without selecting a winner and records only the explicit typed choices", async () => {
  const chooseIdentity = vi.fn(async () => chosenCandidate); const controller = new CandidateInboxController(fakeAPI({ inspect: async () => identityCandidate, identityOptions: async () => identityOptions, chooseIdentity, prepare: async () => identityPreview }));
  await ready(controller); await controller.loadIdentityOptions(); expect(chooseIdentity).not.toHaveBeenCalled(); expect(controller.snapshot().identityOptions?.subject).toHaveLength(2);
  await controller.chooseIdentity({ candidate: identityOptions.candidate, options_sha256: identityOptions.options_sha256, choices: chosenCandidate.identity!.choices });
  expect(chooseIdentity).toHaveBeenCalledExactlyOnceWith("global", { candidate: identityOptions.candidate, options_sha256: identityOptions.options_sha256, choices: chosenCandidate.identity!.choices });
  expect(controller.snapshot().detail?.ref).toEqual(chosenCandidate.ref); expect(controller.snapshot().preview).toBeUndefined(); await controller.prepare("accept"); expect(controller.snapshot().preview).toEqual(identityPreview);
 });
 it("records error/changed choices exactly from the displayed alternatives without inventing an effective time", async () => {
  const chooseTemporal = vi.fn(async () => candidate); const controller = new CandidateInboxController(fakeAPI({ temporalOptions: async () => temporalOptions, chooseTemporal })); await ready(controller); await controller.loadTemporalOptions();
  expect(controller.snapshot().temporalOptions?.effective_time).toBeNull(); const decision = { candidate: candidate.ref, options_sha256: temporalOptions.options_sha256, choice: { old_claim_id: "old-tea", mode: "error" } }; await controller.chooseTemporal(decision);
  expect(chooseTemporal).toHaveBeenCalledExactlyOnceWith("global", decision); expect(controller.snapshot().preview).toBeUndefined();
 });
 it("edits the exact supplied revision and retains same-candidate reason after a stale edit", async () => {
  const edit = vi.fn(async () => { throw new CandidateReviewError("stale_preview", "Interpretation changed"); }); const controller = new CandidateInboxController(fakeAPI({ edit })); await ready(controller); controller.setReason("Keep my interpretation");
  const edited = { ...candidate.candidate.proposal, proposition: { ...candidate.candidate.proposal.proposition, object: { literal: { kind: "text", value: "green tea" } } } }; await controller.edit({ candidate: candidate.ref, proposal: edited, reason: "Correct the extracted value" });
  expect(edit).toHaveBeenCalledExactlyOnceWith("global", { candidate: candidate.ref, proposal: edited, reason: "Correct the extracted value" }); expect(controller.snapshot().reason).toBe("Keep my interpretation"); expect(controller.snapshot().preview).toBeUndefined(); expect(controller.snapshot().problem).toContain("Refresh");
 });
 it("fences delayed identity and mutation responses when the scope changes", async () => {
  const options = deferred<IdentityOptions>(); const mutation = deferred<OwnerCandidate>(); const controller = new CandidateInboxController(fakeAPI({ identityOptions: async () => options.promise, edit: async () => mutation.promise }));
  await ready(controller); const loading = controller.loadIdentityOptions(); await controller.selectScope("session:other"); options.resolve(identityOptions); await loading; expect(controller.snapshot().identityOptions).toBeUndefined();
  await ready(controller); const editing = controller.edit({ candidate: candidate.ref, proposal: candidate.candidate.proposal, reason: "" }); await controller.selectScope(""); mutation.resolve(chosenCandidate); await editing; expect(controller.snapshot().detail).toBeUndefined(); expect(controller.snapshot().scope).toBe("");
 });
 it("rejects options bound to a different interpretation or destination", async () => {
  for (const options of [{ ...identityOptions, scope_key: "session:other" }, { ...identityOptions, candidate: { ...candidate.ref, interpretation_revision: 4 } }]) {
   const controller = new CandidateInboxController(fakeAPI({ identityOptions: async () => options })); await ready(controller); await controller.loadIdentityOptions(); expect(controller.snapshot().identityOptions).toBeUndefined(); expect(controller.snapshot().problem).toContain("changed");
  }
 });
 it("clears displayed evidence and group drafts after a definitive source or freshness failure while retaining owner reason", async () => {
  for (const code of ["source_ineligible", "stale_preview", "review_unauthorized", "review_scope_quarantined"]) {
   const controller = new CandidateInboxController(fakeAPI({ prepare: async () => { throw new CandidateReviewError(code,"Refresh required"); } })); await ready(controller); controller.setReason("My pending decision reason"); await controller.prepare("accept");
   expect(controller.snapshot().detail).toBeUndefined(); expect(controller.snapshot().page).toBeUndefined(); expect(controller.snapshot().disclosureEpoch).toBe(1); expect(controller.snapshot().reason).toBe("My pending decision reason");
  }
 });
 it("supports complete v2/v3/v4 meanings and refuses unknown versions and standalone batch member approval", async () => {
  for (const value of [identityPreview, correctionPreview, { ...preview, version: "owner-review-preview-v4", effect: { ...preview.effect!, version: "owner-review-effect-v4" } }]) expect(supportedPreview(value)).toBe(true);
  expect(supportedPreview({ ...preview, version: "owner-review-preview-v99" })).toBe(false); expect(supportedPreview(batchPreview.groups[0].preview)).toBe(false);
 });
});

describe("mixed immutable interpretation history", () => {
 const meaning = { proposal: candidate.candidate.proposal, support: candidate.candidate.support!, context: candidate.candidate.context!, identity: null, temporal: null };
 const edit: EditRevision = { candidate_id: candidate.ref.candidate_id, revision: 2, parent_revision: 1, review_revision: 2, audit_id: "edit-audit", before: meaning, after: meaning, reason: "Corrected extraction" };
 const identity: IdentityRevision = chosenCandidate.identity!;
 const correction: TemporalRevision = { ...correctionPreview.effect!.correction!.revision, revision: 3, parent_revision: 2, review_revision: 3, options: { ...temporalOptions, candidate: { ...candidate.ref, interpretation_revision: 2, review_revision: 2 } } };
 const current = { ...candidate, ref: { ...candidate.ref, interpretation_revision: 3, review_revision: 3 } };
 const historyAPI = { inspect: async () => current, editRevision: async () => edit, identityRevision: async () => identity, temporalRevision: async () => correction };
 it("loads the requested kind in a shared identity/edit/correction sequence without changing current interpretation", async () => {
  const editRevision = vi.fn(historyAPI.editRevision), identityRevision = vi.fn(historyAPI.identityRevision), temporalRevision = vi.fn(historyAPI.temporalRevision);
  const controller = new CandidateInboxController(fakeAPI({ ...historyAPI, editRevision, identityRevision, temporalRevision })); await ready(controller);
  for (const [kind, value] of [["identity", identity], ["edit", edit], ["correction", correction]] as const) {
   await controller.loadHistoryRevision(kind, value.revision); expect(controller.snapshot().history).toEqual({ kind, value }); expect(controller.snapshot().detail).toEqual(current);
  }
  expect(identityRevision).toHaveBeenCalledExactlyOnceWith("global", candidate.ref.candidate_id, 1);
  expect(editRevision).toHaveBeenCalledExactlyOnceWith("global", candidate.ref.candidate_id, 2);
  expect(temporalRevision).toHaveBeenCalledExactlyOnceWith("global", candidate.ref.candidate_id, 3);
 });
 it("bounds requested revision before dispatch and refuses redacted history reads", async () => {
  const editRevision = vi.fn(historyAPI.editRevision); const controller = new CandidateInboxController(fakeAPI({ ...historyAPI, editRevision })); await ready(controller);
  for (const revision of [0, -1, 1.5, 4, NaN, Infinity]) { await controller.loadHistoryRevision("edit", revision); expect(controller.snapshot().history).toBeUndefined(); }
  expect(editRevision).not.toHaveBeenCalled();
  const redacted = new CandidateInboxController(fakeAPI({ ...historyAPI, inspect: async () => ({ ...current, redacted: true }), editRevision })); await ready(redacted); await redacted.loadHistoryRevision("edit", 2); expect(editRevision).not.toHaveBeenCalled();
 });
 it("rejects returned history for a different candidate, scope, revision, or parent binding", async () => {
  for (const value of [{ ...identity, revision: 2 }, { ...identity, parent_revision: 8 }, { ...identity, review_revision: 7 }, { ...identity, options: { ...identity.options, scope_key: "session:other" } }, { ...identity, options: { ...identity.options, candidate: { ...candidate.ref, candidate_id: "other" } } }]) {
   const controller = new CandidateInboxController(fakeAPI({ ...historyAPI, identityRevision: async () => value })); await ready(controller); await controller.loadHistoryRevision("identity", 1); expect(controller.snapshot().history).toBeUndefined(); expect(controller.snapshot().problem).toContain("does not match");
  }
  const controller = new CandidateInboxController(fakeAPI({ ...historyAPI, editRevision: async () => ({ ...edit, candidate_id: "other" }) })); await ready(controller); await controller.loadHistoryRevision("edit", 2); expect(controller.snapshot().history).toBeUndefined();
 });
 it("fences delayed history by scope, candidate selection and candidate refresh", async () => {
  for (const navigation of ["scope", "candidate", "refresh"]) {
   const response = deferred<TemporalRevision>(); const controller = new CandidateInboxController(fakeAPI({ ...historyAPI, temporalRevision: async () => response.promise })); await ready(controller);
   const reading = controller.loadHistoryRevision("correction", 3);
   if (navigation === "scope") await controller.selectScope("session:other");
   else await controller.inspect(navigation === "candidate" ? "other" : candidate.ref.candidate_id);
   response.resolve(correction); await reading; expect(controller.snapshot().history).toBeUndefined();
  }
 });
 it("clears historical disclosure on source or freshness failures and does not relabel unknown failures as missing history", async () => {
  for (const code of ["source_ineligible", "stale_preview", "review_unauthorized", "review_scope_quarantined", "review_revision_not_found", "review_retryable"]) {
   const controller = new CandidateInboxController(fakeAPI({ ...historyAPI, temporalRevision: async () => { throw new CandidateReviewError(code, code === "review_retryable" ? "Service could not confirm the read" : "Revision unavailable"); } })); await ready(controller);
   await controller.loadHistoryRevision("identity", 1); expect(controller.snapshot().history).toBeDefined(); await controller.loadHistoryRevision("correction", 3); expect(controller.snapshot().history).toBeUndefined();
   if (["source_ineligible", "stale_preview", "review_unauthorized", "review_scope_quarantined"].includes(code)) expect(controller.snapshot().detail).toBeUndefined();
   else expect(controller.snapshot().detail).toEqual(current);
   if (code === "review_retryable") { expect(controller.snapshot().problem).toBe("Service could not confirm the read"); expect(controller.snapshot().problem).not.toContain("not found"); }
  }
 });
});

describe("atomic group batch delivery", () => {
 it("prepares exactly selected refs, dependencies and actions, then explicitly delivers the outer preview", async () => {
  const prepareBatch = vi.fn(async () => batchPreview); const resolveBatch = vi.fn(async () => batchResult); const controller = new CandidateInboxController(fakeAPI({ prepareBatch, resolveBatch }), () => "idem:v1:batch-delivery"); await ready(controller); await controller.prepareBatch(batchRequest);
  expect(prepareBatch).toHaveBeenCalledExactlyOnceWith("global", batchRequest); expect(resolveBatch).not.toHaveBeenCalled(); expect(controller.snapshot().batchPreview).toEqual(batchPreview); controller.setReason("Accept complete closure"); await controller.resolveBatch();
  expect(resolveBatch).toHaveBeenCalledExactlyOnceWith("global", { delivery_key: "idem:v1:batch-delivery", preview_id: "batch-1", preview_sha256: "batch-hash", actions: [{ group_id: "together", action: "accept" }, { group_id: "independent", action: "reject" }], reason: "Accept complete closure" }); expect(controller.snapshot().batchResult).toEqual(batchResult); expect(controller.snapshot().batchPreview).toBeUndefined(); await controller.resolveBatch(); expect(resolveBatch).toHaveBeenCalledTimes(1);
 });
 it("retains exact outer actions/key/reason across uncertain delivery, scope navigation and browser reload", async () => {
  const bytes = new Map<string,string>(); const storage = { getItem: (key: string) => bytes.get(key) ?? null, setItem: (key: string,value: string) => { bytes.set(key,value); }, removeItem: (key: string) => { bytes.delete(key); } };
  const resolveBatch = vi.fn().mockRejectedValueOnce(new CandidateReviewError("review_retryable", "Unknown committed outcome")).mockResolvedValue(batchResult); const api = fakeAPI({ prepareBatch: async () => batchPreview, resolveBatch });
  const first = new CandidateInboxController(api, () => "idem:v1:batch-delivery", pendingDecisionJournal(storage)); await ready(first); await first.prepareBatch(batchRequest); first.setReason("This exact mixed batch"); await first.resolveBatch(); first.setReason("Changed intent"); expect(first.snapshot().reason).toBe("This exact mixed batch"); await first.selectScope("session:another"); first.invalidate();
  const raw = [...bytes.values()].join(""); for (const protectedText of ["Maya", "café", "owner_statement", "I prefer", "new-maya", "candidate-1"]) expect(raw).not.toContain(protectedText);
  const reopened = new CandidateInboxController(api, () => "must-not-create-key", pendingDecisionJournal(storage)); expect(reopened.snapshot().batchPreview).toBeUndefined(); await reopened.recover(); expect(resolveBatch.mock.calls[0]).toEqual(resolveBatch.mock.calls[1]); expect(bytes.size).toBe(0); expect(reopened.snapshot().batchResult?.groups[1].failure_code).toBe("source_ineligible"); await reopened.recover(); expect(resolveBatch).toHaveBeenCalledTimes(2);
 });
 it.each(["stale_preview","source_ineligible","review_dependencies","review_too_large","review_unauthorized","invalid_review_request"])("invalidates complete approval after %s and never retries a refreshed intent", async (code) => {
  const resolveBatch = vi.fn(async () => { throw new CandidateReviewError(code,"Review again"); }); const controller = new CandidateInboxController(fakeAPI({ prepareBatch: async () => batchPreview, resolveBatch })); await ready(controller); await controller.prepareBatch(batchRequest); controller.setReason("Original intent"); await controller.resolveBatch(); expect(controller.snapshot().batchPreview).toBeUndefined(); expect(controller.snapshot().recovery).toBeUndefined(); expect(controller.snapshot().reason).toBe("Original intent"); await controller.resolveBatch(); expect(resolveBatch).toHaveBeenCalledTimes(1);
 });
 it("fails closed when an action, revision, dependency or failure behavior differs from the submitted selection", async () => {
  const variants: BatchPreview[] = [structuredClone(batchPreview),structuredClone(batchPreview),structuredClone(batchPreview),structuredClone(batchPreview)]; variants[0].groups[0].preview.action = "reject"; variants[1].groups[0].preview.candidates[0].ref.review_revision++; variants[2].groups[0].preview.dependencies = []; variants[3].failure_behavior = "best_effort_unknown";
  for (const value of variants) { const controller = new CandidateInboxController(fakeAPI({ prepareBatch: async () => value })); await ready(controller); await controller.prepareBatch(batchRequest); expect(controller.snapshot().batchPreview).toBeUndefined(); expect(controller.snapshot().problem).toContain("Additional review"); }
 });
 it("keeps an uncertain single delivery from being replaced by a batch", async () => {
  const resolveBatch = vi.fn(async () => batchResult); const controller = new CandidateInboxController(fakeAPI({ resolve: async () => { throw new TypeError("lost"); }, prepareBatch: async () => batchPreview, resolveBatch })); await ready(controller); await controller.prepare("accept"); await controller.resolve(); await controller.prepareBatch(batchRequest); await controller.resolveBatch(); expect(resolveBatch).not.toHaveBeenCalled(); expect(controller.snapshot().problem).toContain("Recover the earlier decision");
 });
 it("fences delayed batch preparation on scope navigation and invalidates preview when the group draft changes", async () => {
  const response = deferred<BatchPreview>(); const controller = new CandidateInboxController(fakeAPI({ prepareBatch: async () => response.promise })); await ready(controller); const work = controller.prepareBatch(batchRequest); await controller.selectScope(""); response.resolve(batchPreview); await work; expect(controller.snapshot().batchPreview).toBeUndefined();
  const active = new CandidateInboxController(fakeAPI({ prepareBatch: async () => batchPreview })); await ready(active); await active.prepareBatch(batchRequest); active.invalidateBatchDraft(); expect(active.snapshot().batchPreview).toBeUndefined();
 });
 it("recovers a maximum escaped reason with twenty maximum-length group actions without truncation", async () => {
  const bytes = new Map<string,string>(); const storage = { getItem: (key: string) => bytes.get(key) ?? null, setItem: (key: string,value: string) => { bytes.set(key,value); }, removeItem: (key: string) => { bytes.delete(key); } };
  const reason = "\u0001".repeat(4096);
  const actions = Array.from({length:20}, (_,i) => ({ group_id: `${i}`.padEnd(64,"x"), action: "accept" as const }));
  const decision = { delivery_key: "idem:v1:batch-delivery", preview_id: "batch-1", preview_sha256: "digest".repeat(10), actions, reason };
  pendingDecisionJournal(storage).write({ scope: "global", decision });
  const raw = [...bytes.values()][0]; expect(new TextEncoder().encode(raw).length).toBeGreaterThan(24*1024); expect(new TextEncoder().encode(raw).length).toBeLessThan(32*1024);
  const resolveBatch = vi.fn(async () => batchResult); const reopened = new CandidateInboxController(fakeAPI({ resolveBatch }), () => "never-new-key", pendingDecisionJournal(storage)); await reopened.recover(); expect(resolveBatch).toHaveBeenCalledExactlyOnceWith("global",decision); expect(bytes.size).toBe(0);
  const single = { delivery_key: "idem:v1:single", preview_id: "preview-1", preview_sha256: "digest", action: "reject" as const, reason };
  pendingDecisionJournal(storage).write({ scope: "global", decision: single }); expect(pendingDecisionJournal(storage).read()?.decision).toEqual(single);
 });
 it("shows earlier winning metadata for a failed resolved group without treating it as a current success", async () => {
  const prior = { ...batchResult.groups[0].result!, delivery_key: "prior-delivery" };
  const outcome = { ...batchResult, groups: [{ group_id: "together", outcome: "failed", failure_code: "already_resolved", result: null, prior_resolutions: [prior] }] };
  const operation = vi.fn(fakeAPI().operation); const controller = new CandidateInboxController(fakeAPI({ prepareBatch: async () => batchPreview, resolveBatch: async () => outcome, operation })); await ready(controller); await controller.prepareBatch(batchRequest); await controller.resolveBatch();
  expect(controller.snapshot().batchResult?.groups[0].outcome).toBe("failed"); expect(controller.snapshot().batchResult?.groups[0].result).toBeNull(); expect(controller.snapshot().batchOperations).toEqual([]); expect(controller.snapshot().batchPriorOperations?.[0].group_id).toBe("together"); expect(operation).toHaveBeenCalledExactlyOnceWith("global",prior.operation!.operation_id);
 });
 it("bounds journal metadata by UTF-8 bytes, including non-ASCII reasons", () => { const journal = pendingDecisionJournal(); expect(() => journal.write({ scope: "global", decision: { delivery_key: "key", preview_id: "preview", preview_sha256: "digest", actions: [{ group_id: "one", action: "accept" }], reason: "🌍".repeat(3000) } })).toThrow("too large"); });
});
