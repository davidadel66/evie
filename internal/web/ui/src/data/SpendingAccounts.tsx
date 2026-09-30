import { useCallback, useEffect, useRef, useState } from "react";
import {
  cancelSpendingLink, inspectSpendingAccounts, pollSpendingLink, refreshSpendingAccounts, startSpendingLink,
  type SpendingAccountsSnapshot, type SpendingInstitution, type SpendingLink,
} from "../api/spendingAccounts";
import { Plus } from "../ui/Icon";

type Action = "refresh" | "start" | "cancel";
const control = "border-hair-input text-body hover:bg-hover focus-visible:ring-teal rounded-md border px-3 py-2 text-xs focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40";

export function SpendingAccounts({ onLinked }: { onLinked: () => void }) {
  const [snapshot, setSnapshot] = useState<SpendingAccountsSnapshot>();
  const [loading, setLoading] = useState(true);
  const [action, setAction] = useState<Action>();
  const [problem, setProblem] = useState("");
  const [actionProblem, setActionProblem] = useState("");
  const [notice, setNotice] = useState("");
  const [link, setLink] = useState<SpendingLink>();
  const [linkProblem, setLinkProblem] = useState("");
  const [pollStopped, setPollStopped] = useState(false);
  const [checkSavedAccounts, setCheckSavedAccounts] = useState(false);
  const [hostedExpired, setHostedExpired] = useState(false);
  const readRequest = useRef<AbortController | null>(null);
  const actionRequest = useRef<AbortController | null>(null);
  const stopPolling = useRef<(() => void) | null>(null);
  const blankTab = useRef<Window | null>(null);
  const linkedCallback = useRef(onLinked);
  useEffect(() => { linkedCallback.current = onLinked; }, [onLinked]);

  const reload = useCallback(async (resumeLink = true, onLoaded?: () => void) => {
    readRequest.current?.abort();
    const request = new AbortController();
    readRequest.current = request;
    setLoading(true); setProblem("");
    try {
      const next = await inspectSpendingAccounts(request.signal);
      if (request.signal.aborted) return;
      setSnapshot(next);
      if (resumeLink) {
        setLink(next.pendingLink);
        setLinkProblem("");
        setPollStopped(false);
        setCheckSavedAccounts(false);
      }
      onLoaded?.();
    } catch {
      if (!request.signal.aborted) setProblem("Saved accounts could not be loaded. Try again.");
    } finally {
      if (!request.signal.aborted) setLoading(false);
      if (readRequest.current === request) readRequest.current = null;
    }
  }, []);

  useEffect(() => {
    void reload();
    return () => {
      readRequest.current?.abort();
      actionRequest.current?.abort();
      closeTab(blankTab.current);
      blankTab.current = null;
    };
  }, [reload]);

  useEffect(() => {
    const remaining = link ? Date.parse(link.expiresAt) - Date.now() : 0;
    setHostedExpired(!!link && remaining <= 0);
    if (!link || remaining <= 0) return;
    const timer = setTimeout(() => setHostedExpired(true), remaining);
    return () => clearTimeout(timer);
  }, [link]);

  useEffect(() => {
    if (!link || pollStopped) return;
    const stop = pollSpendingLink(link, {
      onResult: result => {
        setLink(undefined);
        if (result.status === "linked") {
          setCheckSavedAccounts(false);
          setNotice("Account connected. Choose Fetch latest when you want to load transactions.");
          void reload(false);
          linkedCallback.current();
        } else if (result.status === "failed") {
          setLinkProblem("The account connection failed. Choose Add account to try again.");
          setCheckSavedAccounts(true);
        } else {
          setNotice(result.status === "cancelled" ? "Stopped waiting for this connection." : "The connection expired. Choose Add account to try again.");
          setCheckSavedAccounts(true);
        }
      },
      onError: () => {
        setPollStopped(true);
        setLinkProblem("Connection status could not be checked. Check the connection to continue.");
      },
    });
    stopPolling.current = stop;
    return () => {
      stop();
      if (stopPolling.current === stop) stopPolling.current = null;
    };
  }, [link, pollStopped, reload]);

  const begin = (next: Action) => {
    if (actionRequest.current) return;
    const request = new AbortController();
    actionRequest.current = request;
    setAction(next); setActionProblem(""); setNotice("");
    return request;
  };
  const finish = (request: AbortController) => {
    if (actionRequest.current === request) actionRequest.current = null;
    if (!request.signal.aborted) setAction(undefined);
  };

  const refresh = async () => {
    const request = begin("refresh");
    if (!request) return;
    try {
      const result = await refreshSpendingAccounts(request.signal);
      if (request.signal.aborted) return;
      if (result.institutionsFailed) {
        setActionProblem(`${result.institutionsSucceeded} ${result.institutionsSucceeded === 1 ? "institution" : "institutions"} refreshed; ${result.institutionsFailed} could not be refreshed. Saved details are kept.`);
      } else {
        setNotice("Account details refreshed.");
      }
    } catch {
      if (!request.signal.aborted) setActionProblem("Accounts could not be refreshed. Saved account details are still available.");
    } finally {
      if (!request.signal.aborted) await reload(false);
      finish(request);
    }
  };

  const add = async () => {
    if (link) return;
    const request = begin("start");
    if (!request) return;
    setLinkProblem("");
    setCheckSavedAccounts(false);
    // Reserve the tab during the click, before awaiting the Hosted Link URL.
    // The in-page link remains available if a browser blocks or closes it.
    blankTab.current = openBlankTab();
    try {
      const next = await startSpendingLink(request.signal);
      if (request.signal.aborted) return;
      setLink(next); setPollStopped(false);
      const tab = blankTab.current;
      blankTab.current = null;
      try { if (tab && !tab.closed) tab.location.replace(next.hostedUrl); }
      catch { closeTab(tab); }
    } catch {
      if (!request.signal.aborted) {
        setActionProblem("Plaid could not be opened. Reload accounts before trying again.");
        await reload();
      }
    } finally {
      closeTab(blankTab.current); blankTab.current = null;
      finish(request);
    }
  };

  const cancel = async () => {
    if (!link) return;
    const request = begin("cancel");
    if (!request) return;
    stopPolling.current?.();
    setPollStopped(true);
    try {
      await cancelSpendingLink(link.id, request.signal);
      if (request.signal.aborted) return;
      setLink(undefined); setLinkProblem(""); setCheckSavedAccounts(false);
      setNotice("Stopped waiting for this connection. Any accounts already connected are kept.");
      await reload(false);
      if (!request.signal.aborted) linkedCallback.current();
    } catch {
      if (!request.signal.aborted) setLinkProblem("The connection could not be cancelled. Check its status before trying again.");
    } finally { finish(request); }
  };

  return <SpendingAccountsView snapshot={snapshot} loading={loading} action={action} problem={problem || actionProblem}
    notice={notice} link={link} linkProblem={linkProblem} pollStopped={pollStopped} hostedExpired={hostedExpired} checkSavedAccounts={checkSavedAccounts}
    onReload={() => { setActionProblem(""); void reload(true, () => linkedCallback.current()); }} onRefresh={() => void refresh()} onAdd={() => void add()} onCancel={() => void cancel()} />;
}

