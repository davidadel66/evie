import type { ContextScope, ContextSessionSelection } from "../api/contextSessions";

export function selectionForScope(scope: ContextScope): ContextSessionSelection | undefined {
  if (scope.kind === "workspace" && scope.workspaceId && scope.workspaceRevision) {
    return { workspaceId: scope.workspaceId, workspaceRevision: scope.workspaceRevision };
  }
  if (scope.kind === "project" && scope.projectId) return { projectId: scope.projectId };
  if (scope.kind === "unscoped") return { unscoped: true };
  return undefined;
}
