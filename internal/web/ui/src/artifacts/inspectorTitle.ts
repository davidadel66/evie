import type { InspectorTarget } from "./Panel";
import type { SemanticObjectInspection } from "../api/memory";

export function targetTitle(target: InspectorTarget) {
  if (target.kind === "repository-instructions") return target.turnId ? "Turn instructions" : "Repository instructions";
  if (target.kind === "tool") return "Tool activity";
  if (target.kind === "file") return target.file.path.split("/").filter(Boolean).pop() ?? target.file.path;
  if (target.kind === "memory") return memoryTitle(target.detail);
  if (target.kind === "memory-evidence") return "Memory inspection";
  if (target.kind === "workspace") return target.workspace.displayName;
  if (target.kind === "scope") return target.scope.displayName;
  if (target.kind === "file-diff") return target.path;
  if (target.kind === "data") return "Data";
  return "Inspector";
}

export function memoryTitle(detail: SemanticObjectInspection) {
  if (detail.entity) return detail.entity.canonical_name;
  if (detail.claim) return `${detail.claim.predicate.label}: ${claimObject(detail.claim)}`;
  return `${detail.object_kind} ${detail.object_id}`;
}

export function claimObject(claim: NonNullable<SemanticObjectInspection["claim"]>) {
  if (claim.object.entity_id) return claim.object.entity_id;
  return claim.object.literal?.value ?? "Unknown";
}
