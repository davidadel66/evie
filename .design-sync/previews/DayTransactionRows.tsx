import { DayTransactionRows } from "ui";
import * as React from "react";

// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

const categories = ["Coffee", "Credit Card Payments", "Dining", "Groceries", "Health", "Home Improvement", "Mortgage", "Paychecks", "Shopping", "Subscriptions", "Utilities", "excluded"];
const base = { date: "2026-09-18", revision: "r41", legacyCategory: null, legacySource: null };

// Friday, September 18 2026: payday, a split hardware-store run, a card
// payment that the overview leaves out, and a hotel stay nobody has filed yet.
const payday = [
  { ...base, id: "t1", name: "ACME CORP PAYROLL DIRECT DEP", merchantName: "", netCents: "403400", classification: "classified" as const,
    entries: [{ id: "e1", category: "Paychecks", amountCents: "-403400", source: "rule" }] },
  { ...base, id: "t2", name: "LOWES #1234 KALAMAZOO MI", merchantName: "Lowe's", netCents: "-17914", classification: "classified" as const,
    entries: [{ id: "e2", category: "Home Improvement", amountCents: "13674", source: "human" }, { id: "e3", category: "Shopping", amountCents: "4240", source: "human" }] },
  { ...base, id: "t3", name: "*1164 PAYMENT CITI CARD ONLINE ACH WEB", merchantName: "", netCents: "-200000", classification: "excluded" as const,
    entries: [{ id: "e4", category: "excluded", amountCents: "200000", source: "rule" }] },
  { ...base, id: "t4", name: "AC HOTEL BETHESDA BETHESDA MD", merchantName: "", netCents: "-32900", classification: "unclassified" as const, entries: [] },
  { ...base, id: "t5", name: "SQ *BLACK OWL COFFEE KALAMAZOO MI", merchantName: "Black Owl Coffee", netCents: "-610", classification: "classified" as const,
    entries: [{ id: "e5", category: "Coffee", amountCents: "610", source: "agent" }] },
];

// Monday, September 21: a single coffee.
const quiet = [
  { ...base, id: "t6", date: "2026-09-21", name: "SQ *BLACK OWL COFFEE KALAMAZOO MI", merchantName: "Black Owl Coffee", netCents: "-525", classification: "classified" as const,
    entries: [{ id: "e6", category: "Coffee", amountCents: "525", source: "agent" }] },
  { ...base, id: "t7", date: "2026-09-21", name: "KROGER #612 PORTAGE MI", merchantName: "Kroger", netCents: "-9873", classification: "classified" as const,
    entries: [{ id: "e7", category: "Groceries", amountCents: "9873", source: "rule" }] },
];

const callbacks = { onSaving: () => {}, onReload: () => {}, onSaved: () => {} };

export const Payday = () => (
  <AppFrame>
    <DayTransactionRows rows={payday as any} categories={categories} busy={false} {...callbacks} />
  </AppFrame>
);

export const QuietDay = () => (
  <AppFrame>
    <DayTransactionRows rows={quiet as any} categories={categories} busy={false} {...callbacks} />
  </AppFrame>
);

export const Busy = () => (
  <AppFrame>
    <DayTransactionRows rows={payday as any} categories={categories} busy={true} {...callbacks} />
  </AppFrame>
);

export const NoCategories = () => (
  <AppFrame>
    <DayTransactionRows rows={quiet as any} categories={[]} busy={false} {...callbacks} />
  </AppFrame>
);
