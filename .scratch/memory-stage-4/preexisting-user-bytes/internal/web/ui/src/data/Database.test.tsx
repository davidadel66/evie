import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { DatabaseRows, DatabaseSchema } from "../api/database";
import { DatabaseView } from "./Database";

const schema: DatabaseSchema = {
  tables: [
    {
      name: "sessions", kind: "table", row_access: "none", indexes: [], foreign_keys: [],
      columns: [{ name: "id", data_type: "TEXT", nullable: false, primary_key: true }],
    },
    {
      name: "events", kind: "table", row_access: "typed", indexes: [],
      columns: [
        { name: "id", data_type: "TEXT", nullable: false, primary_key: true },
        { name: "session_id", data_type: "TEXT", nullable: false, primary_key: false },
        { name: "content", data_type: "TEXT", nullable: false, primary_key: false, redacted: true },
      ],
      foreign_keys: [{ id: 0, sequence: 0, from_column: "session_id", to_table: "sessions", to_column: "id", on_update: "NO ACTION", on_delete: "NO ACTION" }],
    },
    {
      name: "jobs", kind: "table", row_access: "records", foreign_keys: [],
      columns: [{ name: "id", data_type: "INTEGER", nullable: false, primary_key: true }, { name: "name", data_type: "TEXT", nullable: false, primary_key: false }],
      indexes: [{ name: "jobs_name", unique: true, origin: "c", partial: false, columns: ["name"] }],
    },
  ],
};

const rows: DatabaseRows = {
  table: "jobs", columns: schema.tables[2].columns,
  rows: [[{ kind: "integer", value: "1" }, { kind: "text", value: "morning-review" }]],
  offset: 0, page_size: 20, total_rows: 1,
};

const callbacks = {
  onSearch: () => undefined,
  onSelectTable: () => undefined,
  onDrawerTab: () => undefined,
  onCloseDrawer: () => undefined,
  onToggleDrawer: () => undefined,
  onZoom: () => undefined,
  onRefresh: () => undefined,
  onNextPage: () => undefined,
  onPreviousPage: () => undefined,
};

describe("DatabaseView", () => {
  it("renders live schema relationships and explains protected episodic records", () => {
    const html = renderToStaticMarkup(
      <DatabaseView schema={schema} selectedTable={schema.tables[1]} drawerTab="rows" search="" zoom={0.8} {...callbacks} />,
    );
    for (const text of ["Schema map", "events", "sessions", "Episodic events", "session_id", "sessions.id", "Protected by typed view"]) expect(html).toContain(text);
    expect(html).toContain('aria-label="events.session_id references sessions.id"');
    expect(html).not.toContain("textarea");
    expect(html).not.toContain("Run query");
  });

  it("shows allowlisted records in the expandable table drawer", () => {
    const html = renderToStaticMarkup(
      <DatabaseView schema={schema} selectedTable={schema.tables[2]} rows={rows} drawerTab="rows" search="" zoom={0.8} {...callbacks} />,
    );
    for (const text of ["1 record", "morning-review", "Rows", "Structure", "Indexes", "Expand table drawer"]) expect(html).toContain(text);
  });

  it("filters the table finder without deleting relationships from the map", () => {
    const html = renderToStaticMarkup(
      <DatabaseView schema={schema} selectedTable={schema.tables[1]} drawerTab="structure" search="events" zoom={0.8} {...callbacks} />,
    );

    expect(html).toContain('aria-label="events.session_id references sessions.id"');
    expect(html).toContain(">sessions<");
  });
});
