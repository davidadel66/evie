import { CandidateReviewError, type CandidateReviewAPI, type CandidateScopes, type CandidatePage, type OwnerCandidate, type ReviewDecision, type ReviewPreview, type ReviewResult, type ReviewOperation } from "../api/candidateReview";
import { pendingDecisionJournal, type PendingDecisionJournal, type PendingReview } from "./pendingDecision";

export type InboxState = {
  scopes?: CandidateScopes; scope: string; page?: CandidatePage; detail?: OwnerCandidate; preview?: ReviewPreview;
  result?: ReviewResult; operation?: ReviewOperation; reason: string; busy: boolean; scopesBusy: boolean;
  problem: string; notice: string; pending?: ReviewDecision; recovery?: PendingReview;
};

export function simpleAcceptPreview(preview: ReviewPreview): boolean {
  return preview.version === "owner-review-preview-v1" && preview.action === "accept" && preview.effect?.version === "owner-review-effect-v1" && !preview.effect.identity && preview.effect.claims.length === 1 && preview.candidates.length === 1;
}

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
  private set(patch: Partial<InboxState>) { this.state = { ...this.state, ...patch }; this.listeners.forEach((listener) => listener()); }
  invalidate() { this.epoch++; this.scopesEpoch++; }
  setReason(reason: string) { if (!this.state.pending && !this.state.busy) this.set({ reason }); }

  async loadScopes(cursor = "") {
    const epoch = ++this.scopesEpoch;
    this.set({ scopesBusy: true });
    try { const scopes = await this.api.scopes(cursor); if (epoch === this.scopesEpoch) this.set({ scopes, scopesBusy: false }); }
    catch (error) { if (epoch === this.scopesEpoch) this.set({ problem: message(error), scopesBusy: false }); }
  }

  async selectScope(scope: string) {
    this.epoch++;
    this.reasonCandidateID = "";
    this.set({ scope, page: undefined, detail: undefined, preview: undefined, pending: undefined, result: undefined, operation: undefined, reason: "", problem: "", notice: "", busy: false });
    if (scope) await this.load();
  }

  async load(cursor = "") {
    const scope = this.state.scope; if (!scope) return;
    const epoch = ++this.epoch;
    this.set({ busy: true, problem: "", detail: undefined, preview: undefined, pending: undefined });
    try { const page = await this.api.list(scope, cursor); if (epoch === this.epoch) this.set({ page, busy: false }); }
    catch (error) { if (epoch === this.epoch) this.set({ busy: false, problem: message(error) }); }
  }

  async inspect(id: string) {
    const scope = this.state.scope; if (!scope) return;
    const epoch = ++this.epoch;
    const reason = this.reasonCandidateID === id ? this.state.reason : "";
    this.reasonCandidateID = id;
    this.set({ busy: true, detail: undefined, preview: undefined, pending: undefined, reason, result: undefined, operation: undefined, problem: "", notice: "" });
    try { const detail = await this.api.inspect(scope, id); if (epoch === this.epoch) this.set({ detail, busy: false }); }
    catch (error) { if (epoch === this.epoch) this.set({ busy: false, problem: message(error) }); }
  }

  async prepare(action: "accept" | "reject") {
    const { detail, scope, busy } = this.state; if (!detail || busy) return;
    const epoch = ++this.epoch;
    this.set({ busy: true, preview: undefined, pending: undefined, problem: "", notice: "" });
    try {
      const preview = await this.api.prepare(scope, detail.ref, action);
      if (epoch !== this.epoch) return;
      const sameRef = preview.candidates.length === 1 && preview.candidates[0].ref.candidate_id === detail.ref.candidate_id && preview.candidates[0].ref.interpretation_revision === detail.ref.interpretation_revision && preview.candidates[0].ref.review_revision === detail.ref.review_revision;
      if (preview.scope_key !== scope || preview.action !== action || !sameRef || (action === "accept" && !simpleAcceptPreview(preview))) {
        this.set({ busy: false, problem: "Additional review is required for this candidate. Use local review to inspect all identity or compound effects." }); return;
      }
      this.set({ preview, busy: false });
    } catch (error) { if (epoch === this.epoch) this.set({ busy: false, problem: message(error) }); }
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
    this.set({ scope: pending.scope, page: undefined, detail: undefined, preview: undefined, result: undefined, operation: undefined, recovery: pending, notice: "", reason: pending.decision.reason });
    await this.deliver(epoch, pending.scope, pending.decision);
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
      const definitive = error instanceof CandidateReviewError && ["stale_preview", "source_ineligible", "idempotency_conflict", "review_unauthorized", "already_resolved", "invalid_review_request", "review_scope_quarantined"].includes(error.code);
      if (definitive) this.journal.clear(decision.delivery_key);
      if (epoch !== this.epoch) return;
      this.set({ busy: false, problem: message(error), ...(definitive ? { preview: undefined, pending: undefined, recovery: undefined } : {}) });
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
    } catch (error) { if (epoch === this.epoch) this.set({ busy: false, problem: `Decision recorded. Refresh to load the current inbox and provenance. ${message(error)}` }); }
  }
}

function message(error: unknown) { return error instanceof Error ? error.message : "Candidate review is unavailable. Try refreshing."; }
