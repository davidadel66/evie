import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import {
  inspectDatabaseRows,
  inspectDatabaseSchema,
  type DatabaseRows,
  type DatabaseSchema,
  type DatabaseTable,
} from "../api/database";
import { ChevronLeft, ChevronRight, Cross, Database as DatabaseIcon, Expand, Layers } from "../ui/Icon";
import { connectedTableNames, databaseGroup, layoutDatabaseSchema, type DatabaseLayoutEdge } from "./schemaLayout";

export type DatabaseDrawerTab = "rows" | "structure" | "indexes";

export function Database({ onOpenMemory }: { onOpenMemory?: () => void } = {}) {
  const [schema, setSchema] = useState<DatabaseSchema>();
  const [selectedName, setSelectedName] = useState("");
  const [rows, setRows] = useState<DatabaseRows>();
  const [search, setSearch] = useState("");
  const [drawerTab, setDrawerTab] = useState<DatabaseDrawerTab>("rows");
  const [drawerExpanded, setDrawerExpanded] = useState(false);
  const [zoom, setZoom] = useState(0.8);
  const [problem, setProblem] = useState("");
  const schemaRequest = useRef(0);
  const rowsRequest = useRef(0);

  const loadSchema = useCallback(async () => {
    const request = ++schemaRequest.current;
    try {
      const next = await inspectDatabaseSchema();
      if (request !== schemaRequest.current) return;
      setSchema(next);
      setProblem("");
      setSelectedName((current) => next.tables.some((table) => table.name === current) ? current : "");
    } catch (error) {
      if (request === schemaRequest.current) setProblem(errorMessage(error, "Database schema is unavailable"));
    }
  }, []);

  const loadRows = useCallback(async (table: DatabaseTable, offset = 0) => {
    const request = ++rowsRequest.current;
    setRows(undefined);
    if (table.row_access !== "records") return;
    try {
      const next = await inspectDatabaseRows({ table: table.name, pageSize: 20, offset });
      if (request !== rowsRequest.current) return;
      setRows(next);
      setProblem("");
    } catch (error) {
      if (request === rowsRequest.current) setProblem(errorMessage(error, "Database records are unavailable"));
    }
  }, []);

  useEffect(() => {
    void loadSchema();
  }, [loadSchema]);

  const selectedTable = schema?.tables.find((table) => table.name === selectedName);
  const selectTable = (name: string) => {
    const table = schema?.tables.find((candidate) => candidate.name === name);
    if (!table) return;
    setSelectedName(name);
    setDrawerTab("rows");
    setDrawerExpanded(false);
    void loadRows(table);
  };

  return (
    <DatabaseView
      schema={schema}
      selectedTable={selectedTable}
      rows={rows}
      search={search}
      drawerTab={drawerTab}
      drawerExpanded={drawerExpanded}
      zoom={zoom}
      problem={problem}
      onSearch={setSearch}
      onSelectTable={selectTable}
      onDrawerTab={setDrawerTab}
      onCloseDrawer={() => { rowsRequest.current++; setSelectedName(""); setRows(undefined); }}
      onToggleDrawer={() => setDrawerExpanded((expanded) => !expanded)}
      onZoom={(next) => setZoom(Math.max(0.55, Math.min(1, next)))}
      onRefresh={() => { void loadSchema(); if (selectedTable) void loadRows(selectedTable, rows?.offset ?? 0); }}
      onNextPage={() => { if (selectedTable && rows?.next_offset !== undefined) void loadRows(selectedTable, rows.next_offset); }}
      onPreviousPage={() => { if (selectedTable && rows) void loadRows(selectedTable, Math.max(0, rows.offset - rows.page_size)); }}
      onOpenMemory={onOpenMemory}
    />
  );
}

