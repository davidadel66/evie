import { DataHubView, DatabaseView, SpendingAccountsView, UsageView } from "ui";
import * as React from "react";

// The app root (App.tsx) sets these classes; every Evie screen sits on
// them, so previews render on the same dark surface and text color.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);
// The shell gives the hub a bounded column; its min-h-0 / flex-1 panes
// only resolve inside one.
const Column = ({ children }: { children: React.ReactNode }) => (
  <div className="flex h-[640px] flex-col overflow-hidden">{children}</div>
);
const noop = () => {};

// Database pane: a small slice of Evie's SQLite schema.
const col = (name: string, data_type: string, extra: Partial<{ nullable: boolean; primary_key: boolean; redacted: boolean }> = {}) =>
  ({ name, data_type, nullable: false, primary_key: false, ...extra });
const schema = { tables: [
  { name: "sessions", kind: "table", row_access: "none", foreign_keys: [], indexes: [],
    columns: [col("id", "TEXT", { primary_key: true }), col("workspace_id", "TEXT", { nullable: true }), col("title", "TEXT"), col("created_at", "TEXT")] },
  { name: "events", kind: "table", row_access: "typed", indexes: [],
    foreign_keys: [{ id: 0, sequence: 0, from_column: "session_id", to_table: "sessions", to_column: "id", on_update: "NO ACTION", on_delete: "CASCADE" }],
    columns: [col("id", "TEXT", { primary_key: true }), col("session_id", "TEXT"), col("kind", "TEXT"), col("content", "TEXT", { redacted: true })] },
  { name: "workspaces", kind: "table", row_access: "records", foreign_keys: [], indexes: [],
    columns: [col("id", "TEXT", { primary_key: true }), col("name", "TEXT"), col("preset", "TEXT")] },
  { name: "jobs", kind: "table", row_access: "records", foreign_keys: [], indexes: [],
    columns: [col("id", "INTEGER", { primary_key: true }), col("name", "TEXT"), col("schedule", "TEXT")] },
  { name: "semantic_facts", kind: "table", row_access: "typed", foreign_keys: [], indexes: [],
    columns: [col("id", "TEXT", { primary_key: true }), col("entity_id", "TEXT"), col("predicate", "TEXT"), col("object", "TEXT", { redacted: true })] },
] } as any;
const database = <DatabaseView schema={schema} search="" drawerTab="rows" zoom={0.6}
  onSearch={noop} onSelectTable={noop} onDrawerTab={noop} onCloseDrawer={noop} onToggleDrawer={noop} onZoom={noop} onRefresh={noop} onNextPage={noop} onPreviousPage={noop} onOpenMemory={noop} />;

// Memory pane: the Memory container is not in the bundle (it fetches), so
// the pane is sketched with the same tokens the real one uses.
const memory = <div className="p-5 sm:p-7">
  <h2 className="text-ink text-base font-medium">Memory</h2>
  <p className="text-muted-text mt-1 text-xs">Scoped accepted knowledge · 1,284 facts across 212 entities</p>
  <div className="divide-hair mt-5 divide-y">
    {[
      ["David", "banks with", "Citibank Online, American Express, PNC", "finance"],
      ["finance-daily", "runs at", "09:00 America/Detroit", "finance"],
      ["Evie", "serves on", "evie.local via launchd", "runtime"],
    ].map(([subject, predicate, object, scope]) => <div key={subject + predicate} className="flex flex-wrap items-baseline gap-x-3 gap-y-1 py-3 text-xs">
      <span className="text-body font-medium">{subject}</span><span className="text-muted-text">{predicate}</span><span className="text-body">{object}</span>
      <span className="text-ghost ml-auto font-mono text-[10px]">{scope}</span>
    </div>)}
  </div>
</div>;

