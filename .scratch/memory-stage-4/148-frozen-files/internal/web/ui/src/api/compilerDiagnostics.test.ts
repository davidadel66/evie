import { afterEach, describe, expect, it, vi } from "vitest";
import { compilerDiagnosticsAPI } from "./compilerDiagnostics";
import { diagnosticPage } from "../compilerDiagnostics/fixtures.test-support";
afterEach(() => vi.unstubAllGlobals());
describe("compiler diagnostics HTTP adapter", () => {
 it("sends exact scope and bounded typed query without minting authority, preserving null timings", async () => {
  const fetch = vi.fn(async () => new Response(JSON.stringify(diagnosticPage), { status: 200 })); vi.stubGlobal("fetch", fetch);
  const out = await compilerDiagnosticsAPI.inspect("global", { session_id: "closed-session", view: "jobs", limit: 32, cursor: "cursor" });
  expect(out.jobs[0].measurements[0].inference_nanos).toBeNull();
  const [path, options] = fetch.mock.calls[0] as unknown as [string, RequestInit]; expect(path).toBe("/api/memory/compiler/diagnostics"); expect(options.cache).toBe("no-store"); expect(options.method).toBe("POST");
  expect(JSON.parse(options.body as string)).toEqual({ scope_key: "global", input: { session_id: "closed-session", view: "jobs", limit: 32, cursor: "cursor" } });
 });
 it("pages sessions by metadata only and preserves safe server error codes", async () => {
  const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValueOnce(new Response(JSON.stringify({ session_ids: ["closed"], next_cursor: "next" }), { status: 200 })).mockResolvedValueOnce(new Response(JSON.stringify({ code: "invalid_cursor", error: "restart this paginated listing" }), { status: 400 })); vi.stubGlobal("fetch", fetch);
  expect(await compilerDiagnosticsAPI.sessions("session:closed", "cursor")).toEqual({ session_ids: ["closed"], next_cursor: "next" });
  expect(JSON.parse(fetch.mock.calls[0][1]?.body as string)).toEqual({ scope_key: "session:closed", input: { limit: 32, cursor: "cursor" } });
  await expect(compilerDiagnosticsAPI.sessions("global")).rejects.toMatchObject({ code: "invalid_cursor" });
 });
});
