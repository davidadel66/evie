import { DatabaseView } from "ui";
import * as React from "react";

// The app root (App.tsx) sets these classes; every Evie screen sits on
// them, so previews render on the same dark surface and text color.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);
// The Data hub gives this view a bounded, scrollable column; the drawer's
// percentage height and the canvas's flex-1 only resolve inside one.
const Column = ({ children }: { children: React.ReactNode }) => (
  <div className="flex h-[640px] flex-col overflow-hidden">{children}</div>
);

const col = (name: string, data_type: string, extra: Partial<{ nullable: boolean; primary_key: boolean; redacted: boolean }> = {}) =>
  ({ name, data_type, nullable: false, primary_key: false, ...extra });
const fk = (id: number, from_column: string, to_table: string, to_column = "id") =>
  ({ id, sequence: 0, from_column, to_table, to_column, on_update: "NO ACTION", on_delete: "CASCADE" });

// A slice of Evie's own SQLite schema (finance lives in its own database),
// two or three tables per layout group so four groups fit the canvas.
const tables = [
  { name: "sessions", kind: "table", row_access: "none", foreign_keys: [],
    columns: [col("id", "TEXT", { primary_key: true }), col("workspace_id", "TEXT", { nullable: true }), col("title", "TEXT"), col("created_at", "TEXT"), col("archived_at", "TEXT", { nullable: true })],
    indexes: [{ name: "sessions_workspace_created", unique: false, origin: "c", partial: false, columns: ["workspace_id", "created_at"] }] },
  { name: "events", kind: "table", row_access: "typed", foreign_keys: [fk(0, "session_id", "sessions")],
    columns: [col("id", "TEXT", { primary_key: true }), col("session_id", "TEXT"), col("seq", "INTEGER"), col("kind", "TEXT"), col("content", "TEXT", { redacted: true }), col("created_at", "TEXT")],
    indexes: [{ name: "events_session_seq", unique: true, origin: "c", partial: false, columns: ["session_id", "seq"] }] },
  { name: "workspaces", kind: "table", row_access: "records", foreign_keys: [],
    columns: [col("id", "TEXT", { primary_key: true }), col("name", "TEXT"), col("preset", "TEXT"), col("created_at", "TEXT")],
    indexes: [] },
  { name: "tasks", kind: "table", row_access: "records", foreign_keys: [fk(0, "workspace_id", "workspaces")],
    columns: [col("id", "TEXT", { primary_key: true }), col("workspace_id", "TEXT"), col("title", "TEXT"), col("state", "TEXT"), col("due_at", "TEXT", { nullable: true })],
    indexes: [{ name: "tasks_workspace_state", unique: false, origin: "c", partial: false, columns: ["workspace_id", "state"] }] },
  { name: "jobs", kind: "table", row_access: "records", foreign_keys: [],
    columns: [col("id", "INTEGER", { primary_key: true }), col("name", "TEXT"), col("schedule", "TEXT"), col("enabled", "INTEGER"), col("last_run_at", "TEXT", { nullable: true })],
    indexes: [{ name: "jobs_name", unique: true, origin: "c", partial: false, columns: ["name"] }] },
  { name: "job_runs", kind: "table", row_access: "records", foreign_keys: [fk(0, "job_id", "jobs")],
    columns: [col("id", "INTEGER", { primary_key: true }), col("job_id", "INTEGER"), col("state", "TEXT"), col("started_at", "TEXT"), col("finished_at", "TEXT", { nullable: true }), col("error", "TEXT", { nullable: true })],
    indexes: [{ name: "job_runs_job_started", unique: false, origin: "c", partial: false, columns: ["job_id", "started_at"] }] },
  { name: "workflow_runs", kind: "table", row_access: "records", foreign_keys: [fk(0, "workspace_id", "workspaces")],
    columns: [col("id", "TEXT", { primary_key: true }), col("workspace_id", "TEXT"), col("workflow", "TEXT"), col("state", "TEXT"), col("checkpoint", "INTEGER"), col("created_at", "TEXT")],
    indexes: [{ name: "workflow_runs_waiting", unique: false, origin: "c", partial: true, columns: ["workspace_id"] }] },
  { name: "semantic_entities", kind: "table", row_access: "typed", foreign_keys: [],
    columns: [col("id", "TEXT", { primary_key: true }), col("scope", "TEXT"), col("name", "TEXT"), col("kind", "TEXT"), col("valid_from", "TEXT"), col("valid_to", "TEXT", { nullable: true })],
    indexes: [{ name: "semantic_entities_scope_name", unique: false, origin: "c", partial: false, columns: ["scope", "name"] }] },
  { name: "semantic_facts", kind: "table", row_access: "typed", foreign_keys: [fk(0, "entity_id", "semantic_entities"), fk(1, "evidence_event_id", "events")],
    columns: [col("id", "TEXT", { primary_key: true }), col("entity_id", "TEXT"), col("predicate", "TEXT"), col("object", "TEXT", { redacted: true }), col("evidence_event_id", "TEXT", { nullable: true }), col("accepted_at", "TEXT")],
    indexes: [] },
];
const schema = { tables } as any;
const byName = (name: string) => tables.find((table) => table.name === name) as any;