type ViewProps = {
  snapshot?: SpendingAccountsSnapshot;
  loading: boolean;
  action?: Action;
  problem?: string;
  notice?: string;
  link?: SpendingLink;
  linkProblem?: string;
  pollStopped?: boolean;
  hostedExpired?: boolean;
  checkSavedAccounts?: boolean;
  onReload: () => void;
  onRefresh: () => void;
  onAdd: () => void;
  onCancel: () => void;
};

export function SpendingAccountsView({ snapshot, loading, action, problem, notice, link, linkProblem, pollStopped, hostedExpired, checkSavedAccounts, onReload, onRefresh, onAdd, onCancel }: ViewProps) {
  return <section aria-label="Connected accounts" className="border-hair mb-5 border-b pb-5">
    <header className="flex flex-wrap items-center justify-between gap-3">
      <h2 className="text-ink text-sm font-medium">Connected accounts</h2>
      <div className="flex flex-wrap gap-2">
        <button type="button" onClick={onRefresh} disabled={loading || !!action || !snapshot?.institutions.length} className={control}>{action === "refresh" ? "Refreshing accounts…" : "Refresh accounts"}</button>
        <button type="button" onClick={onAdd} disabled={loading || !!action || !!link || !snapshot?.linkAvailable} className="bg-teal text-primary-foreground hover:bg-teal-hover focus-visible:ring-teal flex items-center gap-1.5 rounded-md px-3 py-2 text-xs font-medium focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40"><Plus size={13} />{action === "start" ? "Opening Plaid…" : "Add account"}</button>
      </div>
    </header>
    {snapshot && !snapshot.linkAvailable && <p className="text-muted-text mt-3 text-xs leading-5">Add account is unavailable until Plaid is configured.</p>}
    {problem && <p role="alert" className="text-amber-ink mt-3 text-xs leading-5">{problem} <button type="button" disabled={loading || !!action} onClick={onReload} className="focus-visible:ring-teal rounded underline underline-offset-2 focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40">Reload saved accounts</button></p>}
    {loading && <p role="status" className="text-muted-text mt-3 text-xs">{snapshot ? "Loading saved accounts…" : "Loading accounts…"}</p>}
    {snapshot?.institutions.length === 0 && <p className="text-muted-text mt-4 text-xs leading-5">No connected accounts yet. Add an account to get started.</p>}
    {snapshot && snapshot.institutions.length > 0 && <div className="divide-hair mt-2 divide-y">{snapshot.institutions.map(institution => <InstitutionAccounts key={institution.id} institution={institution} />)}</div>}
    {link && <div className="border-teal-hair bg-teal-deep/20 mt-4 rounded-lg border p-3">
      <p role="status" className="text-body text-xs leading-5">{action === "cancel" ? "Stopping connection checks…" : pollStopped ? "Connection is waiting for a status check." : hostedExpired ? "Checking whether Plaid finished connecting…" : "Finish connecting in Plaid, then return here."}</p>
      <p className="text-muted-text mt-1 text-[11px]">{hostedExpired ? "The Plaid page has expired." : `Link expires ${new Date(link.expiresAt).toLocaleString()}.`}</p>
      <div className="mt-3 flex flex-wrap items-center gap-3">
        {action !== "cancel" && !hostedExpired && <a href={link.hostedUrl} target="_blank" rel="noopener noreferrer" className="text-teal-hover focus-visible:ring-teal rounded text-xs font-medium underline underline-offset-2 focus-visible:ring-2 focus-visible:outline-none">Continue with Plaid</a>}
        {pollStopped && <button type="button" onClick={onReload} disabled={loading || !!action} className={control}>Check connection</button>}
        <button type="button" onClick={onCancel} disabled={!!action} className="text-muted-text hover:text-ink focus-visible:ring-teal rounded px-1 py-1 text-xs focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40">Cancel connection</button>
      </div>
    </div>}
    {linkProblem && <p role="alert" className="text-amber-ink mt-3 text-xs leading-5">{linkProblem}</p>}
    {notice && <p role="status" className="text-muted-text mt-3 text-xs leading-5">{notice}</p>}
    {checkSavedAccounts && <button type="button" onClick={onReload} disabled={loading || !!action} className={`${control} mt-3`}>Check saved accounts</button>}
  </section>;
}

