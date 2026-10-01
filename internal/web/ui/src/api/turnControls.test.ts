import { afterEach, describe, expect, it, vi } from "vitest";
import { cancelTurn, compactSession, compactionNotice } from "./turnControls";
import { ApiError } from "./stream";

afterEach(() => vi.unstubAllGlobals());

function reply(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

describe("cancelTurn", () => {
  it("asks the server to stop the identified conversation's turn", async () => {
    const fetch = vi.fn().mockResolvedValue(reply(202, { status: "stopping" }));
    vi.stubGlobal("fetch", fetch);
    await expect(cancelTurn("session-1")).resolves.toBe("stopping");
    expect(fetch.mock.calls[0][0]).toBe("/api/cancel");
    expect(fetch.mock.calls[0][1].method).toBe("POST");
    expect(fetch.mock.calls[0][1].headers).toEqual({ "Content-Type": "application/json" });
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({ sessionId: "session-1" });
  });

  it("treats a turn that already ended as nothing left to stop", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(reply(409, { code: "no_active_turn", error: "no turn is running in this conversation" })));
    await expect(cancelTurn("session-1")).resolves.toBe("idle");
  });

  it("surfaces any other refusal", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(reply(403, { error: "cross-origin request rejected" })));
    const failure = cancelTurn("session-1");
    await expect(failure).rejects.toBeInstanceOf(ApiError);
    await expect(failure).rejects.toThrow("cross-origin request rejected");
  });
});

describe("compactSession", () => {
  it("reports a durable compaction", async () => {
    const fetch = vi.fn().mockResolvedValue(reply(200, { outcome: "compacted", eventId: "event-9" }));
    vi.stubGlobal("fetch", fetch);
    await expect(compactSession("session-1")).resolves.toEqual({ outcome: "compacted", eventId: "event-9" });
    expect(fetch.mock.calls[0][0]).toBe("/api/compact");
    expect(fetch.mock.calls[0][1].headers).toEqual({ "Content-Type": "application/json" });
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({ sessionId: "session-1" });
  });

  it("reports that nothing was eligible", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(reply(200, { outcome: "nothing_to_compact" })));
    await expect(compactSession("session-1")).resolves.toEqual({ outcome: "nothing_to_compact" });
  });

  it("reports the failure category without provider detail", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(reply(502, {
      code: "compaction_failed", classification: "provider_error", error: "context compaction failed; the conversation is unchanged",
    })));
    await expect(compactSession("session-1")).resolves.toEqual({ outcome: "failed", classification: "provider_error" });
  });

  it("reports a refusal while a turn is running", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(reply(409, { code: "turn_in_progress", error: "finish or stop the current turn before compacting" })));
    await expect(compactSession("session-1")).resolves.toEqual({
      outcome: "refused", code: "turn_in_progress", message: "finish or stop the current turn before compacting",
    });
  });

  it("throws for a guard rejection", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(reply(403, { error: "content type must be application/json" })));
    await expect(compactSession("session-1")).rejects.toThrow("content type must be application/json");
  });
});

describe("compactionNotice", () => {
  it("describes each outcome in the owner's terms", () => {
    expect(compactionNotice({ outcome: "compacted", eventId: "e" })).toEqual({ tone: "info", text: "Context compacted." });
    expect(compactionNotice({ outcome: "nothing_to_compact" })).toEqual({ tone: "info", text: "Nothing eligible for compaction yet." });
    expect(compactionNotice({ outcome: "failed", classification: "provider_error" })).toEqual({
      tone: "warning", text: "Compaction failed (provider error). The conversation is unchanged.",
    });
    expect(compactionNotice({ outcome: "failed", classification: "provider_response_invalid" }).text).toContain("(invalid summary)");
    expect(compactionNotice({ outcome: "failed", classification: "context_overflow" }).text).toContain("(context overflow)");
    expect(compactionNotice({ outcome: "failed", classification: "local_failure" }).text).toContain("(local error)");
    expect(compactionNotice({ outcome: "failed", classification: "something_new" }).text).toContain("(something new)");
    expect(compactionNotice({ outcome: "refused", code: "turn_in_progress", message: "x" })).toEqual({
      tone: "warning", text: "Finish or stop the current turn before compacting.",
    });
    expect(compactionNotice({ outcome: "refused", code: "session_busy", message: "busy elsewhere" })).toEqual({ tone: "warning", text: "busy elsewhere" });
  });
});
