import { describe, expect, it, vi } from "vitest";
import { CandidateReviewError, type CandidatePage, type OwnerCandidate, type ReviewPreview, type ReviewResult } from "../api/candidateReview";
import { CandidateInboxController } from "./controller";
import { candidate, fakeAPI, preview, result } from "./fixtures.test-support";
import { pendingDecisionJournal } from "./pendingDecision";

function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; }); return { promise, resolve, reject }; }
async function ready(controller: CandidateInboxController) { await controller.selectScope("global"); await controller.inspect(candidate.ref.candidate_id); }

describe("candidate inbox state and exact approval", () => {
  it("prepares without resolving and sends the exact preview only on explicit confirmation", async () => {
    const resolve = vi.fn(async () => result);
    const prepare = vi.fn(fakeAPI().prepare);
    const controller = new CandidateInboxController(fakeAPI({ resolve, prepare }), () => "idem:v1:delivery-1");
    await ready(controller); await controller.prepare("accept");
    expect(resolve).not.toHaveBeenCalled();
    expect(prepare).toHaveBeenCalledWith("global", candidate.ref, "accept");
    controller.setReason("The source matches"); await controller.resolve();
    expect(resolve).toHaveBeenCalledExactlyOnceWith("global", { delivery_key: "idem:v1:delivery-1", preview_id: "preview-1", preview_sha256: "preview-hash", action: "accept", reason: "The source matches" });
    expect(controller.snapshot().operation?.preview).toEqual(preview);
    expect(controller.snapshot().preview).toBeUndefined();
    expect(controller.snapshot().notice).toContain("accepted");
  });

  it("retries uncertain delivery with the same key, action, digest and reason", async () => {
    const resolve = vi.fn().mockRejectedValueOnce(new TypeError("Connection lost")).mockResolvedValue(result);
    const key = vi.fn(() => "idem:v1:delivery-1");
    const controller = new CandidateInboxController(fakeAPI({ resolve }), key);
    await ready(controller); await controller.prepare("accept"); controller.setReason("Keep this"); await controller.resolve();
    expect(controller.snapshot().pending?.reason).toBe("Keep this");
    controller.setReason("changed after uncertain response"); await controller.resolve();
    expect(resolve.mock.calls[0]).toEqual(resolve.mock.calls[1]); expect(key).toHaveBeenCalledTimes(1);
  });

  it.each(["stale_preview", "source_ineligible", "idempotency_conflict", "review_unauthorized"])("does not carry approval after %s", async (code) => {
    const resolve = vi.fn(async () => { throw new CandidateReviewError(code, "Refresh and review again"); });
    const controller = new CandidateInboxController(fakeAPI({ resolve }));
    await ready(controller); await controller.prepare("accept"); controller.setReason("My intent"); await controller.resolve();
    expect(controller.snapshot().preview).toBeUndefined(); expect(controller.snapshot().pending).toBeUndefined();
    expect(controller.snapshot().reason).toBe("My intent");
    await controller.resolve(); expect(resolve).toHaveBeenCalledTimes(1);
  });

  it("preserves the reason for the same candidate after stale refresh without retaining approval", async () => {
    const resolve = vi.fn(async () => { throw new CandidateReviewError("stale_preview", "Refresh and review again"); });
    const api = fakeAPI({ resolve });
    const controller = new CandidateInboxController(api);
    await ready(controller); await controller.prepare("accept"); controller.setReason("My reason survives refreshed evidence"); await controller.resolve();
    await controller.load();
    const revised = { ...candidate, ref: { ...candidate.ref, interpretation_revision: 2, review_revision: 2 } };
    api.inspect = async () => revised;
    await controller.inspect(candidate.ref.candidate_id);
    expect(controller.snapshot().reason).toBe("My reason survives refreshed evidence");
    expect(controller.snapshot().preview).toBeUndefined(); expect(controller.snapshot().pending).toBeUndefined();
    await controller.resolve(); expect(resolve).toHaveBeenCalledTimes(1);
    api.inspect = async () => ({ ...candidate, ref: { ...candidate.ref, candidate_id: "another-candidate" } });
    await controller.inspect("another-candidate"); expect(controller.snapshot().reason).toBe("");
    controller.setReason("Different draft"); await controller.selectScope(""); expect(controller.snapshot().reason).toBe("");
  });

  it("shows the recorded competing resolution and refreshes without approving again", async () => {
    const rejected = { ...result, action: "reject", operation: null };
    const controller = new CandidateInboxController(fakeAPI({ resolve: async () => { throw new CandidateReviewError("already_resolved", "Already decided", rejected); } }));
    await ready(controller); await controller.prepare("accept"); await controller.resolve();
    expect(controller.snapshot().result).toEqual(rejected); expect(controller.snapshot().notice).toContain("already rejected"); expect(controller.snapshot().preview).toBeUndefined();
  });

  it("rejects advanced acceptance previews and retains the exact interpretation revision", async () => {
    const revised = { ...candidate, ref: { ...candidate.ref, interpretation_revision: 3, review_revision: 3 } };
    const prepare = vi.fn(async (): Promise<ReviewPreview> => ({ ...preview, version: "owner-review-preview-v2", candidates: [revised] }));
    const controller = new CandidateInboxController(fakeAPI({ inspect: async () => revised, prepare }));
    await ready(controller); await controller.prepare("accept");
    expect(prepare).toHaveBeenCalledWith("global", revised.ref, "accept"); expect(controller.snapshot().preview).toBeUndefined(); expect(controller.snapshot().problem).toContain("Additional review");
  });

  it("passes exact pagination and invalidates an old preview before reading the next page", async () => {
    const list = vi.fn(fakeAPI().list); const controller = new CandidateInboxController(fakeAPI({ list }));
    await ready(controller); await controller.prepare("accept"); await controller.load("next-1");
    expect(list).toHaveBeenLastCalledWith("global", "next-1"); expect(controller.snapshot().preview).toBeUndefined();
  });

  it("recovers an uncertain delivery after reload using only bounded request metadata", async () => {
    const bytes = new Map<string, string>();
    const storage = { getItem: (key: string) => bytes.get(key) ?? null, setItem: (key: string, value: string) => { bytes.set(key, value); }, removeItem: (key: string) => { bytes.delete(key); } };
    const resolve = vi.fn().mockRejectedValueOnce(new TypeError("Connection lost")).mockResolvedValue(result);
    const api = fakeAPI({ resolve });
    const first = new CandidateInboxController(api, () => "idem:v1:delivery-1", pendingDecisionJournal(storage));
    await ready(first); await first.prepare("accept"); first.setReason("Keep the preference"); await first.resolve(); first.invalidate();
    const retained = [...bytes.values()].join(""); expect(retained).not.toContain("café"); expect(retained).not.toContain("Recorded."); expect(retained).not.toContain("owner_statement"); expect(retained.length).toBeLessThan(8192);
    const reopened = new CandidateInboxController(api, () => "never-a-new-key", pendingDecisionJournal(storage));
    expect(reopened.snapshot().preview).toBeUndefined(); expect(reopened.snapshot().recovery?.decision.reason).toBe("Keep the preference"); expect(resolve).toHaveBeenCalledTimes(1);
    await reopened.recover(); expect(resolve.mock.calls[0]).toEqual(resolve.mock.calls[1]); expect(reopened.snapshot().operation?.preview).toEqual(preview); expect(bytes.size).toBe(0);
  });

  it.each(["review_retryable", "review_unavailable", "request_failed"])("retains exact delivery after unknown server outcome %s across reload", async (code) => {
    const bytes = new Map<string, string>();
    const storage = { getItem: (key: string) => bytes.get(key) ?? null, setItem: (key: string, value: string) => { bytes.set(key, value); }, removeItem: (key: string) => { bytes.delete(key); } };
    const resolve = vi.fn().mockRejectedValueOnce(new CandidateReviewError(code, "Outcome uncertain")).mockResolvedValue(result);
    const api = fakeAPI({ resolve });
    const first = new CandidateInboxController(api, () => "idem:v1:delivery-1", pendingDecisionJournal(storage));
    await ready(first); await first.prepare("accept"); first.setReason("Keep this exact interpretation"); await first.resolve(); first.invalidate();
    expect(first.snapshot().recovery?.decision.delivery_key).toBe("idem:v1:delivery-1");
    const reopened = new CandidateInboxController(api, () => "never-a-new-key", pendingDecisionJournal(storage));
    expect(reopened.snapshot().preview).toBeUndefined();
    await reopened.recover();
    expect(resolve).toHaveBeenCalledTimes(2); expect(resolve.mock.calls[0]).toEqual(resolve.mock.calls[1]);
    expect(reopened.snapshot().result).toEqual(result); expect(bytes.size).toBe(0);
  });

  it("does not dispatch when the recovery request cannot be persisted", async () => {
    const resolve = vi.fn(async () => result);
    const journal = pendingDecisionJournal({ getItem: () => null, setItem: () => { throw new Error("Storage unavailable"); }, removeItem: () => undefined });
    const controller = new CandidateInboxController(fakeAPI({ resolve }), undefined, journal);
    await ready(controller); await controller.prepare("accept"); await controller.resolve();
    expect(resolve).not.toHaveBeenCalled(); expect(controller.snapshot().problem).toBe("Storage unavailable");
  });
});

