import type { MemoryGraphPath } from "../api/memoryEvidence";
import { memoryEvidenceAnchor } from "./memoryEvidenceAnchor";

export type GraphEvidenceItem = {
  reference: { id: string; claim_id?: string; graph_paths?: MemoryGraphPath[] };
  available: boolean;
  evidence?: { graph_paths?: MemoryGraphPath[]; sources: { event_id: string }[] };
};

export function GraphEvidencePath({ item, evidence }: { item: GraphEvidenceItem; evidence: GraphEvidenceItem[] }) {
  if (!item.available || !item.evidence || !item.reference.graph_paths?.length) return null;
  return <details className="text-muted-text mt-3 break-all text-xs leading-5">
    <summary className="cursor-pointer">Original relationship path</summary>
    <p className="mt-2">A relationship path does not establish a new accepted fact.</p>
    {item.reference.graph_paths.map((path) => {
      const support = path.claim_ids.map((id) => evidence.find((candidate) => candidate.reference.claim_id === id && candidate.available && (candidate.evidence?.sources.length ?? 0) > 0));
      const current = item.evidence?.graph_paths?.some((candidate) => candidate.anchor_entity_id === path.anchor_entity_id && candidate.claim_ids.length === path.claim_ids.length && candidate.claim_ids.every((id, index) => id === path.claim_ids[index]));
      return <div key={`${path.anchor_entity_id}:${path.claim_ids.join(":")}`} className="mt-3">
        <p>Anchor: {path.anchor_entity_id}</p>
        <p>{current && support.every(Boolean) ? "Current source support available" : "Current path support unavailable"}</p>
        <ol className="mt-2 ml-4 list-decimal">
          {path.claim_ids.map((id, index) => {
            const source = support[index];
            return <li key={id} className="mt-1">
              {source ? <><a className="text-teal underline" href={`#${encodeURIComponent(memoryEvidenceAnchor(source.reference.id))}`}>{id}</a><span> · Source: {source.evidence?.sources.map((entry) => entry.event_id).join(", ")}</span></> : <>{id} · Source unavailable</>}
            </li>;
          })}
        </ol>
      </div>;
    })}
  </details>;
}
