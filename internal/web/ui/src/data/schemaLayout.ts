import type { DatabaseForeignKey, DatabaseTable } from "../api/database";

export type DatabaseGroup = "Conversations" | "Work" | "Runtime" | "Memory" | "System";

export type DatabaseLayoutNode = {
  table: DatabaseTable;
  group: DatabaseGroup;
  x: number;
  y: number;
  width: number;
  height: number;
};

export type DatabaseLayoutEdge = {
  key: string;
  from: DatabaseLayoutNode;
  to: DatabaseLayoutNode;
  foreignKey: DatabaseForeignKey;
};

export type DatabaseLayout = {
  nodes: DatabaseLayoutNode[];
  edges: DatabaseLayoutEdge[];
  width: number;
  height: number;
  groups: { name: DatabaseGroup; x: number }[];
};

const groupOrder: DatabaseGroup[] = ["Conversations", "Work", "Runtime", "Memory", "System"];
const nodeWidth = 194;
const nodeHeight = 92;
const columnGap = 44;
const rowGap = 18;
const insetX = 28;
const insetY = 54;

export function databaseGroup(table: string): DatabaseGroup {
  if (table === "events" || table === "sessions" || table.startsWith("session_")) return "Conversations";
  if (table === "tasks" || table.startsWith("task_") || table === "projects" || table === "workspaces") return "Work";
  if (table.startsWith("semantic_")) return "Memory";
  if (table === "jobs" || table === "job_runs" || table.startsWith("plugin_") || table.startsWith("workflow_")) return "Runtime";
  return "System";
}

export function layoutDatabaseSchema(input: DatabaseTable[]): DatabaseLayout {
  const tables = [...input].sort((left, right) => {
    const groupDelta = groupOrder.indexOf(databaseGroup(left.name)) - groupOrder.indexOf(databaseGroup(right.name));
    return groupDelta || left.name.localeCompare(right.name);
  });
  const populatedGroups = groupOrder.filter((group) => tables.some((table) => databaseGroup(table.name) === group));
  const groups = populatedGroups.map((name, index) => ({ name, x: insetX + index * (nodeWidth + columnGap) }));
  const groupCounts = new Map<DatabaseGroup, number>();
  const nodes = tables.map((table): DatabaseLayoutNode => {
    const group = databaseGroup(table.name);
    const row = groupCounts.get(group) ?? 0;
    groupCounts.set(group, row + 1);
    return {
      table,
      group,
      x: groups.find((item) => item.name === group)?.x ?? insetX,
      y: insetY + row * (nodeHeight + rowGap),
      width: nodeWidth,
      height: nodeHeight,
    };
  });
  const byName = new Map(nodes.map((node) => [node.table.name, node]));
  const edges: DatabaseLayoutEdge[] = [];
  for (const from of nodes) {
    for (const foreignKey of from.table.foreign_keys) {
      const to = byName.get(foreignKey.to_table);
      if (!to) continue;
      edges.push({
        key: `${from.table.name}:${foreignKey.id}:${foreignKey.sequence}:${foreignKey.from_column}`,
        from,
        to,
        foreignKey,
      });
    }
  }
  const widestX = nodes.reduce((maximum, node) => Math.max(maximum, node.x + node.width), 0);
  const tallestY = nodes.reduce((maximum, node) => Math.max(maximum, node.y + node.height), 0);
  return {
    nodes,
    edges,
    groups,
    width: Math.max(720, widestX + insetX),
    height: Math.max(440, tallestY + insetY),
  };
}

export function connectedTableNames(tables: DatabaseTable[], selected: string): Set<string> {
  const adjacency = new Map<string, Set<string>>();
  for (const table of tables) adjacency.set(table.name, new Set());
  for (const table of tables) {
    for (const foreignKey of table.foreign_keys) {
      if (!adjacency.has(foreignKey.to_table)) continue;
      adjacency.get(table.name)?.add(foreignKey.to_table);
      adjacency.get(foreignKey.to_table)?.add(table.name);
    }
  }
  if (!adjacency.has(selected)) return new Set();
  const connected = new Set<string>([selected]);
  const pending = [selected];
  while (pending.length > 0) {
    const current = pending.shift()!;
    for (const neighbor of adjacency.get(current) ?? []) {
      if (connected.has(neighbor)) continue;
      connected.add(neighbor);
      pending.push(neighbor);
    }
  }
  return connected;
}
