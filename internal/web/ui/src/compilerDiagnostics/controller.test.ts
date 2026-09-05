import { describe, expect, it, vi } from "vitest";
import { CompilerDiagnosticsController } from "./controller";
import { diagnosticAPI, diagnosticPage } from "./fixtures.test-support";
import { CandidateReviewError } from "../api/candidateReview";
import type { CompilerDiagnostics, DiagnosticSessions } from "../api/compilerDiagnostics";

function deferred<T>() { let resolve!: (value: T) => void; let reject!: (reason?: unknown) => void; const promise = new Promise<T>((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; }

describe("compiler health navigation", () => {
 it("fences late scope-session listings and never selects a session implicitly", async () => {
  const old = deferred<DiagnosticSessions>();
  const controller = new CompilerDiagnosticsController(diagnosticAPI({ sessions: (scope) => scope === "global" ? old.promise : Promise.resolve({ session_ids: ["new-session"] }) }));
  const first = controller.selectScope("global"); await controller.selectScope("session:new-session"); old.resolve({ session_ids: ["private-old-session"] }); await first;
  expect(controller.snapshot().sessions?.session_ids).toEqual(["new-session"]); expect(controller.snapshot().session).toBe(""); expect(controller.snapshot().page).toBeUndefined();
 });
 it("fences late session and view results, including errors, and clears the preceding disclosure immediately", async () => {
  const old = deferred<CompilerDiagnostics>(); const staleError = deferred<CompilerDiagnostics>();
  const api = diagnosticAPI({ inspect: (_scope, query) => query.session_id === "old" ? old.promise : query.view === "candidates" ? staleError.promise : Promise.resolve({ ...diagnosticPage, session_id: query.session_id, view: query.view }) });
  const controller = new CompilerDiagnosticsController(api); await controller.selectScope("global");
  const first = controller.selectSession("old"); await controller.selectSession("closed-session"); old.resolve({ ...diagnosticPage, session_id: "old" }); await first;
  expect(controller.snapshot().page?.session_id).toBe("closed-session");
  const second = controller.selectView("candidates"); expect(controller.snapshot().page).toBeUndefined(); await controller.selectView("foreground"); staleError.reject(new Error("private old text")); await second;
  expect(controller.snapshot().page?.view).toBe("foreground"); expect(controller.snapshot().problem).toBe("");
 });
 it("uses one bounded next page, resets cursor on refresh, and does not poll", async () => {
  const inspect = vi.fn(diagnosticAPI().inspect); const controller = new CompilerDiagnosticsController(diagnosticAPI({ inspect }));
  await controller.selectScope("global"); await controller.selectSession("closed-session"); await controller.load("next-page"); await controller.load();
  expect(inspect.mock.calls.map(([, query]) => query.cursor)).toEqual(["", "next-page", ""]);
  expect(inspect.mock.calls.every(([, query]) => query.limit === 32)).toBe(true); expect(controller.snapshot().page?.jobs).toHaveLength(2);
  await Promise.resolve(); expect(inspect).toHaveBeenCalledTimes(3);
 });
 it("requires an explicit generation for selection, excludes it from other views, and fences changed IDs", async () => {
  const pending = deferred<CompilerDiagnostics>();
  const inspect = vi.fn(diagnosticAPI({ inspect: (_scope, query) => query.view === "selection" ? pending.promise : Promise.resolve({ ...diagnosticPage, view: query.view }) }).inspect);
  const controller = new CompilerDiagnosticsController(diagnosticAPI({ inspect })); await controller.selectScope("global"); await controller.selectSession("closed-session"); await controller.selectView("selection");
  expect(inspect).toHaveBeenCalledTimes(1); expect(controller.snapshot().generations).toEqual(["generation-1", "generation-2"]);
  controller.setGeneration("generation-1"); const loading = controller.load(); expect(inspect.mock.calls[1][1].generation_id).toBe("generation-1");
  controller.setGeneration("generation-2"); pending.resolve({ ...diagnosticPage, view: "selection" }); await loading; expect(controller.snapshot().page).toBeUndefined();
  await controller.selectView("jobs"); expect(inspect.mock.calls[2][1].generation_id).toBeUndefined();
 });
 it("drops a response for another exact scope and hides arbitrary transport errors", async () => {
  const controller = new CompilerDiagnosticsController(diagnosticAPI({ inspect: async () => ({ ...diagnosticPage, scope_key: "workspace:foreign" }) }));
  await controller.selectScope("global"); await controller.selectSession("closed-session"); expect(controller.snapshot().page).toBeUndefined(); expect(controller.snapshot().problem).toContain("unavailable");
  const other = new CompilerDiagnosticsController(diagnosticAPI({ inspect: async () => { throw new Error("SQLite /private/db password=secret"); } }));
  await other.selectScope("global"); await other.selectSession("closed-session"); expect(other.snapshot().problem).not.toMatch(/SQLite|private|password/);
 });
 it("retains an empty candidate page's continuation without claiming the inbox is empty", async () => {
  const controller = new CompilerDiagnosticsController(diagnosticAPI()); await controller.selectScope("global"); await controller.selectSession("closed-session"); await controller.selectView("candidates");
  expect(controller.snapshot().page?.candidates).toEqual([]); expect(controller.snapshot().page?.next_cursor).toBe("next-page");
 });
 it("clears earlier session metadata when current authorization is revoked", async () => {
  const controller = new CompilerDiagnosticsController(diagnosticAPI({ inspect: async () => { throw new CandidateReviewError("review_unauthorized", "scope no longer available"); } }));
  await controller.selectScope("global"); expect(controller.snapshot().sessions?.session_ids).toEqual(["closed-session"]); await controller.selectSession("closed-session");
  expect(controller.snapshot().page).toBeUndefined(); expect(controller.snapshot().sessions).toBeUndefined(); expect(controller.snapshot().generations).toEqual([]);
 });
 it("invalidates in-flight reads when the view unmounts", async () => {
  const pending = deferred<CompilerDiagnostics>(); const controller = new CompilerDiagnosticsController(diagnosticAPI({ inspect: () => pending.promise })); await controller.selectScope("global");
  const load = controller.selectSession("closed-session"); controller.invalidate(); pending.resolve(diagnosticPage); await load; expect(controller.snapshot().page).toBeUndefined();
 });
});
