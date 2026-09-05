import { CandidateReviewError, type CandidateReviewAPI, type CandidateScopes, type CandidatePage, type OwnerCandidate, type ReviewDecision, type ReviewPreview, type ReviewResult, type ReviewOperation, type IdentityOptions, type IdentityDecision, type TemporalOptions, type TemporalDecision, type EditDecision, type EditRevision, type IdentityRevision, type TemporalRevision, type BatchRequest, type BatchPreview, type BatchDecision, type BatchResult } from "../api/candidateReview";
import { supportedPreview, supportedBatch } from "./previewSupport";
import { pendingDecisionJournal, type PendingDecisionJournal, type PendingReview } from "./pendingDecision";

export type InterpretationHistory = { kind: "edit"; value: EditRevision } | { kind: "identity"; value: IdentityRevision } | { kind: "correction"; value: TemporalRevision };
export type InboxState = {
  disclosureEpoch?: number; identityOptions?: IdentityOptions; temporalOptions?: TemporalOptions; history?: InterpretationHistory;
  batchPreview?: BatchPreview; batchResult?: BatchResult; batchOperations?: ReviewOperation[]; batchPriorOperations?: { group_id: string; operation: ReviewOperation }[]; batchPending?: BatchDecision;
  scopes?: CandidateScopes; scope: string; page?: CandidatePage; detail?: OwnerCandidate; preview?: ReviewPreview;
  result?: ReviewResult; operation?: ReviewOperation; reason: string; busy: boolean; scopesBusy: boolean;
  problem: string; notice: string; pending?: ReviewDecision; recovery?: PendingReview;
};

// Each selected-scope request supersedes earlier reads and previews. A completed
// request can commit UI state only while its scope/selection is still current.
export class CandidateInboxController {
  private state: InboxState = { scope: "", reason: "", busy: false, scopesBusy: false, problem: "", notice: "" };
  private listeners = new Set<() => void>();
  private epoch = 0;
  private scopesEpoch = 0;
  private reasonCandidateID = "";
  private api: CandidateReviewAPI;
  private deliveryKey: () => string;
  private journal: PendingDecisionJournal;
  constructor(api: CandidateReviewAPI, deliveryKey = () => `idem:v1:${crypto.randomUUID()}`, journal = pendingDecisionJournal()) {
    this.api = api; this.deliveryKey = deliveryKey; this.journal = journal;
    try { this.state.recovery = journal.read(); } catch { this.state.problem = "Earlier review delivery could not be read. Refresh this browser before confirming a decision."; }
  }
  snapshot = () => this.state;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private clearAdvanced = { identityOptions: undefined, temporalOptions: undefined, history: undefined, batchOperations: undefined, batchPriorOperations: undefined, batchResult: undefined, batchPreview: undefined, batchPending: undefined };
  private set(patch: Partial<InboxState>) { this.state = { ...this.state, ...patch }; this.listeners.forEach((listener) => listener()); }
  // A definitive freshness/source failure invalidates displayed disclosure too,
  // including older batch selections. Owner reasons and delivery metadata survive.
  private fail(error: unknown, patch: Partial<InboxState>) {
    const protectedRead = error instanceof CandidateReviewError && ["stale_preview", "source_ineligible", "review_unauthorized", "review_scope_quarantined"].includes(error.code);
    this.set({ ...patch, ...(protectedRead ? { ...this.clearAdvanced, detail: undefined, page: undefined, preview: undefined, operation: undefined, batchResult: this.state.batchResult, disclosureEpoch: (this.state.disclosureEpoch ?? 0) + 1 } : {}) });
  }
  invalidate() { this.epoch++; this.scopesEpoch++; }
  invalidateBatchDraft() { this.set({ batchPreview: undefined, batchPending: undefined }); }
  setReason(reason: string) { if (!this.state.pending && !this.state.batchPending && !this.state.busy) this.set({ reason }); }