export function DatabaseView({
  schema,
  selectedTable,
  rows,
  search,
  drawerTab,
  drawerExpanded = false,
  zoom,
  problem,
  onSearch,
  onSelectTable,
  onDrawerTab,
  onCloseDrawer,
  onToggleDrawer,
  onZoom,
  onRefresh,
  onNextPage,
  onPreviousPage,
  onOpenMemory,
}: {
  schema?: DatabaseSchema;
  selectedTable?: DatabaseTable;
  rows?: DatabaseRows;
  search: string;
  drawerTab: DatabaseDrawerTab;
  drawerExpanded?: boolean;
  zoom: number;
  problem?: string;
  onSearch: (value: string) => void;
  onSelectTable: (name: string) => void;
  onDrawerTab: (tab: DatabaseDrawerTab) => void;
  onCloseDrawer: () => void;
  onToggleDrawer: () => void;
  onZoom: (zoom: number) => void;
  onRefresh: () => void;
  onNextPage: () => void;
  onPreviousPage: () => void;
  onOpenMemory?: () => void;
}) {
  const filteredTables = useMemo(() => {
    const query = search.trim().toLowerCase();
    return schema?.tables.filter((table) => !query || table.name.toLowerCase().includes(query)) ?? [];
  }, [schema, search]);
  const groups = useMemo(() => {
    const grouped = new Map<string, DatabaseTable[]>();
    for (const table of filteredTables) {
      const group = databaseGroup(table.name);
      grouped.set(group, [...(grouped.get(group) ?? []), table]);
    }
    return [...grouped.entries()];
  }, [filteredTables]);

  return (
    <section aria-label="Database" className="flex min-h-0 flex-1 overflow-hidden">
      <aside className="border-hair bg-docked hidden w-[218px] flex-none flex-col border-r sm:flex">
        <div className="border-hair border-b p-3">
          <label className="text-fainter block text-[10px]">
            Find a table
            <input
              type="search"
              value={search}
              onChange={(event) => onSearch(event.target.value)}
              placeholder="Search schema"
              className="border-hair-input bg-app text-body placeholder:text-ghost focus:border-teal mt-1.5 w-full rounded-[5px] border px-2.5 py-2 text-[11px] outline-none"
            />
          </label>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto py-2">
          {groups.length === 0 && <p className="text-fainter px-3 py-4 text-[10.5px]">No matching tables.</p>}
          {groups.map(([group, tables]) => (
            <section key={group} aria-label={`${group} tables`} className="mb-3">
              <h3 className="text-ghost px-3 py-1 text-[9.5px] font-medium">{group}</h3>
              {tables.map((table) => (
                <button
                  type="button"
                  key={table.name}
                  aria-pressed={selectedTable?.name === table.name}
                  onClick={() => onSelectTable(table.name)}
                  className={`${selectedTable?.name === table.name ? "bg-selected text-body" : "text-faint hover:bg-hover hover:text-body"} flex w-full items-center gap-2 px-3 py-1.5 text-left font-mono text-[10.5px]`}
                >
                  <span className={`${table.row_access === "records" ? "bg-teal" : table.row_access === "typed" ? "bg-amber" : "bg-ghost"} h-1.5 w-1.5 rounded-full`} />
                  <span className="min-w-0 flex-1 truncate">{table.name}</span>
                  <span className="text-ghost text-[9px]">{table.columns.length}</span>
                </button>
              ))}
            </section>
          ))}
        </div>
        <div className="border-hair text-ghost border-t px-3 py-2.5 text-[9.5px] leading-4">
          Teal tables expose bounded records. Amber tables open through a typed domain view.
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <div className="border-hair flex h-[42px] flex-none items-center gap-2 border-b px-3 sm:px-4">
          <span className="text-faint"><Layers size={13} /></span>
          <span className="text-body text-[11.5px] font-medium">Schema map</span>
          <span className="text-ghost font-mono text-[9.5px]">{schema?.tables.length ?? 0} tables</span>
          <select
            aria-label="Choose database table"
            value={selectedTable?.name ?? ""}
            onChange={(event) => event.target.value && onSelectTable(event.target.value)}
            className="border-hair bg-app text-body ml-2 max-w-[180px] rounded border px-2 py-1 text-[10px] sm:hidden"
          >
            <option value="">Choose table…</option>
            {filteredTables.map((table) => <option key={table.name} value={table.name}>{table.name}</option>)}
          </select>
          <div className="flex-1" />
          <button type="button" aria-label="Zoom out" onClick={() => onZoom(zoom - 0.1)} className="text-faint hover:text-body rounded px-2 py-1 text-xs">−</button>
          <span className="text-ghost w-8 text-center font-mono text-[9px]">{Math.round(zoom * 100)}%</span>
          <button type="button" aria-label="Zoom in" onClick={() => onZoom(zoom + 0.1)} className="text-faint hover:text-body rounded px-2 py-1 text-xs">+</button>
          <button type="button" onClick={onRefresh} className="text-faint hover:text-body ml-1 text-[10px]">Refresh</button>
        </div>

        {problem && <div role="alert" className="border-danger-hair bg-danger-bg text-danger-ink border-b px-4 py-2 text-[11px]">{problem}</div>}
        <div className="relative flex min-h-0 flex-1 flex-col">
          <div className="database-canvas min-h-[180px] flex-1 overflow-auto bg-app">
            {schema ? (
              schema.tables.length > 0 ? <SchemaCanvas tables={schema.tables} selected={selectedTable?.name} zoom={zoom} onSelect={onSelectTable} /> :
                <EmptyCanvas title="No tables found" detail="Evie's physical schema is empty." />
            ) : <EmptyCanvas title="Reading the schema" detail="Loading tables, columns, keys, and indexes…" />}
          </div>
          {selectedTable && (
            <TableDrawer
              table={selectedTable}
              rows={rows}
              tab={drawerTab}
              expanded={drawerExpanded}
              onTab={onDrawerTab}
              onClose={onCloseDrawer}
              onToggle={onToggleDrawer}
              onNextPage={onNextPage}
              onPreviousPage={onPreviousPage}
              onOpenMemory={onOpenMemory}
            />
          )}
        </div>
      </div>
    </section>
  );
}