describe("stale asynchronous review responses", () => {
  for (const action of ["scope change", "unmount"] as const) {
    it.each(["list", "detail", "prepare", "resolve"] as const)(`ignores %s results after ${action}`, async (stage) => {
      const page = deferred<CandidatePage>(); const detail = deferred<OwnerCandidate>(); const prepared = deferred<ReviewPreview>(); const resolved = deferred<ReviewResult>();
      const api = fakeAPI();
      const controller = new CandidateInboxController(api);
      if (stage !== "list") await ready(controller);
      if (stage === "resolve") await controller.prepare("accept");
      let pending: Promise<void>;
      if (stage === "list") { api.list = async () => page.promise; pending = controller.selectScope("global"); }
      else if (stage === "detail") { api.inspect = async () => detail.promise; pending = controller.inspect("candidate-1"); }
      else if (stage === "prepare") { api.prepare = async () => prepared.promise; pending = controller.prepare("accept"); }
      else { api.resolve = async () => resolved.promise; pending = controller.resolve(); }
      if (action === "scope change") await controller.selectScope(""); else controller.invalidate();
      const snapshot = controller.snapshot();
      page.resolve({ scope_key: "global", revision: 1, candidates: [candidate], next_cursor: "" }); detail.resolve(candidate); prepared.resolve(preview); resolved.resolve(result);
      await pending; expect(controller.snapshot()).toBe(snapshot);
    });
  }

  it("ignores stale errors and stale source-operation reads after a scope change", async () => {
    const operation = deferred<{ operation_id: string; preview: ReviewPreview; audit_id: string }>();
    const started = deferred<void>();
    const controller = new CandidateInboxController(fakeAPI({ operation: async () => { started.resolve(); return operation.promise; } }));
    await ready(controller); await controller.prepare("accept"); const pending = controller.resolve(); await started.promise;
    await controller.selectScope(""); operation.reject(new Error("old protected source read failed")); await pending;
    expect(controller.snapshot().problem).toBe(""); expect(controller.snapshot().operation).toBeUndefined();
  });
});