  async loadScopes(cursor = "") {
    const epoch = ++this.scopesEpoch;
    this.set({ scopesBusy: true });
    try { const scopes = await this.api.scopes(cursor); if (epoch === this.scopesEpoch) this.set({ scopes, scopesBusy: false }); }
    catch (error) { if (epoch === this.scopesEpoch) this.fail(error, { problem: message(error), scopesBusy: false }); }
  }

  async selectScope(scope: string) {
    this.epoch++;
    this.reasonCandidateID = "";
    this.set({ ...this.clearAdvanced, batchResult: undefined, batchOperations: undefined, scope, page: undefined, detail: undefined, preview: undefined, pending: undefined, result: undefined, operation: undefined, reason: "", problem: "", notice: "", busy: false });
    if (scope) await this.load();
  }

  async load(cursor = "") {
    const scope = this.state.scope; if (!scope) return;
    const epoch = ++this.epoch;
    this.set({ ...this.clearAdvanced, busy: true, problem: "", detail: undefined, preview: undefined, pending: undefined });
    try { const page = await this.api.list(scope, cursor); if (epoch === this.epoch) this.set({ page, busy: false }); }
    catch (error) { if (epoch === this.epoch) this.fail(error, { busy: false, problem: message(error) }); }
  }

  async inspect(id: string) {
    const scope = this.state.scope; if (!scope) return;
    const epoch = ++this.epoch;
    const reason = this.reasonCandidateID === id ? this.state.reason : "";
    this.reasonCandidateID = id;
    this.set({ ...this.clearAdvanced, busy: true, detail: undefined, preview: undefined, pending: undefined, reason, result: undefined, operation: undefined, problem: "", notice: "" });
    try { const detail = await this.api.inspect(scope, id); if (epoch === this.epoch) this.set({ detail, busy: false }); }
    catch (error) { if (epoch === this.epoch) this.fail(error, { busy: false, problem: message(error) }); }
  }

  async prepare(action: "accept" | "reject") {
    const { detail, scope, busy } = this.state; if (!detail || busy) return;
    const epoch = ++this.epoch;
    this.set({ ...this.clearAdvanced, busy: true, preview: undefined, pending: undefined, problem: "", notice: "" });
    try {
      const preview = await this.api.prepare(scope, detail.ref, action);
      if (epoch !== this.epoch) return;
      const sameRef = preview.candidates.length === 1 && preview.candidates[0].ref.candidate_id === detail.ref.candidate_id && preview.candidates[0].ref.interpretation_revision === detail.ref.interpretation_revision && preview.candidates[0].ref.review_revision === detail.ref.review_revision;
      if (preview.scope_key !== scope || preview.action !== action || !sameRef || !supportedPreview(preview)) {
        this.set({ busy: false, problem: "Additional review is required: this preview contains an unsupported effect or a different candidate binding." }); return;
      }
      this.set({ preview, busy: false });
    } catch (error) { if (epoch === this.epoch) this.fail(error, { busy: false, problem: message(error) }); }
  }

  async resolve() {
    const { scope, preview, busy, pending, reason } = this.state; if (!preview || busy) return;
    const epoch = ++this.epoch;
    const decision = pending ?? { delivery_key: this.deliveryKey(), preview_id: preview.preview_id, preview_sha256: preview.preview_sha256, action: preview.action, reason };
    try { this.journal.write({ scope, decision }); }
    catch (error) { this.set({ problem: message(error) }); return; }
    this.set({ recovery: { scope, decision } });
    await this.deliver(epoch, scope, decision);
  }

