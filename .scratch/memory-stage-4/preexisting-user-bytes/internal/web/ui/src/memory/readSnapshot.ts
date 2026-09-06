import type { ExactReadMetadata, MemoryTimeFilter } from "../api/memory";

export function pinMemoryRead(filter: MemoryTimeFilter, now = new Date().toISOString()): MemoryTimeFilter {
  return {
    history: filter.history,
    validAt: filter.validAt || now,
    asKnownAt: filter.asKnownAt || now,
  };
}

export function metadataTimeFilter(metadata: ExactReadMetadata, history = false): MemoryTimeFilter {
  return {
    history,
    validAt: metadata.valid_at,
    asKnownAt: metadata.as_known_at,
  };
}

export function sameMemorySnapshot(left: ExactReadMetadata, right: ExactReadMetadata): boolean {
  return snapshotKey(left) === snapshotKey(right);
}

function snapshotKey(metadata: ExactReadMetadata) {
  const revisions = [...metadata.scope_revisions]
    .sort((left, right) => left.scope_key.localeCompare(right.scope_key))
    .map((revision) => `${revision.scope_key}:${revision.revision}`)
    .join("|");
  return `${metadata.selected_scope ?? ""}\n${metadata.valid_at}\n${metadata.as_known_at}\n${revisions}`;
}
