import { afterEach, describe, expect, it, vi } from "vitest";
import { inspectUsage, shiftDate } from "./usage";

describe("Usage API", () => {
  afterEach(() => vi.unstubAllGlobals());
  it("posts the exact half-open range with cancellation and preserves nullable string counters", async () => {
    const response = { period: {}, sources: [{ summary: { metrics: { input: { value: "9007199254740993" }, cached: { value: null } } } }] };
    const fetchMock = vi.fn(async () => new Response(JSON.stringify(response)));
    vi.stubGlobal("fetch", fetchMock);
    const query = { from: "2026-09-01", to: "2026-09-03", timezone: "America/Detroit" };
    const controller = new AbortController();
    expect(await inspectUsage(query, controller.signal)).toEqual(response);
    expect(fetchMock).toHaveBeenCalledWith("/api/data/usage/summary", expect.objectContaining({ method: "POST", body: JSON.stringify(query), signal: controller.signal }));
  });
  it("does not display raw external or backend error details", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("secret-path-and-token", { status: 500 })));
    await expect(inspectUsage({ from: "", to: "", timezone: "UTC" })).rejects.toThrow("Usage is unavailable");
  });
  it("turns the selected last day into the next calendar date across month and DST boundaries", () => {
    expect(shiftDate("2026-03-08", 1)).toBe("2026-03-09");
    expect(shiftDate("2026-09-30", 1)).toBe("2026-10-01");
  });
});