  async recover() {
    if (this.state.busy) return;
    let pending: PendingReview | undefined;
    try { pending = this.journal.read(); } catch (error) { this.set({ problem: message(error) }); return; }
    if (!pending) { this.set({ recovery: undefined }); return; }
    const epoch = ++this.epoch;
    this.reasonCandidateID = "";
    this.set({ scope: pending.scope, page: undefined, detail: undefined, preview: undefined, result: undefined, operation: undefined, recovery: pending, notice: "", reason: pending.decision.reason, ...this.clearAdvanced });
    if ("actions" in pending.decision) await this.deliverBatch(epoch, pending.scope, pending.decision);
    else await this.deliver(epoch, pending.scope, pending.decision);
  }

  private async deliver(epoch: number, scope: string, decision: ReviewDecision) {
    this.set({ busy: true, pending: decision, problem: "" });
    try {
      const result = await this.api.resolve(scope, decision);
      this.journal.clear(decision.delivery_key);
      if (epoch !== this.epoch) return;
      this.reasonCandidateID = "";
      this.set({ result, reason: "", pending: undefined, recovery: undefined, preview: undefined, detail: undefined, notice: result.action === "accept" ? "Candidate accepted. Its reviewed claim and sources are now in memory." : "Candidate rejected. Accepted memory is unchanged." });
      await this.refreshAfterResolution(epoch, scope, result);
    } catch (error) {
      if (error instanceof CandidateReviewError && error.code === "already_resolved" && error.result) {
        this.journal.clear(decision.delivery_key);
        if (epoch !== this.epoch) return;
        this.reasonCandidateID = "";
        this.set({ result: error.result, reason: "", pending: undefined, recovery: undefined, preview: undefined, detail: undefined, notice: `This candidate was already ${error.result.action === "accept" ? "accepted" : "rejected"}. The recorded decision is shown.` });
        await this.refreshAfterResolution(epoch, scope, error.result); return;
      }
      // Definitive Kernel conflicts require a freshly inspected and explicitly
      // approved preview. An uncertain transport result retains this exact key.
      const definitive = error instanceof CandidateReviewError && ["stale_preview", "source_ineligible", "idempotency_conflict", "review_unauthorized", "already_resolved", "invalid_review_request", "review_scope_quarantined", "review_dependencies", "review_too_large", "needs_choice"].includes(error.code);
      if (definitive) this.journal.clear(decision.delivery_key);
      if (epoch !== this.epoch) return;
      this.fail(error, { busy: false, problem: message(error), ...(definitive ? { preview: undefined, pending: undefined, recovery: undefined } : {}) });
    }
  }

