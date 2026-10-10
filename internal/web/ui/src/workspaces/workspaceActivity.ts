import type { WorkflowCapStatus, WorkflowInterruption } from "../api/workflows";

// The Workspace header (workspace-activity.spec.md): tabs, what needs the
// owner now, and today's workflow spend against the daily cap.

export type WorkspaceTab = "runs" | "workflows" | "settings";

export const workspaceTabs: { id: WorkspaceTab; label: string }[] = [
  { id: "runs", label: "Runs" },
  { id: "workflows", label: "Workflows" },
  { id: "settings", label: "Settings" },
];

/**
 * How many runs in each Workspace wait on the owner now: runs with an open
 * interruption, a question to answer or an effect to settle. A run asking
 * twice counts once.
 */
export function needsByWorkspace(interruptions: WorkflowInterruption[]): Record<string, number> {
  const runs = new Map<string, Set<string>>();
  for (const interruption of interruptions) {
    if (interruption.resolution) continue;
    const set = runs.get(interruption.workspace_id) ?? new Set<string>();
    set.add(interruption.run_id);
    runs.set(interruption.workspace_id, set);
  }
  return Object.fromEntries([...runs].map(([workspace, set]) => [workspace, set.size]));
}

/** A waiting count for assistive technology. */
export function waitingText(runs: number): string {
  return `${runs} ${runs === 1 ? "run waits" : "runs wait"} on you`;
}

/** A token count at a glance: 950, 1.5k, 412k, 2M. */
export function compactCount(n: number): string {
  const unit = (value: number, suffix: string) => `${value < 10 ? Math.round(value * 10) / 10 : Math.round(value)}${suffix}`;
  if (n < 1000) return String(n);
  if (n < 999_500) return unit(n / 1000, "k");
  return unit(n / 1_000_000, "M");
}

/** Today's spend of every workflow run, against the daily cap when one is enforced. */
export function workflowSpendLine(caps: WorkflowCapStatus): string {
  const dollars = `$${(caps.spent.micro_usd / 1_000_000).toFixed(2)}`;
  const tokens = caps.enforced
    ? `${compactCount(caps.spent.tokens)} of ${compactCount(caps.cap.tokens)} tokens`
    : `${compactCount(caps.spent.tokens)} tokens`;
  return `Workflows today: ${tokens} · ${dollars}`;
}
