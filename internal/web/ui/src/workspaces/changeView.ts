import type { Change, Check, Repository } from "../api/changes";
import { ago } from "../ui/time";

// What the Changes tab says about each Change (changes.spec.md story 6a):
// its state's label and colour, the group it sits in, the one action that
// moves it on, and a note on what that action does.

export type ChangeTone = "amber" | "teal" | "danger" | "draft" | "merged" | "closed";
export type ChangeGroupKind = "review" | "progress" | "done";
export type ChangeActionKind = "merge" | "arm" | "disarm" | "push";

export type ChangeAction = { kind: ChangeActionKind; label: string; muted?: boolean; disabled?: boolean };
export type ChangeStatus = { label: string; tone: ChangeTone; group: ChangeGroupKind; action?: ChangeAction; note: string };

const merge: ChangeAction = { kind: "merge", label: "Approve and merge" };
const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

/** How a Change's checks stand: none ran, some still run, one failed, or all passed. */
export function checksState(checks: Check[] = []): "none" | "running" | "failing" | "passing" {
  if (checks.length === 0) return "none";
  if (checks.some((check) => check.state === "failed")) return "failing";
  if (checks.some((check) => check.state === "pending" || check.state === "running")) return "running";
  return "passing";
}

/**
 * A Change's state as the tab shows it. Everything waiting on the owner is
 * "Needs your review": ready to merge, failing checks, or a conflict. A
 * merge is the owner's to make whatever the checks say; "Merge when checks
 * pass" waits for them.
 */
export function changeStatus(change: Change, kind: Repository["kind"], now: Date): ChangeStatus {
  if (change.state === "merged") return { label: "Merged", tone: "merged", group: "done", note: `Merged ${ago(change.updated_at, now).toLowerCase()}` };
  if (change.state === "closed") return { label: "Closed", tone: "closed", group: "done", note: `Closed ${ago(change.updated_at, now).toLowerCase()}` };
  if (change.needs_rebase) {
    return { label: "Needs rebase", tone: "amber", group: "review", action: { ...merge, disabled: true }, note: `It conflicts with ${change.base}; rebase it, then merge` };
  }
  if (kind === "github") {
    const pull = change.pull_request;
    if (!pull) {
      return { label: "Draft, not pushed", tone: "draft", group: "progress", action: change.author === "evie" ? { kind: "push", label: "Push and open PR" } : undefined, note: "Only in the worktree" };
    }
    if (pull.draft) return { label: "Draft on GitHub", tone: "draft", group: "progress", note: "Mark it ready for review on GitHub to merge it" };
    if (change.armed_at) return { label: "Merges when checks pass", tone: "teal", group: "progress", action: { kind: "disarm", label: "Undo auto-merge", muted: true }, note: "Merges itself when checks pass" };
    const checks = change.checks ?? [];
    switch (checksState(checks)) {
      case "failing":
        return { label: "Checks failing", tone: "danger", group: "review", action: merge, note: `${plural(checks.filter((check) => check.state === "failed").length, "check", "checks")} failing` };
      case "running":
        return { label: "Checks running", tone: "teal", group: "progress", action: { kind: "arm", label: "Merge when checks pass" },
          note: `${plural(checks.filter((check) => check.state === "pending" || check.state === "running").length, "check", "checks")} still running` };
      case "passing":
        return { label: "Needs your review", tone: "amber", group: "review", action: merge, note: "Checks passed" };
    }
    return { label: "Needs your review", tone: "amber", group: "review", action: merge, note: "No checks ran" };
  }
  if (change.state === "open") return { label: "Needs your review", tone: "amber", group: "review", action: merge, note: "Verify passed" };
  return { label: "Draft, not verified", tone: "draft", group: "progress", action: change.worktree ? merge : undefined, note: "Verify runs before it merges" };
}

export type ChangeGroup = { kind: ChangeGroupKind; label: string; changes: Change[] };

const groupLabels: Record<ChangeGroupKind, string> = { review: "Needs your review", progress: "In progress", done: "Merged and closed" };

/** The tab's groups in order, empty ones left out, each newest first. */
export function groupChanges(changes: Change[], kind: Repository["kind"], now: Date): ChangeGroup[] {
  const order: ChangeGroupKind[] = ["review", "progress", "done"];
  const newest = (a: Change, b: Change) => b.updated_at.localeCompare(a.updated_at);
  return order
    .map((group) => ({ kind: group, label: groupLabels[group], changes: changes.filter((change) => changeStatus(change, kind, now).group === group).sort(newest) }))
    .filter((group) => group.changes.length > 0);
}

/** How many Changes wait on the owner: the tab's badge. */
export function reviewCount(changes: Change[], kind: Repository["kind"], now: Date): number {
  return changes.filter((change) => changeStatus(change, kind, now).group === "review").length;
}

/** A Change's number: its pull request's, or "Draft". */
export function changeNumber(change: Change): string {
  return change.pull_request ? `#${change.pull_request.number}` : "Draft";
}

/** Where a Change came from: its workflow, a chat, or nothing for the owner's own. */
export function changeOrigin(change: Change): string {
  if (change.origin?.workflow) return `from ${change.origin.workflow}`;
  if (change.origin?.session_id) return "from chat";
  return "";
}

/** A Change's checks in a word, with how it reads. */
export function checksSummary(checks: Check[] = []): { label: string; tone: "muted" | "teal" | "danger" | "none" } {
  switch (checksState(checks)) {
    case "failing":
      return { label: "Failing", tone: "danger" };
    case "running":
      return { label: "Running", tone: "teal" };
    case "passing":
      return { label: "Passing", tone: "muted" };
  }
  return { label: "None", tone: "none" };
}

/** Five segments of added against deleted lines; all empty without either. */
export function diffBar(additions = 0, deletions = 0): ("add" | "del" | "none")[] {
  const total = additions + deletions;
  if (total === 0) return Array(5).fill("none");
  const added = Math.round((additions / total) * 5);
  return Array.from({ length: 5 }, (_, i) => (i < added ? "add" : "del"));
}

/** "+11 −4", with a real minus sign. */
export function lineCounts(additions = 0, deletions = 0): string {
  return `+${additions} −${deletions}`;
}
