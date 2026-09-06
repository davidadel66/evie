import { afterEach, describe, expect, it, vi } from "vitest";
import { inspectDatabaseRows, inspectDatabaseSchema } from "./database";

describe("Database inspection API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("posts only typed schema and bounded row queries", async () => {
    const responses = [{ tables: [] }, { table: "jobs", columns: [], rows: [], offset: 0, page_size: 20, total_rows: 0 }];
    const fetchMock = vi.fn(async () => new Response(JSON.stringify(responses.shift()), {
      status: 200, headers: { "Content-Type": "application/json" },
    }));
    vi.stubGlobal("fetch", fetchMock);

    await inspectDatabaseSchema();
    await inspectDatabaseRows({ table: "jobs", pageSize: 20, offset: 0 });

    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/data/database/schema", expect.objectContaining({ method: "POST", body: "{}" }));
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/data/database/rows", expect.objectContaining({
      method: "POST", body: JSON.stringify({ table: "jobs", pageSize: 20, offset: 0 }),
    }));
  });

  it("surfaces the server's safe error message", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: "Use the Memory view" }), {
      status: 403, headers: { "Content-Type": "application/json" },
    })));
    await expect(inspectDatabaseRows({ table: "semantic_claims" })).rejects.toThrow("Use the Memory view");
  });

  it("normalizes empty SQLite metadata arrays from older servers", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      tables: [{ name: "jobs", kind: "table", row_access: "records", columns: [], foreign_keys: null, indexes: null }],
    }), { status: 200, headers: { "Content-Type": "application/json" } })));

    const schema = await inspectDatabaseSchema();

    expect(schema.tables[0]).toMatchObject({ foreign_keys: [], indexes: [] });
  });
});
