import { SpendingAccountsView } from "ui";
import * as React from "react";

// The app root (App.tsx) sets these classes; every Evie screen sits on
// them, so previews render on the same dark surface and text color.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

// David's three Plaid-linked institutions as the accounts inspector shows them.
const snapshot = {
  linkAvailable: true,
  institutions: [
    { id: "ins_citi", name: "Citibank Online", accountsKnown: true, accountsUpdatedAt: "2026-10-07T13:00:05Z", accounts: [
      { id: "acc_citi_checking", name: "Citi Priority Checking", mask: "4418", type: "depository", subtype: "checking" },
      { id: "acc_citi_savings", name: "Citi Accelerate Savings", mask: "7702", type: "depository", subtype: "savings" },
      { id: "acc_citi_card", name: "Citi Double Cash", mask: "1164", type: "credit", subtype: "credit_card" },
    ] },
    { id: "ins_amex", name: "American Express", accountsKnown: true, accountsUpdatedAt: "2026-10-07T13:00:06Z", accounts: [
      { id: "acc_amex_gold", name: "American Express Gold Card", mask: "81009", type: "credit", subtype: "credit_card" },
    ] },
    { id: "ins_pnc", name: "PNC", accountsKnown: false, accountsUpdatedAt: null, accounts: [] },
  ],
};
const link = { id: "link_8f2c", hostedUrl: "https://secure.plaid.com/link/hosted/8f2c1e", expiresAt: "2026-10-08T19:45:00Z" };
const noop = () => {};
const callbacks = { onReload: noop, onRefresh: noop, onAdd: noop, onCancel: noop };

export const Connected = () => (
  <AppFrame>
    <SpendingAccountsView snapshot={snapshot} loading={false} {...callbacks} />
  </AppFrame>
);

export const LinkingInProgress = () => (
  <AppFrame>
    <SpendingAccountsView snapshot={snapshot} loading={false} link={link} {...callbacks} />
  </AppFrame>
);

export const ConnectionCheckStopped = () => (
  <AppFrame>
    <SpendingAccountsView snapshot={snapshot} loading={false} link={link} pollStopped={true}
      linkProblem="Connection status could not be checked. Check the connection to continue." {...callbacks} />
  </AppFrame>
);

export const RefreshProblem = () => (
  <AppFrame>
    <SpendingAccountsView snapshot={snapshot} loading={false}
      problem="2 institutions refreshed; 1 could not be refreshed. Saved details are kept." {...callbacks} />
  </AppFrame>
);

export const Loading = () => (
  <AppFrame>
    <SpendingAccountsView loading={true} {...callbacks} />
  </AppFrame>
);

export const NoAccountsYet = () => (
  <AppFrame>
    <SpendingAccountsView snapshot={{ linkAvailable: true, institutions: [] }} loading={false}
      notice="The connection expired. Choose Add account to try again." checkSavedAccounts={true} {...callbacks} />
  </AppFrame>
);
