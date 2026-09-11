import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryEvidenceView } from "./MemoryEvidence";
import type { MemoryEvidenceReceipt } from "../api/memoryEvidence";

test("inspection shows the original discovery generation without rerunning retrieval", () => {
  const receipt: MemoryEvidenceReceipt = { sessionId: "reader", snapshotId: "original-request", version: "memory-retrieval-v2", status: "success", evidence: [{
    reference: { id: "claim:original", claim_id: "original", kind: "accepted_memory", scope_key: "global", status: "active", as_known_at: "2026-09-10T00:00:00Z", valid_at: "2026-09-10T00:00:00Z", paths: ["dense", "exact"], retrieval_generation: "original-minilm-generation", sources: [] },
    available: true, current_status: "active", evidence: { text: "Original accepted preference.", sources: [] },
  }] };
  const html = renderToStaticMarkup(<MemoryEvidenceView receipt={receipt} />);
  expect(html).toContain("Original discovery details");
  expect(html).toContain("original-minilm-generation");
  expect(html).toContain("dense, exact");
  expect(html).toContain("Original accepted preference.");
  const restricted = { ...receipt, evidence: receipt.evidence.map((item) => ({ ...item, available: false })) };
  const unavailable = renderToStaticMarkup(<MemoryEvidenceView receipt={restricted} />);
  expect(unavailable).toContain("Source unavailable under current access.");
  expect(unavailable).not.toContain("Original accepted preference.");
  expect(unavailable).not.toContain("original-minilm-generation");
});
