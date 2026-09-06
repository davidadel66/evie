import { describe, expect, it } from "vitest";
import type { DatabaseTable } from "../api/database";
import { connectedTableNames, layoutDatabaseSchema } from "./schemaLayout";

const column = (name: string, primaryKey = false) => ({
  name, data_type: "TEXT", nullable: false, primary_key: primaryKey,
});

const tables: DatabaseTable[] = [
  { name: "sessions", kind: "table", row_access: "none", columns: [column("id", true)], foreign_keys: [], indexes: [] },
  {
    name: "events", kind: "table", row_access: "typed", columns: [column("id", true), column("session_id")], indexes: [],
    foreign_keys: [{ id: 0, sequence: 0, from_column: "session_id", to_table: "sessions", to_column: "id", on_update: "NO ACTION", on_delete: "NO ACTION" }],
  },
  { name: "jobs", kind: "table", row_access: "records", columns: [column("id", true)], foreign_keys: [], indexes: [] },
];

describe("database schema layout", () => {
  it("places every table deterministically without overlap and finds a selected component", () => {
    const first = layoutDatabaseSchema(tables);
    const second = layoutDatabaseSchema([...tables].reverse());

    expect(second.nodes).toEqual(first.nodes);
    expect(first.edges).toHaveLength(1);
    for (let left = 0; left < first.nodes.length; left++) {
      for (let right = left + 1; right < first.nodes.length; right++) {
        const a = first.nodes[left];
        const b = first.nodes[right];
        expect(a.x + a.width <= b.x || b.x + b.width <= a.x || a.y + a.height <= b.y || b.y + b.height <= a.y).toBe(true);
      }
    }
    expect([...connectedTableNames(tables, "events")].sort()).toEqual(["events", "sessions"]);
    expect([...connectedTableNames(tables, "jobs")]).toEqual(["jobs"]);
  });
});
