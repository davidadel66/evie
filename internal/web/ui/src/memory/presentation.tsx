import { createContext, useContext, type ReactNode } from "react";
import type { ContextSessionSnapshot } from "../api/contextSessions";
import type { SemanticEntity } from "../api/memory";

type Presentation = { ownerName: string; preferredScope: string; names: Record<string, string> };
const MemoryPresentation = createContext<Presentation>({ ownerName: "You", preferredScope: "global", names: {} });

export function MemoryPresentationProvider({ snapshot, children }: { snapshot?: ContextSessionSnapshot; children: ReactNode }) {
  const names: Record<string, string> = {};
  for (const workspace of snapshot?.workspaces ?? []) names[`workspace:${workspace.id}`] = workspace.displayName;
  for (const project of snapshot?.projects ?? []) names[`project:${project.id}`] = project.displayName;
  for (const session of snapshot?.sessions ?? []) names[`session:${session.id}`] = session.title.trim() || "Untitled conversation";
  const active = snapshot?.activeScope;
  const preferredScope = active?.workspaceId ? `workspace:${active.workspaceId}` : active?.projectId ? `project:${active.projectId}` : "global";
  return <MemoryPresentation.Provider value={{ ownerName: snapshot?.ownerDisplayName || "You", preferredScope, names }}>{children}</MemoryPresentation.Provider>;
}

export function useMemoryPresentation() { return useContext(MemoryPresentation); }

export function entityLabel(entity: SemanticEntity, ownerName = "You") {
  return entity.anchor_kind === "owner" && entity.canonical_name === "owner" ? ownerName : entity.canonical_name;
}

export function scopeLabel(key: string, names: Record<string, string> = {}, fallback?: string) {
  if (key === "global") return "Global";
  if (names[key]) return names[key];
  if (fallback && fallback !== key) return fallback;
  if (key.startsWith("workspace:")) return "Workspace";
  if (key.startsWith("session:")) return "Conversation";
  if (key.startsWith("project:")) return "Project";
  return key;
}

type ScopeOption = { scope_key: string; label?: string; quarantined?: boolean };
export function MemoryScopeSelect({ scopes, value, onChange, label = "Memory scope" }: { scopes: ScopeOption[]; value: string; onChange: (scope: string) => void; label?: string }) {
  const { names } = useMemoryPresentation();
  const primary = scopes.filter((scope) => scope.scope_key === "global" || scope.scope_key.startsWith("workspace:"));
  const secondary = scopes.filter((scope) => scope.scope_key !== "global" && !scope.scope_key.startsWith("workspace:"));
  const option = (scope: ScopeOption) => <option key={scope.scope_key} value={scope.scope_key}>{scopeLabel(scope.scope_key, names, scope.label)}{scope.quarantined ? " (unavailable)" : ""}</option>;
  return <select aria-label={label} className="border-hair-input bg-app text-body min-w-40 max-w-64 rounded-md border px-3 py-2 text-sm" value={value} onChange={(event) => onChange(event.target.value)}>
    <option value="">Choose workspace</option>
    {value && !scopes.some((scope) => scope.scope_key === value) && <option value={value}>{scopeLabel(value, names)}</option>}
    {primary.map(option)}
    {secondary.length > 0 && <optgroup label="Other scopes">{secondary.map(option)}</optgroup>}
  </select>;
}

export function applicabilityLabel(key: string, names: Record<string,string> = {}) {
  if (key === "global") return "Everywhere";
  if (key.startsWith("session:")) return "This conversation";
  return `${key.startsWith("project:") ? "Project" : "Workspace"} · ${scopeLabel(key,names)}`;
}
