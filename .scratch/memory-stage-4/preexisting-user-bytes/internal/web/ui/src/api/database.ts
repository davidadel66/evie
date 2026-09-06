export type DatabaseRowAccess = "records" | "typed" | "none";

export type DatabaseColumn = {
  name: string;
  data_type: string;
  nullable: boolean;
  primary_key: boolean;
  hidden?: boolean;
  redacted?: boolean;
};

export type DatabaseForeignKey = {
  id: number;
  sequence: number;
  from_column: string;
  to_table: string;
  to_column: string;
  on_update: string;
  on_delete: string;
};

export type DatabaseIndex = {
  name: string;
  unique: boolean;
  origin: string;
  partial: boolean;
  columns: string[];
};

export type DatabaseTable = {
  name: string;
  kind: "table" | "view";
  row_access: DatabaseRowAccess;
  columns: DatabaseColumn[];
  foreign_keys: DatabaseForeignKey[];
  indexes: DatabaseIndex[];
};

export type DatabaseSchema = { tables: DatabaseTable[] };

export type DatabaseCell = {
  kind: "null" | "integer" | "real" | "boolean" | "text" | "blob" | "redacted";
  value: string | null;
  redacted?: boolean;
  truncated?: boolean;
};

export type DatabaseRows = {
  table: string;
  columns: DatabaseColumn[];
  rows: DatabaseCell[][];
  offset: number;
  page_size: number;
  total_rows: number;
  next_offset?: number;
};

export type DatabaseRowsQuery = {
  table: string;
  pageSize?: number;
  offset?: number;
};

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const value = (await response.json()) as T & { error?: string };
  if (!response.ok) throw new Error(value.error ?? `Request failed (${response.status})`);
  return value;
}

export function inspectDatabaseSchema(): Promise<DatabaseSchema> {
  return postJSON<DatabaseSchema>("/api/data/database/schema", {}).then((schema) => ({
    tables: (schema.tables ?? []).map((table) => ({
      ...table,
      columns: table.columns ?? [],
      foreign_keys: table.foreign_keys ?? [],
      indexes: (table.indexes ?? []).map((index) => ({ ...index, columns: index.columns ?? [] })),
    })),
  }));
}

export function inspectDatabaseRows(query: DatabaseRowsQuery): Promise<DatabaseRows> {
  return postJSON<DatabaseRows>("/api/data/database/rows", query).then((rows) => ({
    ...rows,
    columns: rows.columns ?? [],
    rows: rows.rows ?? [],
  }));
}
