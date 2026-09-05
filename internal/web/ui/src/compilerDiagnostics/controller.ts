import { CandidateReviewError, type CandidateScopes } from "../api/candidateReview";
import type { CompilerDiagnostics, CompilerDiagnosticsAPI, DiagnosticSessions, DiagnosticView } from "../api/compilerDiagnostics";

export type DiagnosticsState = {
 scopes?: CandidateScopes; sessions?: DiagnosticSessions; page?: CompilerDiagnostics;
 scope: string; session: string; view: DiagnosticView; generation: string; generations: string[];
 scopesBusy: boolean; busy: boolean; problem: string;
};

// Every navigation/refresh replaces one bounded page. Request epochs prevent
// a late response from restoring a previous scope, session, view, or generation.
export class CompilerDiagnosticsController {
 private state: DiagnosticsState = { scope: "", session: "", view: "jobs", generation: "", generations: [], scopesBusy: false, busy: false, problem: "" };
 private listeners = new Set<() => void>();
 private epoch = 0;
 private scopesEpoch = 0;
 private api: CompilerDiagnosticsAPI;
 constructor(api: CompilerDiagnosticsAPI) { this.api = api; }
 snapshot = () => this.state;
 subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
 private set(patch: Partial<DiagnosticsState>) { this.state = { ...this.state, ...patch }; this.listeners.forEach((listener) => listener()); }
 invalidate() { this.epoch++; this.scopesEpoch++; }
 async loadScopes(cursor = "") {
  const epoch = ++this.scopesEpoch;
  this.set({ scopesBusy: true, problem: "" });
  try { const scopes = await this.api.scopes(cursor); if (epoch === this.scopesEpoch) this.set({ scopes, scopesBusy: false }); }
  catch { if (epoch === this.scopesEpoch) { this.epoch++; this.set({ scopes: undefined, sessions: undefined, page: undefined, generations: [], scopesBusy: false, busy: false, problem: "Memory scopes are unavailable. Refresh scopes to try again." }); } }
 }
 async selectScope(scope: string) {
  this.epoch++;
  this.set({ scope, session: "", generation: "", generations: [], sessions: undefined, page: undefined, busy: false, problem: "" });
  if (scope) await this.loadSessions();
 }
 async loadSessions(cursor = "") {
  const scope = this.state.scope; if (!scope) return;
  const epoch = ++this.epoch;
  this.set({ busy: true, session: "", sessions: undefined, page: undefined, generations: [], generation: "", problem: "" });
  try { const sessions = await this.api.sessions(scope, cursor); if (epoch === this.epoch) this.set({ sessions, busy: false }); }
  catch { if (epoch === this.epoch) this.set({ busy: false, problem: "Sessions are unavailable in this scope. Refresh scopes or sessions to try again." }); }
 }
 async selectSession(session: string) {
  this.epoch++;
  this.set({ session, generation: "", generations: [], page: undefined, busy: false, problem: "" });
  if (session) await this.load();
 }
 async selectView(view: DiagnosticView) {
  this.epoch++;
  this.set({ view, page: undefined, busy: false, problem: "" });
  await this.load();
 }
 setGeneration(generation: string) { this.epoch++; this.set({ generation, page: undefined, busy: false, problem: "" }); }
 async load(cursor = "") {
  const { scope, session, view, generation } = this.state;
  if (!scope || !session || (view === "selection" && !generation)) return;
  const epoch = ++this.epoch;
  this.set({ busy: true, page: undefined, problem: "" });
  try {
   const page = await this.api.inspect(scope, { session_id: session, view, ...(view === "selection" ? { generation_id: generation } : {}), limit: 32, cursor });
   if (epoch !== this.epoch) return;
   if (page.scope_key !== scope || page.session_id !== session || page.view !== view) throw new Error("mismatched diagnostic page");
   const metadata = [...page.jobs, ...page.activations, ...page.history, ...page.candidates, ...page.selections];
   this.set({ page, busy: false, ...(metadata.length ? { generations: [...new Set(metadata.map((item) => item.generation_id))].slice(0, 32) } : {}) });
  } catch (error) { if (epoch === this.epoch) this.set({ busy: false, page: undefined, generations: [], ...(error instanceof CandidateReviewError && ["review_unauthorized", "review_scope_quarantined", "source_ineligible"].includes(error.code) ? { sessions: undefined } : {}), problem: "Diagnostics are unavailable or the selection changed. Refresh this page; for an expired cursor, refresh starts again at the first page." }); }
 }
}
