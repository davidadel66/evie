import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import { GraphEvidencePath, type GraphEvidenceItem } from "./GraphEvidencePath";
import { MemoryEvidenceView } from "./MemoryEvidence";

it("renders an ordered two-hop original path with links to each available supporting Claim's source", () => {
  const path = { anchor_entity_id: "entity-owner", claim_ids: ["relationship-claim", "preference-claim"] };
  const bridge: GraphEvidenceItem = { reference: { id: "claim:relationship-claim", claim_id: "relationship-claim" }, available: true, evidence: { sources: [{ event_id: "relationship-source" }] } };
  const terminal: GraphEvidenceItem = { reference: { id: "claim:preference-claim", claim_id: "preference-claim", graph_paths: [path] }, available: true, evidence: { graph_paths: [path], sources: [{ event_id: "preference-source" }] } };
  const html = renderToStaticMarkup(<GraphEvidencePath item={terminal} evidence={[terminal, bridge]} />);
  for (const value of ["Original relationship path", "entity-owner", "Current source support available", "relationship-source", "preference-source", 'href="#memory-evidence-claim%3Arelationship-claim"', 'href="#memory-evidence-claim%3Apreference-claim"']) expect(html).toContain(value);
  expect(html.indexOf("relationship-claim")).toBeLessThan(html.indexOf("preference-claim"));
  expect(html).toContain("A relationship path does not establish a new accepted fact");
  const receipt = { sessionId: "reader", snapshotId: "request", version: "retrieval-v2", status: "success", evidence: [terminal, bridge].map((item) => ({
    reference: { ...item.reference, kind: "accepted_memory", scope_key: "global", status: "active", as_known_at: "2026-09-10T00:00:00Z", valid_at: "2026-09-10T00:00:00Z", paths: ["graph_two_hop"], sources: [] },
    available: item.available, current_status: "active", evidence: { text: "Original accepted fact.", graph_paths: item.evidence?.graph_paths, sources: (item.evidence?.sources ?? []).map((source) => ({ ...source, session_id: "source-session", source_scope_key: "global", authority: "owner_statement", observed_at: "2026-09-09T00:00:00Z", evidence: "Original owner statement.", locator_kind: "whole", locator_value: "", evidence_sha256: "original-hash" })) },
  })) };
  const mounted = renderToStaticMarkup(<MemoryEvidenceView receipt={receipt} />);
  expect(mounted).toContain("Original relationship path");
  for (const id of ["relationship-claim", "preference-claim"]) {
    expect(mounted).toContain(`href="#memory-evidence-claim%3A${id}"`);
    expect(mounted).toContain(`id="memory-evidence-claim:${id}"`);
  }
});

it("keeps the original path but does not link or render inaccessible bridge source data", () => {
  const path = { anchor_entity_id: "entity-owner", claim_ids: ["relationship-claim", "preference-claim"] };
  const bridge: GraphEvidenceItem = { reference: { id: "claim:relationship-claim", claim_id: "relationship-claim" }, available: false, evidence: { sources: [{ event_id: "restricted-source-canary" }] } };
  const terminal: GraphEvidenceItem = { reference: { id: "claim:preference-claim", claim_id: "preference-claim", graph_paths: [path] }, available: true, evidence: { graph_paths: [], sources: [{ event_id: "preference-source" }] } };
  const html = renderToStaticMarkup(<GraphEvidencePath item={terminal} evidence={[bridge, terminal]} />);
  expect(html).toContain("Original relationship path");
  expect(html).toContain("Current path support unavailable");
  expect(html).toContain("Source unavailable");
  expect(html).toContain("relationship-claim");
  expect(html).not.toContain('href="#memory-evidence-claim%3Arelationship-claim"');
  expect(html).not.toContain("restricted-source-canary");
  expect(html).toContain('href="#memory-evidence-claim%3Apreference-claim"');
  expect(renderToStaticMarkup(<GraphEvidencePath item={{ ...terminal, available: false }} evidence={[bridge, terminal]} />)).toBe("");
});

it("omits path UI for ordinary direct evidence and preserves a supported single hop", () => {
  const direct: GraphEvidenceItem = { reference: { id: "claim:direct", claim_id: "direct" }, available: true, evidence: { sources: [{ event_id: "direct-source" }] } };
  expect(renderToStaticMarkup(<GraphEvidencePath item={direct} evidence={[direct]} />)).toBe("");
  const path = { anchor_entity_id: "entity-person", claim_ids: ["direct"] };
  const oneHop = { ...direct, reference: { ...direct.reference, graph_paths: [path] }, evidence: { sources: [{ event_id: "direct-source" }], graph_paths: [path] } };
  const html = renderToStaticMarkup(<GraphEvidencePath item={oneHop} evidence={[oneHop]} />);
  expect(html).toContain("entity-person");
  expect(html.match(/<li/g)).toHaveLength(1);
});