function InstitutionAccounts({ institution }: { institution: SpendingInstitution }) {
  const updated = institution.accountsUpdatedAt && Number.isFinite(Date.parse(institution.accountsUpdatedAt)) ? new Date(institution.accountsUpdatedAt) : undefined;
  return <section aria-label={institution.name || "Connected institution"} className="py-3">
    <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
      <h3 className="text-body text-xs font-medium">{institution.name || "Connected institution"}</h3>
      {updated && <p className="text-muted-text text-[10px]">Saved <time dateTime={institution.accountsUpdatedAt ?? undefined}>{updated.toLocaleString()}</time></p>}
    </div>
    {!institution.accountsKnown && <p className="text-muted-text mt-2 text-xs leading-5">Account details have not been loaded. Choose Refresh accounts to load them.</p>}
    {institution.accountsKnown && institution.accounts.length === 0 && <p className="text-muted-text mt-2 text-xs">No accounts were returned for this institution.</p>}
    {institution.accounts.length > 0 && <ul className="mt-2 space-y-2">{institution.accounts.map(account => {
      const mask = account.mask?.replace(/\D/g, "").slice(-4);
      const kind = (account.subtype || account.type).replaceAll("_", " ");
      return <li key={account.id} className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 text-xs">
        <span className="text-body min-w-0 break-words">{account.name || "Unnamed account"}{mask && <span className="text-muted-text ml-2 whitespace-nowrap" aria-label={`ending in ${mask}`}>•••• {mask}</span>}</span>
        {kind && <span className="text-muted-text text-[11px]">{kind}</span>}
      </li>;
    })}</ul>}
  </section>;
}

function closeTab(tab: Window | null) {
  try { tab?.close(); } catch { /* The in-page link remains available. */ }
}

function openBlankTab(): Window | null {
  let tab: Window | null = null;
  try {
    tab = window.open("about:blank", "_blank");
    if (tab) {
      tab.opener = null;
      const referrer = tab.document.createElement("meta");
      referrer.name = "referrer"; referrer.content = "no-referrer";
      tab.document.head.append(referrer);
    }
    return tab;
  } catch { closeTab(tab); return null; }
}