const text = (value: string) => ({ kind: "text" as const, value });
const int = (value: string) => ({ kind: "integer" as const, value });
const nul = () => ({ kind: "null" as const, value: null });
const jobRows = {
  table: "job_runs", columns: byName("job_runs").columns, offset: 20, page_size: 20, total_rows: 212, next_offset: 40,
  rows: [
    [int("193"), int("3"), text("succeeded"), text("2026-10-07 13:00:04"), text("2026-10-07 13:00:06"), nul()],
    [int("192"), int("1"), text("succeeded"), text("2026-10-07 06:00:00"), text("2026-10-07 06:00:41"), nul()],
    [int("191"), int("3"), text("failed"), text("2026-10-06 13:00:04"), text("2026-10-06 13:00:09"), text("1 banks failed to sync")],
    [int("190"), int("2"), text("succeeded"), text("2026-10-06 09:00:00"), text("2026-10-06 09:00:12"), nul()],
    [int("189"), int("1"), text("succeeded"), text("2026-10-06 06:00:00"), text("2026-10-06 06:00:38"), nul()],
    [int("188"), int("3"), text("running"), text("2026-10-05 13:00:04"), nul(), nul()],
  ],
} as any;

const noop = () => {};
const callbacks = {
  onSearch: noop, onSelectTable: noop, onDrawerTab: noop, onCloseDrawer: noop, onToggleDrawer: noop,
  onZoom: noop, onRefresh: noop, onNextPage: noop, onPreviousPage: noop, onOpenMemory: noop,
};

export const SchemaMap = () => (
  <AppFrame><Column>
    <DatabaseView schema={schema} search="" drawerTab="rows" zoom={0.6} {...callbacks} />
  </Column></AppFrame>
);

export const JobRunsRecords = () => (
  <AppFrame><Column>
    <DatabaseView schema={schema} selectedTable={byName("job_runs")} rows={jobRows} search="" drawerTab="rows" zoom={0.6} {...callbacks} />
  </Column></AppFrame>
);

export const ProtectedEvents = () => (
  <AppFrame><Column>
    <DatabaseView schema={schema} selectedTable={byName("events")} search="" drawerTab="rows" zoom={0.6} {...callbacks} />
  </Column></AppFrame>
);

export const StructureExpanded = () => (
  <AppFrame><Column>
    <DatabaseView schema={schema} selectedTable={byName("semantic_facts")} search="semantic" drawerTab="structure" drawerExpanded={true} zoom={0.6} {...callbacks} />
  </Column></AppFrame>
);

export const ReadingSchema = () => (
  <AppFrame><Column>
    <DatabaseView search="" drawerTab="rows" zoom={0.6} {...callbacks} />
  </Column></AppFrame>
);

export const RowsProblem = () => (
  <AppFrame><Column>
    <DatabaseView schema={schema} selectedTable={byName("workflow_runs")} search="" drawerTab="rows" zoom={0.6}
      problem="Database records are unavailable: database is locked" {...callbacks} />
  </Column></AppFrame>
);
