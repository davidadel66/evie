import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  cancelSpendingLink, inspectSpendingAccounts, inspectSpendingLink, isSafePlaidLink, pollSpendingLink,
  refreshSpendingAccounts, startSpendingLink, type SpendingLink,
} from "./spendingAccounts";

const link: SpendingLink = { id: "connection-one", hostedUrl: "https://secure.plaid.com/link/test", expiresAt: "2026-09-21T18:30:00Z" };

describe("Spending account API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("reads saved inventory without refreshing or starting a link", async () => {
    const snapshot = { institutions: [], linkAvailable: true, pendingLink: link };
    const fetch = vi.fn(async () => Response.json(snapshot));
    vi.stubGlobal("fetch", fetch);
    const controller = new AbortController();
    await expect(inspectSpendingAccounts(controller.signal)).resolves.toEqual(snapshot);
    expect(fetch).toHaveBeenCalledExactlyOnceWith("/api/data/spending/accounts", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: "{}", signal: controller.signal,
    });
  });

  it("uses explicit guarded operations and preserves partial refresh outcomes", async () => {
    const fetch = vi.fn(async (path: string) => Response.json(path.endsWith("/refresh") ? { institutionsSucceeded: 2, institutionsFailed: 1 } : path.endsWith("/start") ? link : {}));
    vi.stubGlobal("fetch", fetch);
    await expect(refreshSpendingAccounts()).resolves.toEqual({ institutionsSucceeded: 2, institutionsFailed: 1 });
    await expect(startSpendingLink()).resolves.toEqual(link);
    await cancelSpendingLink(link.id);
    expect(fetch.mock.calls.map(call => call[0])).toEqual(["/api/data/spending/accounts/refresh", "/api/data/spending/link/start", "/api/data/spending/link/cancel"]);
    expect(fetch).toHaveBeenLastCalledWith("/api/data/spending/link/cancel", expect.objectContaining({ method: "POST", body: JSON.stringify({ id: link.id }) }));
  });

  it("keeps raw provider errors and network details out of public errors", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ error: "access-token-private item-private bank credentials" }, { status: 500 })));
    await expect(refreshSpendingAccounts()).rejects.toThrow("Accounts could not be refreshed. Saved account details are still available.");
    await expect(startSpendingLink()).rejects.toThrow("Plaid could not be opened. Reload accounts before trying again.");
    vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("private-token in network URL"); }));
    await expect(inspectSpendingAccounts()).rejects.toThrow("Saved accounts could not be loaded. Try again.");
  });

  it("treats a missing link status as expired instead of polling a removed link", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("private error body", { status: 404 })));
    await expect(inspectSpendingLink(link.id)).resolves.toEqual({ status: "expired", institutionsLinked: 0 });
  });

  it("accepts only the credential-free HTTPS Plaid hosted origin", async () => {
    expect(isSafePlaidLink(link.hostedUrl)).toBe(true);
    for (const url of ["javascript:alert(1)", "http://secure.plaid.com/link", "https://secure.plaid.com.evil.example/link", "https://user:secret@secure.plaid.com/link", "https://secure.plaid.com:444/link", "https://plaid.com/link"]) {
      expect(isSafePlaidLink(url)).toBe(false);
      vi.stubGlobal("fetch", vi.fn(async () => Response.json({ ...link, hostedUrl: url })));
      await expect(startSpendingLink()).rejects.toThrow("could not be opened safely");
    }
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ institutions: [], linkAvailable: true, pendingLink: { ...link, expiresAt: "not a date" } })));
    await expect(inspectSpendingAccounts()).rejects.toThrow("could not be opened safely");
  });
});

describe("Spending connection polling", () => {
  const start = Date.parse("2026-09-21T18:00:00Z");
  beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(start); });
  afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });
  const callbacks = () => ({ onResult: vi.fn(), onError: vi.fn() });

  it("checks only the active link every three seconds and stops on success", async () => {
    const fetch = vi.fn(async () => Response.json({ status: "pending", institutionsLinked: 0 }));
    vi.stubGlobal("fetch", fetch);
    const events = callbacks();
    const stop = pollSpendingLink(link, events);
    expect(fetch).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(3000);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenLastCalledWith("/api/data/spending/link/status", expect.objectContaining({ body: JSON.stringify({ id: link.id }) }));
    fetch.mockImplementation(async () => Response.json({ status: "linked", institutionsLinked: 2 }));
    await vi.advanceTimersByTimeAsync(3000);
    expect(events.onResult).toHaveBeenCalledExactlyOnceWith({ status: "linked", institutionsLinked: 2 });
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(events.onError).not.toHaveBeenCalled();
    stop();
  });

  it("does not abort a completion begun before Hosted Link expires", async () => {
    let resolve!: (value: Response) => void;
    const fetch = vi.fn((_path: string, _options: RequestInit) => new Promise<Response>(done => { resolve = done; }));
    vi.stubGlobal("fetch", fetch);
    const events = callbacks();
    pollSpendingLink({ ...link, expiresAt: new Date(start + 4000).toISOString() }, events);
    await vi.advanceTimersByTimeAsync(3000);
    const signal = fetch.mock.calls[0][1].signal;
    await vi.advanceTimersByTimeAsync(2000);
    expect(signal?.aborted).toBe(false);
    expect(fetch).toHaveBeenCalledTimes(1);
    resolve(Response.json({ status: "linked", institutionsLinked: 1 }));
    await vi.advanceTimersByTimeAsync(0);
    expect(events.onResult).toHaveBeenCalledExactlyOnceWith({ status: "linked", institutionsLinked: 1 });
    expect(events.onError).not.toHaveBeenCalled();
  });

  it("checks at URL expiry and keeps polling a server-owned exchange until terminal", async () => {
    const fetch = vi.fn(async () => Response.json({ status: "pending", institutionsLinked: 0 }));
    vi.stubGlobal("fetch", fetch);
    const events = callbacks();
    pollSpendingLink({ ...link, expiresAt: new Date(start + 4000).toISOString() }, events);
    await vi.advanceTimersByTimeAsync(4000);
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(events.onResult).not.toHaveBeenCalled();
    fetch.mockImplementation(async () => Response.json({ status: "expired", institutionsLinked: 0 }));
    await vi.advanceTimersByTimeAsync(3000);
    expect(events.onResult).toHaveBeenCalledExactlyOnceWith({ status: "expired", institutionsLinked: 0 });
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetch).toHaveBeenCalledTimes(3);
  });

  it("stops on a status error until the owner explicitly retries", async () => {
    const fetch = vi.fn(async () => new Response("private-provider-error", { status: 502 }));
    vi.stubGlobal("fetch", fetch);
    const events = callbacks();
    pollSpendingLink(link, events);
    await vi.advanceTimersByTimeAsync(3000);
    expect(events.onError).toHaveBeenCalledExactlyOnceWith();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(events.onResult).not.toHaveBeenCalled();
  });

  it("aborts reads and ignores late completion after cancellation or unmount", async () => {
    let resolve!: (value: Response) => void;
    const fetch = vi.fn((_path: string, _options: RequestInit) => new Promise<Response>(done => { resolve = done; }));
    vi.stubGlobal("fetch", fetch);
    const events = callbacks();
    const stop = pollSpendingLink(link, events);
    await vi.advanceTimersByTimeAsync(3000);
    stop();
    expect(fetch.mock.calls[0][1].signal?.aborted).toBe(true);
    resolve(Response.json({ status: "linked", institutionsLinked: 1 }));
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(events.onResult).not.toHaveBeenCalled();
    expect(events.onError).not.toHaveBeenCalled();
  });
});
