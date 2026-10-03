import { useEffect, useState } from "react";
import { memoryProposalAPI, type MemoryProposalAPI } from "../api/memoryProposals";

const pendingPollMs = 30_000;

/** usePendingMemoryProposals counts memories Evie proposed that await the
 *  owner's review, so a badge can show them: on mount, whenever refreshKey
 *  changes (a turn finished, a proposal was reviewed), on window focus and
 *  every 30 seconds. A failed read shows no badge rather than an error. */
export function usePendingMemoryProposals(refreshKey?: unknown, api: Pick<MemoryProposalAPI, "count"> = memoryProposalAPI): number {
  const [pending, setPending] = useState(0);
  useEffect(() => {
    let live = true;
    const load = () => {
      api.count().then((value) => { if (live) setPending(value.pending); }, () => { if (live) setPending(0); });
    };
    load();
    const timer = window.setInterval(load, pendingPollMs);
    window.addEventListener("focus", load);
    return () => {
      live = false;
      window.clearInterval(timer);
      window.removeEventListener("focus", load);
    };
  }, [api, refreshKey]);
  return pending;
}

/** pendingLabel names a review count for assistive technology. */
export function pendingLabel(pending: number): string {
  return `${pending} memory ${pending === 1 ? "proposal awaits" : "proposals await"} review`;
}