  async loadIdentityOptions() {
    const { detail, scope, busy } = this.state; if (!detail || busy) return;
    const epoch = ++this.epoch; this.set({ busy: true, problem: "", preview: undefined, identityOptions: undefined });
    try { const options = await this.api.identityOptions(scope, detail.ref); if (epoch !== this.epoch) return;
      if (options.scope_key !== scope || !sameRef(options.candidate, detail.ref)) throw new Error("Identity alternatives changed. Inspect the candidate again.");
      this.set({ identityOptions: options, busy: false });
    } catch (error) { if (epoch === this.epoch) this.fail(error, { busy: false, problem: message(error) }); }
  }
  async loadTemporalOptions() {
    const { detail, scope, busy } = this.state; if (!detail || busy) return;
    const epoch = ++this.epoch; this.set({ busy: true, problem: "", preview: undefined, temporalOptions: undefined });
    try { const options = await this.api.temporalOptions(scope, detail.ref); if (epoch !== this.epoch) return;
      if (options.scope_key !== scope || !sameRef(options.candidate, detail.ref)) throw new Error("Correction alternatives changed. Inspect the candidate again.");
      this.set({ temporalOptions: options, busy: false });
    } catch (error) { if (epoch === this.epoch) this.fail(error, { busy: false, problem: message(error) }); }
  }
  async chooseIdentity(decision: IdentityDecision) { await this.interpret(() => this.api.chooseIdentity(this.state.scope, decision)); }
  async chooseTemporal(decision: TemporalDecision) { await this.interpret(() => this.api.chooseTemporal(this.state.scope, decision)); }
  async edit(decision: EditDecision) { await this.interpret(() => this.api.edit(this.state.scope, decision)); }
  private async interpret(call: () => Promise<OwnerCandidate>) {
    const { scope, detail, busy } = this.state; if (busy || !detail) return;
    const epoch = ++this.epoch; this.set({ ...this.clearAdvanced, busy: true, preview: undefined, problem: "", notice: "" });
    try { const item = await call(); if (epoch !== this.epoch) return;
      if (item.destination !== scope || item.ref.candidate_id !== detail.ref.candidate_id) throw new Error("The returned interpretation does not match this candidate.");
      this.set({ detail: item, busy: false, notice: "Interpretation recorded. Review a new exact preview before accepting memory." });
    } catch (error) { if (epoch === this.epoch) this.fail(error, { busy: false, problem: `${message(error)} Refresh to inspect the recorded interpretation before another edit or choice.` }); }
  }
  async loadHistoryRevision(kind: InterpretationHistory["kind"], revision: number) {
    const { scope, detail, busy } = this.state; if (!detail || detail.redacted || busy) return;
    if (!Number.isSafeInteger(revision) || revision < 1 || revision > detail.ref.interpretation_revision) {
      this.set({ history: undefined, problem: `Choose an interpretation revision from 1 to ${detail.ref.interpretation_revision}.` }); return;
    }
    const epoch = ++this.epoch; this.set({ busy: true, problem: "", history: undefined });
    try {
      const id = detail.ref.candidate_id;
      let history: InterpretationHistory;
      if (kind === "edit") history = { kind, value: await this.api.editRevision(scope, id, revision) };
      else if (kind === "identity") history = { kind, value: await this.api.identityRevision(scope, id, revision) };
      else history = { kind, value: await this.api.temporalRevision(scope, id, revision) };
      if (epoch !== this.epoch) return;
      const value = history.value;
      const bound = value.revision === revision && value.parent_revision === revision - 1 && value.review_revision > 0 && value.review_revision <= detail.ref.review_revision;
      const candidateBound = history.kind === "edit" ? history.value.candidate_id === id :
        history.value.options.scope_key === scope && history.value.options.candidate.candidate_id === id &&
        history.value.options.candidate.interpretation_revision === value.parent_revision && history.value.options.candidate.review_revision === value.review_revision - 1;
      if (!bound || !candidateBound) throw new Error("The returned history does not match the requested candidate, kind or revision. Refresh and inspect again.");
      this.set({ history, busy: false });
    }
    catch (error) { if (epoch === this.epoch) this.fail(error, { busy: false, problem: message(error) }); }
  }
  async prepareBatch(request: BatchRequest) {
    const { scope, busy } = this.state; if (!scope || busy) return;
    const epoch = ++this.epoch; this.set({ ...this.clearAdvanced, busy: true, preview: undefined, problem: "", batchResult: undefined, batchOperations: undefined });
    try { const batchPreview = await this.api.prepareBatch(scope, request); if (epoch !== this.epoch) return;
      const bound = batchPreview.groups.length === request.groups.length && batchPreview.groups.every((group, i) => {
        const wanted = request.groups[i];
        return group.group_id === wanted.group_id && group.preview.action === wanted.action && group.preview.candidates.length === wanted.candidates.length && group.preview.candidates.every((candidate, j) => sameRef(candidate.ref, wanted.candidates[j])) && JSON.stringify(sortedDependencies(group.preview.dependencies ?? [])) === JSON.stringify(sortedDependencies(wanted.dependencies));
      });
      if (batchPreview.scope_key !== scope || !supportedBatch(batchPreview) || !bound) throw new Error("Additional review is required: batch effects or exact group bindings are unsupported.");
      this.set({ batchPreview, busy: false });
    } catch (error) { if (epoch === this.epoch) this.fail(error, { busy: false, problem: message(error) }); }
  }
  async resolveBatch() {
    const { scope, batchPreview, busy, batchPending, reason } = this.state; if (!batchPreview || busy || !supportedBatch(batchPreview)) return;
    const decision = batchPending ?? { delivery_key: this.deliveryKey(), preview_id: batchPreview.preview_id, preview_sha256: batchPreview.preview_sha256, actions: batchPreview.groups.map((group) => ({ group_id: group.group_id, action: group.preview.action })), reason };
    try { this.journal.write({ scope, decision }); } catch (error) { this.set({ problem: message(error) }); return; }
    const epoch = ++this.epoch; this.set({ recovery: { scope, decision } });
    await this.deliverBatch(epoch, scope, decision);
  }
  private async deliverBatch(epoch: number, scope: string, decision: BatchDecision) {
    this.set({ busy: true, batchPending: decision, problem: "" });
    try {
      const batchResult = await this.api.resolveBatch(scope, decision);
      this.journal.clear(decision.delivery_key); if (epoch !== this.epoch) return;
      this.set({ batchResult, disclosureEpoch: (this.state.disclosureEpoch ?? 0) + 1, batchPreview: undefined, batchPending: undefined, recovery: undefined, detail: undefined, reason: "", notice: "Batch decision recorded. Failed groups require a new preview and explicit approval." });
      const page = await this.api.list(scope); if (epoch !== this.epoch) return; this.set({ page });
      const operations: ReviewOperation[] = [];
      for (const group of batchResult.groups) if (group.result?.operation) { const op = await this.api.operation(scope, group.result.operation.operation_id); if (epoch !== this.epoch) return; operations.push(op); }
      const priorOperations: { group_id: string; operation: ReviewOperation }[] = [];
      for (const group of batchResult.groups) for (const prior of group.prior_resolutions ?? []) if (prior.operation) {
        const operation = await this.api.operation(scope, prior.operation.operation_id); if (epoch !== this.epoch) return;
        priorOperations.push({ group_id: group.group_id, operation });
      }
      this.set({ batchOperations: operations, batchPriorOperations: priorOperations, busy: false });
    } catch (error) {
      const definitive = error instanceof CandidateReviewError && ["stale_preview", "source_ineligible", "idempotency_conflict", "review_unauthorized", "already_resolved", "invalid_review_request", "review_scope_quarantined", "review_dependencies", "review_too_large", "needs_choice"].includes(error.code);
      if (definitive) this.journal.clear(decision.delivery_key);
      if (epoch === this.epoch) this.fail(error, { busy: false, problem: message(error), ...(definitive ? { batchPreview: undefined, batchPending: undefined, recovery: undefined } : {}) });
    }
  }

  private async refreshAfterResolution(epoch: number, scope: string, result: ReviewResult) {
    try {
      const page = await this.api.list(scope);
      if (epoch !== this.epoch) return;
      this.set({ page });
      if (result.operation) {
        const operation = await this.api.operation(scope, result.operation.operation_id);
        if (epoch !== this.epoch) return;
        this.set({ operation });
      }
      this.set({ busy: false });
    } catch (error) { if (epoch === this.epoch) this.fail(error, { busy: false, problem: `Decision recorded. Refresh to load the current inbox and provenance. ${message(error)}` }); }
  }
}

function message(error: unknown) { return error instanceof Error ? error.message : "Candidate review is unavailable. Try refreshing."; }

function sameRef(a: import("../api/candidateReview").CandidateRef, b: import("../api/candidateReview").CandidateRef) { return a.candidate_id === b.candidate_id && a.interpretation_revision === b.interpretation_revision && a.review_revision === b.review_revision; }
function sortedDependencies(deps: import("../api/candidateReview").ReviewDependency[]) { return deps.map((dep) => JSON.stringify([dep.candidate_id, dep.field, dep.from_candidate_id, dep.from_field])).sort(); }
