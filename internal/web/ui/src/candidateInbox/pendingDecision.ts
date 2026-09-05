import type { ReviewDecision, BatchDecision } from "../api/candidateReview";

export type PendingReview = { scope: string; decision: ReviewDecision | BatchDecision };
export interface PendingDecisionJournal {
  read(): PendingReview | undefined;
  write(value: PendingReview): void;
  clear(deliveryKey: string): void;
}

// Only the exact delivery request is retained, never candidate/source/preview
// contents. The 32 KiB byte ceiling admits a 4 KiB reason even when JSON
// escaping expands every byte, plus twenty maximum-length group actions.
// One uncertain decision is recovered before another can replace it.
export function pendingDecisionJournal(storage?: Pick<Storage, "getItem" | "setItem" | "removeItem">): PendingDecisionJournal {
  const key = "evie.owner-review.pending.v1";
  let memory: PendingReview | undefined;
  const read = () => {
    const raw = storage?.getItem(key);
    if (!raw) return memory;
    if (new TextEncoder().encode(raw).length > 32*1024) return undefined;
    try {
      const value = JSON.parse(raw) as PendingReview;
      if (typeof value.scope !== "string" || !value.scope || !value.decision || !("actions" in value.decision ? Array.isArray(value.decision.actions) && value.decision.actions.length > 0 && value.decision.actions.length <= 20 && value.decision.actions.every((action) => typeof action.group_id === "string" && ["accept", "reject"].includes(action.action)) : ["accept", "reject"].includes(value.decision.action)) || ![value.decision.delivery_key, value.decision.preview_id, value.decision.preview_sha256, value.decision.reason].every((field) => typeof field === "string")) return undefined;
      return value;
    } catch { return undefined; }
  };
  return {
    read,
    write(value) { if (new TextEncoder().encode(value.decision.reason).length > 4096) throw new Error("This reason is too large; use at most 4 KiB of text."); const previous = read(); if (previous && previous.decision.delivery_key !== value.decision.delivery_key) throw new Error("Recover the earlier decision before confirming another candidate."); const raw = JSON.stringify(value); if (new TextEncoder().encode(raw).length > 32*1024) throw new Error("This decision is too large to retain safely; shorten the optional reason."); storage?.setItem(key, raw); memory = value; },
    clear(deliveryKey) { if (read()?.decision.delivery_key === deliveryKey) { storage?.removeItem(key); memory = undefined; } },
  };
}

export function browserPendingDecisionJournal() {
  // Access inside the methods lets the controller report blocked storage
  // without crashing the component or dispatching an unrecorded delivery.
  return pendingDecisionJournal({ getItem: (key) => window.sessionStorage.getItem(key), setItem: (key, value) => window.sessionStorage.setItem(key, value), removeItem: (key) => window.sessionStorage.removeItem(key) });
}