function SchemaCanvas({ tables, selected, zoom, onSelect }: { tables: DatabaseTable[]; selected?: string; zoom: number; onSelect: (name: string) => void }) {
  const layout = useMemo(() => layoutDatabaseSchema(tables), [tables]);
  const connected = useMemo(() => selected ? connectedTableNames(tables, selected) : new Set(tables.map((table) => table.name)), [selected, tables]);
  const scaled: CSSProperties = { width: layout.width * zoom, height: layout.height * zoom };
  const canvas: CSSProperties = { width: layout.width, height: layout.height, transform: `scale(${zoom})`, transformOrigin: "top left" };
  return (
    <div style={scaled} className="relative min-h-full min-w-full">
      <div style={canvas} className="absolute left-0 top-0">
        {layout.groups.map((group) => <div key={group.name} style={{ left: group.x, top: 22 }} className="text-ghost absolute text-[9.5px] font-medium">{group.name}</div>)}
        <svg aria-label="Database relationships" width={layout.width} height={layout.height} className="pointer-events-none absolute inset-0">
          <defs>
            <marker id="database-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="5" markerHeight="5" orient="auto-start-reverse">
              <path d="M0 0 8 4 0 8z" fill="currentColor" />
            </marker>
          </defs>
          {layout.edges.map((edge) => {
            const direct = !selected || edge.from.table.name === selected || edge.to.table.name === selected;
            const visible = !selected || (connected.has(edge.from.table.name) && connected.has(edge.to.table.name));
            return <path
              key={edge.key}
              aria-label={`${edge.from.table.name}.${edge.foreignKey.from_column} references ${edge.to.table.name}.${edge.foreignKey.to_column}`}
              d={databaseEdgePath(edge)}
              fill="none"
              stroke="currentColor"
              strokeWidth={direct ? 1.35 : 0.8}
              markerEnd="url(#database-arrow)"
              className={direct ? "text-teal" : visible ? "text-faint" : "text-hair-strong"}
              opacity={direct ? 0.8 : visible ? 0.38 : 0.12}
            />;
          })}
        </svg>
        {layout.nodes.map((node) => {
          const active = node.table.name === selected;
          const visible = connected.has(node.table.name);
          return (
            <button
              key={node.table.name}
              type="button"
              onClick={() => onSelect(node.table.name)}
              aria-pressed={active}
              style={{ left: node.x, top: node.y, width: node.width, height: node.height }}
              className={`${active ? "border-teal bg-selected shadow-[0_0_0_1px_var(--color-teal-hair)]" : "border-hair-strong bg-card hover:border-hair-input hover:bg-hover"} ${visible ? "opacity-100" : "opacity-35"} absolute overflow-hidden rounded-[7px] border text-left`}
            >
              <div className="border-hair flex h-8 items-center gap-2 border-b px-2.5">
                <DatabaseIcon size={11} className={active ? "text-teal" : "text-faint"} />
                <span className="text-body min-w-0 flex-1 truncate font-mono text-[10.5px] font-medium">{node.table.name}</span>
                <span className={`${node.table.row_access === "records" ? "text-teal" : node.table.row_access === "typed" ? "text-amber" : "text-ghost"} text-[8.5px]`}>{node.table.kind}</span>
              </div>
              <div className="px-2.5 py-1.5">
                {node.table.columns.slice(0, 3).map((column) => (
                  <div key={column.name} className="flex h-[15px] items-center gap-1.5 font-mono text-[8.5px]">
                    <span className={column.primary_key ? "text-amber" : "text-ghost"}>{column.primary_key ? "◆" : "·"}</span>
                    <span className="text-faint min-w-0 flex-1 truncate">{column.name}</span>
                    <span className="text-ghost truncate">{column.data_type || "any"}</span>
                  </div>
                ))}
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}

function TableDrawer({
  table, rows, tab, expanded, onTab, onClose, onToggle, onNextPage, onPreviousPage, onOpenMemory,
}: {
  table: DatabaseTable;
  rows?: DatabaseRows;
  tab: DatabaseDrawerTab;
  expanded: boolean;
  onTab: (tab: DatabaseDrawerTab) => void;
  onClose: () => void;
  onToggle: () => void;
  onNextPage: () => void;
  onPreviousPage: () => void;
  onOpenMemory?: () => void;
}) {
  return (
    <section aria-label={`${table.name} table drawer`} style={{ height: expanded ? "72%" : "44%" }} className="border-hair bg-docked flex min-h-[190px] flex-none flex-col border-t">
      <header className="border-hair flex h-[43px] flex-none items-center border-b px-3">
        <button type="button" aria-label={expanded ? "Collapse table drawer" : "Expand table drawer"} onClick={onToggle} className="text-faint hover:text-body mr-2 rounded p-1"><Expand size={12} /></button>
        <span className="text-body font-mono text-[11px] font-medium">{table.name}</span>
        <span className="text-ghost ml-2 text-[9.5px]">{table.columns.length} columns</span>
        <div role="tablist" aria-label="Table details" className="ml-5 flex self-stretch">
          {(["rows", "structure", "indexes"] as const).map((item) => (
            <button key={item} type="button" role="tab" aria-selected={tab === item} onClick={() => onTab(item)} className={`${tab === item ? "border-teal text-body" : "border-transparent text-faint hover:text-body"} border-b px-3 text-[10.5px]`}>{drawerTabLabel(item)}</button>
          ))}
        </div>
        <div className="flex-1" />
        {rows && <span className="text-ghost mr-3 font-mono text-[9px]">{rows.total_rows} {rows.total_rows === 1 ? "record" : "records"}</span>}
        <button type="button" aria-label="Close table drawer" onClick={onClose} className="text-faint hover:text-body rounded p-1"><Cross size={12} /></button>
      </header>
      <div className="min-h-0 flex-1 overflow-auto">
        {tab === "rows" && <RowsView table={table} rows={rows} onNextPage={onNextPage} onPreviousPage={onPreviousPage} onOpenMemory={onOpenMemory} />}
        {tab === "structure" && <StructureView table={table} />}
        {tab === "indexes" && <IndexesView table={table} />}
      </div>
    </section>
  );
}

function RowsView({ table, rows, onNextPage, onPreviousPage, onOpenMemory }: { table: DatabaseTable; rows?: DatabaseRows; onNextPage: () => void; onPreviousPage: () => void; onOpenMemory?: () => void }) {
  if (table.row_access === "typed") {
    const episodic = table.name === "events";
    const semantic = table.name.startsWith("semantic_");
    return <div className="flex min-h-full items-center justify-center p-7 text-center">
      <div className="max-w-[420px]">
        <span className="border-amber-hair text-amber inline-flex rounded border px-2 py-1 text-[9.5px]">Protected by typed view</span>
        <h3 className="text-body mt-3 text-sm font-medium">{episodic ? "Episodic events" : semantic ? "Semantic Memory" : "Scoped records"}</h3>
        <p className="text-fainter mt-2 text-xs leading-5">
          {episodic
            ? "Event identity and relationships live here. Evidence text stays behind Memory’s scoped provenance view because raw events may contain secrets."
            : "This projection is inspected through Memory so scope, time, lifecycle, and evidence rules stay intact."}
        </p>
        {semantic && onOpenMemory && <button type="button" onClick={onOpenMemory} className="text-teal hover:text-teal-hover mt-4 text-xs">Open Memory</button>}
      </div>
    </div>;
  }
  if (table.row_access === "none") {
    return <div className="text-fainter flex min-h-full items-center justify-center p-7 text-center text-xs">This internal table exposes schema only.</div>;
  }
  if (!rows) return <div className="text-fainter flex min-h-full items-center justify-center p-7 text-xs">Reading bounded records…</div>;
  if (rows.rows.length === 0) return <div className="text-fainter flex min-h-full items-center justify-center p-7 text-xs">No records in this table.</div>;
  return <div className="min-w-max">
    <table className="w-full border-collapse font-mono text-[10px]">
      <thead className="bg-docked sticky top-0 z-10">
        <tr>{rows.columns.map((column) => <th key={column.name} className="border-hair text-faint border-b border-r px-3 py-2 text-left font-medium">{column.name}</th>)}</tr>
      </thead>
      <tbody>{rows.rows.map((row, rowIndex) => <tr key={`${rows.offset}:${rowIndex}`} className="hover:bg-hover">
        {row.map((cell, columnIndex) => <td key={rows.columns[columnIndex]?.name ?? columnIndex} className="border-hair text-body max-w-[300px] truncate border-b border-r px-3 py-2" title={cell.value ?? undefined}>
          {cell.redacted ? <span className="text-amber">protected</span> : cell.kind === "null" ? <span className="text-ghost">NULL</span> : cell.value}
        </td>)}
      </tr>)}</tbody>
    </table>
    <div className="flex items-center justify-end gap-2 p-2">
      <button type="button" aria-label="Previous records" disabled={rows.offset === 0} onClick={onPreviousPage} className="border-hair text-faint hover:text-body rounded border p-1.5 disabled:opacity-30"><ChevronLeft size={11} /></button>
      <span className="text-ghost font-mono text-[9px]">{rows.offset + 1}–{rows.offset + rows.rows.length} of {rows.total_rows}</span>
      <button type="button" aria-label="Next records" disabled={rows.next_offset === undefined} onClick={onNextPage} className="border-hair text-faint hover:text-body rounded border p-1.5 disabled:opacity-30"><ChevronRight size={11} /></button>
    </div>
  </div>;
}

function StructureView({ table }: { table: DatabaseTable }) {
  return <div className="grid gap-5 p-4 lg:grid-cols-[minmax(0,1fr)_300px]">
    <div>
      <h3 className="text-body mb-2 text-[11px] font-medium">Columns</h3>
      <div className="border-hair overflow-hidden rounded border">
        {table.columns.map((column) => <div key={column.name} className="border-hair grid grid-cols-[minmax(120px,1fr)_120px_80px] gap-3 border-b px-3 py-2 font-mono text-[9.5px] last:border-0">
          <span className="text-body truncate">{column.name}{column.primary_key && <span className="text-amber ml-2">primary</span>}</span>
          <span className="text-faint truncate">{column.data_type || "any"}</span>
          <span className="text-ghost text-right">{column.nullable ? "nullable" : "required"}</span>
        </div>)}
      </div>
    </div>
    <div>
      <h3 className="text-body mb-2 text-[11px] font-medium">Foreign keys</h3>
      {table.foreign_keys.length === 0 ? <p className="text-fainter text-[10.5px]">No declared foreign keys.</p> : table.foreign_keys.map((foreignKey) => <div key={`${foreignKey.id}:${foreignKey.sequence}`} className="border-hair border-b py-2 font-mono text-[9.5px]">
        <span className="text-body">{foreignKey.from_column}</span><span className="text-ghost mx-2">→</span><span className="text-teal">{foreignKey.to_table}.{foreignKey.to_column}</span>
      </div>)}
    </div>
  </div>;
}

function IndexesView({ table }: { table: DatabaseTable }) {
  return <div className="p-4">
    {table.indexes.length === 0 ? <p className="text-fainter text-xs">No named indexes.</p> : <div className="border-hair overflow-hidden rounded border">
      {table.indexes.map((index) => <div key={index.name} className="border-hair grid grid-cols-[minmax(180px,1fr)_minmax(140px,1fr)_90px] gap-3 border-b px-3 py-2 font-mono text-[9.5px] last:border-0">
        <span className="text-body truncate">{index.name}</span>
        <span className="text-faint truncate">{index.columns.join(", ") || "expression"}</span>
        <span className="text-ghost text-right">{index.unique ? "unique" : index.partial ? "partial" : index.origin}</span>
      </div>)}
    </div>}
  </div>;
}

function EmptyCanvas({ title, detail }: { title: string; detail: string }) {
  return <div className="flex min-h-full items-center justify-center p-8 text-center">
    <div><DatabaseIcon size={18} className="text-ghost mx-auto" /><h3 className="text-body mt-3 text-xs font-medium">{title}</h3><p className="text-fainter mt-1 text-[10.5px]">{detail}</p></div>
  </div>;
}

function databaseEdgePath(edge: DatabaseLayoutEdge) {
  const fromX = edge.from.x > edge.to.x ? edge.from.x : edge.from.x + edge.from.width;
  const toX = edge.from.x > edge.to.x ? edge.to.x + edge.to.width : edge.to.x;
  const fromY = edge.from.y + edge.from.height / 2;
  const toY = edge.to.y + edge.to.height / 2;
  const bend = Math.max(36, Math.abs(toX - fromX) * 0.42);
  const direction = toX >= fromX ? 1 : -1;
  return `M ${fromX} ${fromY} C ${fromX + bend * direction} ${fromY}, ${toX - bend * direction} ${toY}, ${toX} ${toY}`;
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

function drawerTabLabel(tab: DatabaseDrawerTab) {
  return tab === "rows" ? "Rows" : tab === "structure" ? "Structure" : "Indexes";
}