// Usage pane: thirty days of recorded conversation tokens.
const period = { from: "2026-09-08", to: "2026-10-08", timezone: "America/Detroit" };
const counters = (calls: number, input: number, cached: number, output: number) => ({
  calls, measuredCalls: calls,
  input: { value: String(input), reported: calls }, cached: { value: String(cached), reported: calls },
  output: { value: String(output), reported: calls }, total: { value: String(input + output), reported: calls },
  reasoning: { value: String(Math.round(output * 0.28)), reported: calls }, cacheWrite: { value: String(Math.round(cached * 0.12)), reported: calls },
  cacheInput: String(input), cacheCalls: calls, cachePercent: input ? Math.round(cached / input * 1000) / 10 : null, anomalies: 0,
});
const weight = [11, 14, 9, 17, 12, 4, 2, 13, 19, 8, 15, 10, 3, 0, 16, 12, 18, 9, 14, 5, 2, 12, 21, 11, 16, 13, 4, 0, 15, 20];
const daily = weight.map((w, index) => {
  const date = new Date(Date.UTC(2026, 8, 8 + index)).toISOString().slice(0, 10);
  const calls = w * 3; const input = calls * 9100;
  return { date, metrics: counters(calls, input, Math.round(input * 0.71), calls * 640) };
});
const usageReport = { period, sources: [{
  id: "evie-conversation", kind: "evie", label: "Evie conversations", state: "ready", collectedAt: "2026-10-08T12:41:07Z",
  coverage: "Recorded conversation responses through OpenRouter", account: null,
  summary: { metrics: counters(904, 7962500, 5618200, 572200), daily, firstObserved: "2026-09-08T11:02:13Z", lastObserved: "2026-10-07T22:14:50Z",
    models: [{ name: "anthropic/claude-sonnet-4.5", metrics: counters(612, 5700400, 4120000, 402100) }, { name: "openai/gpt-5", metrics: counters(292, 2262100, 1498200, 170100) }] },
}] } as any;
const usage = <UsageView report={usageReport} draft={period} selected="evie-conversation" loading={false} onDraft={noop} onApply={noop} onRefresh={noop} onSelect={noop} />;

// Spending pane: the connected-accounts section that heads the Spending source.
const snapshot = { linkAvailable: true, institutions: [
  { id: "ins_citi", name: "Citibank Online", accountsKnown: true, accountsUpdatedAt: "2026-10-07T13:00:05Z", accounts: [
    { id: "a1", name: "Citi Priority Checking", mask: "4418", type: "depository", subtype: "checking" },
    { id: "a2", name: "Citi Double Cash", mask: "1164", type: "credit", subtype: "credit_card" } ] },
  { id: "ins_amex", name: "American Express", accountsKnown: true, accountsUpdatedAt: "2026-10-07T13:00:06Z", accounts: [
    { id: "a3", name: "American Express Gold Card", mask: "81009", type: "credit", subtype: "credit_card" } ] },
  { id: "ins_pnc", name: "PNC", accountsKnown: false, accountsUpdatedAt: null, accounts: [] },
] };
const spending = <div className="min-h-0 flex-1 overflow-y-auto p-5 sm:p-7">
  <SpendingAccountsView snapshot={snapshot} loading={false} onReload={noop} onRefresh={noop} onAdd={noop} onCancel={noop} />
  <p className="text-muted-text text-xs">Overview, cash flow and transactions for October follow below.</p>
</div>;

export const DatabaseSource = () => (
  <AppFrame><Column>
    <DataHubView source="database" onSource={noop} database={database} memory={memory} usage={usage} spending={spending} />
  </Column></AppFrame>
);

export const MemorySource = () => (
  <AppFrame><Column>
    <DataHubView source="memory" onSource={noop} database={database} memory={memory} usage={usage} spending={spending} />
  </Column></AppFrame>
);

export const UsageSource = () => (
  <AppFrame><Column>
    <DataHubView source="usage" onSource={noop} database={database} memory={memory} usage={usage} spending={spending} />
  </Column></AppFrame>
);

export const SpendingSource = () => (
  <AppFrame><Column>
    <DataHubView source="spending" onSource={noop} database={database} memory={memory} usage={usage} spending={spending} />
  </Column></AppFrame>
);
