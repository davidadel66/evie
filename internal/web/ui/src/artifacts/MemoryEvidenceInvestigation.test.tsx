import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryEvidenceView } from "./MemoryEvidence";
import type { MemoryEvidenceReceipt } from "../api/memoryEvidence";

test("an explicit knowledge date stays visible on a current evidence view", () => {
  const receipt: MemoryEvidenceReceipt = { sessionId: "reader", snapshotId: "request", version: "memory-retrieval-v2", status: "success", evidence: [{
    reference: { id: "claim:old", kind: "accepted_memory", scope_key: "global", status: "active", intent: "current", as_known_at: "2025-03-01T00:00:00Z", as_known_at_constrained: true, valid_at: "2026-09-10T00:00:00Z", paths: ["exact"], sources: [] },
    available: true, current_status: "active", evidence: { text: "The fact at the requested knowledge date.", sources: [] },
  }] };
  const html = renderToStaticMarkup(<MemoryEvidenceView receipt={receipt} />);
  expect(html).toContain("Read filters");
  expect(html).toContain("Known by:");
  expect(html).toContain("2025-03-01T00:00:00Z");
  expect(html).not.toContain("Valid at:");
});
