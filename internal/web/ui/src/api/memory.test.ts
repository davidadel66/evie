import { afterEach, describe, expect, it, vi } from "vitest";
import { inspectMemoryObject, listMemoryObjects, listMemoryScopes } from "./memory";

describe("Semantic Memory API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("posts explicit read-only scope, page, temporal, and detail queries", async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ metadata: {}, scopes: [], objects: [] }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await listMemoryScopes();
    await listMemoryObjects({ scopeKey: "workspace:one", kinds: ["claim"], pageSize: 20, cursor: "opaque", history: true, validAt: "2026-09-01T00:00:00Z" });
    await inspectMemoryObject("workspace:one", "claim", "claim-1", { history: true, asKnownAt: "2026-09-02T00:00:00Z" });

    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/memory/scopes", expect.objectContaining({ method: "POST", body: "{}" }));
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/memory/objects", expect.objectContaining({ body: JSON.stringify({ scopeKey: "workspace:one", kinds: ["claim"], pageSize: 20, cursor: "opaque", history: true, validAt: "2026-09-01T00:00:00Z" }) }));
    expect(fetchMock).toHaveBeenNthCalledWith(3, "/api/memory/inspect", expect.objectContaining({ body: JSON.stringify({ scopeKey: "workspace:one", kind: "claim", id: "claim-1", history: true, asKnownAt: "2026-09-02T00:00:00Z" }) }));
  });
});
