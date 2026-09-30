export type SpendingAccount = {
  id: string;
  name: string;
  mask: string;
  type: string;
  subtype: string;
};

export type SpendingInstitution = {
  id: string;
  name: string;
  accountsKnown: boolean;
  accountsUpdatedAt?: string | null;
  accounts: SpendingAccount[];
};

export type SpendingLink = { id: string; hostedUrl: string; expiresAt: string };
export type SpendingAccountsSnapshot = {
  institutions: SpendingInstitution[];
  linkAvailable: boolean;
  pendingLink?: SpendingLink;
};
export type SpendingAccountsRefresh = { institutionsSucceeded: number; institutionsFailed: number };
export type SpendingLinkStatus = {
  status: "pending" | "linked" | "expired" | "failed" | "cancelled";
  institutionsLinked: number;
};

type Action = "accounts" | "accounts/refresh" | "link/start" | "link/status" | "link/cancel";
const errors: Record<Action, string> = {
  accounts: "Saved accounts could not be loaded. Try again.",
  "accounts/refresh": "Accounts could not be refreshed. Saved account details are still available.",
  "link/start": "Plaid could not be opened. Reload accounts before trying again.",
  "link/status": "Connection status could not be checked. Check the connection to continue.",
  "link/cancel": "The connection could not be cancelled. Check its status before trying again.",
};

async function request<T>(action: Action, body: object, signal?: AbortSignal): Promise<T> {
  try {
    const response = await fetch(`/api/data/spending/${action}`, {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), signal,
    });
    if (action === "link/status" && response.status === 404) {
      return { status: "expired", institutionsLinked: 0 } as T;
    }
    if (!response.ok) throw new Error(errors[action]);
    return await response.json() as T;
  } catch (error: unknown) {
    if (signal?.aborted) throw error;
    // Provider responses and network errors may contain credentials or IDs.
    throw new Error(errors[action]);
  }
}

export function isSafePlaidLink(value: string): boolean {
  try {
    const url = new URL(value);
    return url.protocol === "https:" && url.hostname === "secure.plaid.com" && !url.port && !url.username && !url.password;
  } catch { return false; }
}

function checkedLink(link: SpendingLink): SpendingLink {
  if (!link.id || !isSafePlaidLink(link.hostedUrl) || !Number.isFinite(Date.parse(link.expiresAt))) {
    throw new Error("The Plaid connection could not be opened safely. Reload accounts to try again.");
  }
  return link;
}

export async function inspectSpendingAccounts(signal?: AbortSignal): Promise<SpendingAccountsSnapshot> {
  const snapshot = await request<SpendingAccountsSnapshot>("accounts", {}, signal);
  if (snapshot.pendingLink) checkedLink(snapshot.pendingLink);
  return snapshot;
}

export function refreshSpendingAccounts(signal?: AbortSignal): Promise<SpendingAccountsRefresh> {
  return request("accounts/refresh", {}, signal);
}

export async function startSpendingLink(signal?: AbortSignal): Promise<SpendingLink> {
  return checkedLink(await request<SpendingLink>("link/start", {}, signal));
}

export function inspectSpendingLink(id: string, signal?: AbortSignal): Promise<SpendingLinkStatus> {
  return request("link/status", { id }, signal);
}

export function cancelSpendingLink(id: string, signal?: AbortSignal): Promise<object> {
  return request("link/cancel", { id }, signal);
}

// Status checks only follow a link already created by the owner. Expiring the
// hosted page does not prove an in-flight exchange failed, so check completion
// after that deadline instead of cancelling the request. A pending result may
// mean another process owns a bounded exchange; the server owns that lifecycle.
export function pollSpendingLink(link: SpendingLink, callbacks: { onResult: (status: SpendingLinkStatus) => void; onError: () => void }): () => void {
  const controller = new AbortController();
  const deadline = Date.parse(link.expiresAt);
  let stopped = false;
  let inFlight = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let expiry: ReturnType<typeof setTimeout> | undefined;
  const stop = () => {
    stopped = true;
    clearTimeout(timer); clearTimeout(expiry);
    controller.abort();
  };
  const fail = () => { stop(); callbacks.onError(); };
  const check = async () => {
    if (stopped || inFlight) return;
    inFlight = true;
    try {
      const status = await inspectSpendingLink(link.id, controller.signal);
      if (stopped) return;
      if (status.status !== "pending") {
        stop(); callbacks.onResult(status);
      } else {
        timer = setTimeout(() => { void check(); }, 3000);
      }
    } catch { if (!stopped) fail(); }
    finally { inFlight = false; }
  };
  expiry = setTimeout(() => {
    clearTimeout(timer);
    // A request begun before expiry may still durably complete the connection.
    // Its result schedules another check if it reports pending.
    if (!inFlight) void check();
  }, Math.max(0, deadline - Date.now()));
  timer = setTimeout(() => { void check(); }, Math.min(3000, Math.max(0, deadline - Date.now())));
  return stop;
}
