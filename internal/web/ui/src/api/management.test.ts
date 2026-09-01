import { afterEach, describe, expect, it, vi } from "vitest";
import { validatePreset } from "./management";

describe("validatePreset", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("posts the requested preset and returns aggregate invalid diagnostics", async () => {
    const report = {
      id: "standard", version: "sha256:abc", valid: false, immutable: true,
      requiredCapabilities: [], optionalCapabilities: [],
      errors: ["missing finance.sync", "missing web.search"], warnings: ["optional reports missing"],
    };
    const fetchMock = vi.fn(async () => new Response(JSON.stringify(report), {
      status: 422,
      headers: { "Content-Type": "application/json" },
    }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(validatePreset("standard")).resolves.toEqual(report);
    expect(fetchMock).toHaveBeenCalledWith("/api/presets/validate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id: "standard" }),
    });
  });

  it("surfaces transport errors", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: "offline" }), {
      status: 503,
      headers: { "Content-Type": "application/json" },
    })));
    await expect(validatePreset("standard")).rejects.toThrow("offline");
  });
});
