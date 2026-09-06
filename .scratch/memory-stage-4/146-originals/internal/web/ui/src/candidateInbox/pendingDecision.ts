import type { ReviewDecision } from "../api/candidateReview";

export type PendingReview = { scope: string; decision: ReviewDecision };
export interface PendingDecisionJournal {
  read(): PendingReview | undefined;
  write(value: PendingReview): void;
  clear(deliveryKey: string): void;
}

// Only the exact delivery request is retained, never candidate/source/preview
// contents. One uncertain decision is recovered before another can replace it.
export function pendingDecisionJournal(storage?: Pick<Storage, "getItem" | "setItem" | "removeItem">): PendingDecisionJournal {
  const key = "evie.owner-review.pending.v1";
  let memory: PendingReview | undefined;
  const read = () => {
    const raw = storage?.getItem(key);
    if (!raw) return memory;
    if (raw.length > 8192) return undefined;
    try {
      const value = JSON.parse(raw) as PendingReview;
      if (typeof value.scope !== "string" || !value.scope || !value.decision || !["accept", "reject"].includes(value.decision.action) || ![value.decision.delivery_key, value.decision.preview_id, value.decision.preview_sha256, value.decision.reason].every((field) => typeof field === "string")) return undefined;
      return value;
    } catch { return undefined; }
  };
  return {
    read,
    write(value) { const previous = read(); if (previous && previous.decision.delivery_key !== value.decision.delivery_key) throw new Error("Recover the earlier decision before confirming another candidate."); const raw = JSON.stringify(value); if (raw.length > 8192) throw new Error("This decision is too large to retain safely; shorten the optional reason."); storage?.setItem(key, raw); memory = value; },
    clear(deliveryKey) { if (read()?.decision.delivery_key === deliveryKey) { storage?.removeItem(key); memory = undefined; } },
  };
}

export function browserPendingDecisionJournal() {
  // Access inside the methods lets the controller report blocked storage
  // without crashing the component or dispatching an unrecorded delivery.
  return pendingDecisionJournal({ getItem: (key) => window.sessionStorage.getItem(key), setItem: (key, value) => window.sessionStorage.setItem(key, value), removeItem: (key) => window.sessionStorage.removeItem(key) });
}
