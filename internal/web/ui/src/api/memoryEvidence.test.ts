import { afterEach, expect, it, vi } from "vitest";
import { inspectMemoryEvidence } from "./memoryEvidence";

afterEach(() => vi.unstubAllGlobals());

it("inspects the selected request with no-store and rejects a different conversation response", async () => {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ sessionId: "session-2", snapshotId: "request-1", status: "success", evidence: [] }) });
  vi.stubGlobal("fetch", fetchMock);
  await expect(inspectMemoryEvidence("session-1", "request-1")).rejects.toThrow("selected conversation changed");
  expect(fetchMock).toHaveBeenCalledWith("/api/memory/evidence", expect.objectContaining({ method: "POST", cache: "no-store", body: JSON.stringify({ sessionId: "session-1", snapshotId: "request-1" }) }));
});
