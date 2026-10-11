import { describe, expect, it } from "vitest";
import type { Change } from "../api/changes";
import { changeNumber, changeOrigin, changeStatus, checksSummary, diffBar, groupChanges, lineCounts, reviewCount } from "./changeView";

const now = new Date("2026-10-10T12:00:00Z");
const base: Change = { id: "build:evie/1a2b3c4d-fix", workspace_id: "w", branch: "evie/1a2b3c4d-fix", base: "main", state: "draft", author: "evie",
  title: "Fix the footer", worktree: "/repo/.evie/worktrees/1a2b3c4d", updated_at: "2026-10-10T11:59:00Z" };
const pull = { number: 142, url: "https://github.com/davidadel66/notes/pull/142", state: "open", head_sha: "abc", author: "davidadel66", updated_at: "2026-10-10T11:00:00Z" };

// changes.spec.md story 6a: each Change's state, group, action and note.
describe("Change status", () => {
  it("on GitHub follows the pull request and its checks", () => {
    expect(changeStatus(base, "github", now)).toMatchObject({ label: "Draft, not pushed", group: "progress", action: { kind: "push", label: "Push and open PR" }, note: "Only in the worktree" });
    const open = { ...base, state: "open" as const, pull_request: pull };
    expect(changeStatus({ ...open, checks: [{ name: "ci", state: "running" }, { name: "lint", state: "passed" }] }, "github", now))
      .toMatchObject({ label: "Checks running", tone: "teal", group: "progress", action: { kind: "arm", label: "Merge when checks pass" }, note: "1 check still running" });
    expect(changeStatus({ ...open, checks: [{ name: "ci", state: "passed" }, { name: "lint", state: "neutral" }] }, "github", now))
      .toMatchObject({ label: "Needs your review", tone: "amber", group: "review", action: { kind: "merge", label: "Approve and merge" }, note: "Checks passed" });
    expect(changeStatus({ ...open, checks: [{ name: "ci", state: "failed" }] }, "github", now))
      .toMatchObject({ label: "Checks failing", tone: "danger", group: "review", action: { kind: "merge" }, note: "1 check failing" });
    expect(changeStatus({ ...open, checks: [] }, "github", now)).toMatchObject({ label: "Needs your review", note: "No checks ran" });
    expect(changeStatus({ ...open, armed_at: "2026-10-10T11:00:00Z", checks: [{ name: "ci", state: "running" }] }, "github", now))
      .toMatchObject({ label: "Merges when checks pass", group: "progress", action: { kind: "disarm", label: "Undo auto-merge", muted: true } });
    const githubDraft = changeStatus({ ...open, pull_request: { ...pull, draft: true } }, "github", now);
    expect(githubDraft.label).toBe("Draft on GitHub");
    expect(githubDraft.action).toBeUndefined();
    expect(changeStatus({ ...open, needs_rebase: true }, "github", now)).toMatchObject({ label: "Needs rebase", group: "review", action: { kind: "merge", disabled: true } });
    // The owner's own pull request merges too, but is never pushed by Evie.
    expect(changeStatus({ ...base, author: "owner" }, "github", now).action).toBeUndefined();
  });

  it("locally follows the merge gate", () => {
    expect(changeStatus(base, "local", now)).toMatchObject({ label: "Draft, not verified", group: "progress", action: { kind: "merge" }, note: "Verify runs before it merges" });
    expect(changeStatus({ ...base, state: "open" }, "local", now)).toMatchObject({ label: "Needs your review", group: "review", note: "Verify passed" });
    const merged = changeStatus({ ...base, state: "merged", updated_at: "2026-10-10T11:00:00Z" }, "local", now);
    expect(merged).toMatchObject({ label: "Merged", group: "done", note: "Merged 1h ago" });
    expect(merged.action).toBeUndefined();
    expect(changeStatus({ ...base, state: "closed" }, "local", now)).toMatchObject({ label: "Closed", group: "done" });
  });

  it("groups what waits on the owner first and counts it for the badge", () => {
    const changes: Change[] = [
      { ...base, id: "a", state: "merged", updated_at: "2026-10-09T10:00:00Z" },
      { ...base, id: "b", state: "open", updated_at: "2026-10-10T10:00:00Z" },
      { ...base, id: "c", updated_at: "2026-10-10T11:00:00Z" },
      { ...base, id: "d", state: "open", needs_rebase: true, updated_at: "2026-10-10T11:30:00Z" },
    ];
    const groups = groupChanges(changes, "local", now);
    expect(groups.map((group) => [group.label, group.changes.map((change) => change.id)])).toEqual([
      ["Needs your review", ["d", "b"]], ["In progress", ["c"]], ["Merged and closed", ["a"]],
    ]);
    expect(reviewCount(changes, "local", now)).toBe(2);
  });
});

describe("Change rows", () => {
  it("name the number, origin and checks", () => {
    expect(changeNumber(base)).toBe("Draft");
    expect(changeNumber({ ...base, pull_request: pull })).toBe("#142");
    expect(changeOrigin({ ...base, origin: { run_id: "r", workflow: "flaky-test-fix", step: "build" } })).toBe("from flaky-test-fix");
    expect(changeOrigin({ ...base, origin: { session_id: "s" } })).toBe("from chat");
    expect(changeOrigin({ ...base, author: "owner" })).toBe("");
    expect(checksSummary([])).toEqual({ label: "None", tone: "none" });
    expect(checksSummary([{ name: "ci", state: "passed" }])).toEqual({ label: "Passing", tone: "muted" });
    expect(checksSummary([{ name: "ci", state: "pending" }])).toEqual({ label: "Running", tone: "teal" });
    expect(checksSummary([{ name: "ci", state: "passed" }, { name: "e2e", state: "failed" }])).toEqual({ label: "Failing", tone: "danger" });
  });

  it("draw added against deleted lines in five segments", () => {
    expect(diffBar(212, 198)).toEqual(["add", "add", "add", "del", "del"]);
    expect(diffBar(11, 0)).toEqual(["add", "add", "add", "add", "add"]);
    expect(diffBar(0, 0)).toEqual(["none", "none", "none", "none", "none"]);
    expect(lineCounts(11, 4)).toBe("+11 −4");
  });
});
