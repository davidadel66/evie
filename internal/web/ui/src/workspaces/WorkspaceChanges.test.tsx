import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Change, ChangeDetail } from "../api/changes";
import { groupChanges } from "./changeView";
import { ChangeList, ChangeReview, FileDiff } from "./WorkspaceChanges";
import { WorkspaceHeader } from "./WorkspaceHeader";

const now = new Date("2026-10-10T12:00:00Z");
const pull = { number: 142, url: "https://github.com/davidadel66/notes/pull/142", state: "open", head_sha: "abc", author: "davidadel66", updated_at: "2026-10-10T11:00:00Z" };
const change: Change = {
  id: "build:evie/7f3c1a2b-flaky", workspace_id: "w", branch: "evie/7f3c1a2b-flaky", base: "main", state: "open", author: "evie",
  origin: { run_id: "run-7", workflow: "flaky-test-fix", step: "patch" }, title: "Fix race in archived-session restore test",
  notes: "The restore test raced the archive.\n\nIt now waits for idle.", worktree: "/Users/me/code/notes/.evie/worktrees/7f3c1a2b",
  insertions: 11, deletions: 4, pull_request: pull, checks: [{ name: "unit", state: "passed" }, { name: "e2e", state: "running" }], updated_at: "2026-10-10T11:59:00Z",
};
const detail: ChangeDetail = {
  repository: { kind: "github", owner: "davidadel66", name: "notes", base_branch: "main" },
  change,
  files: [
    { path: "ui/src/restore.test.ts", status: "modified", additions: 2, deletions: 1, blob: "b1",
      patch: "@@ -41,3 +41,4 @@ describe\n context();\n-const timeout = 100;\n+const timeout = 2000;\n+waitForIdle();\n" },
    { path: "logo.png", status: "added", additions: 0, deletions: 0, binary: true, blob: "b2" },
    { path: ".env", status: "modified", additions: 0, deletions: 0, excluded: true, blob: "b3" },
  ],
};
const review = (overrides: Partial<Parameters<typeof ChangeReview>[0]> = {}) => renderToStaticMarkup(<ChangeReview detail={detail} now={now} busy={false}
  viewed={{}} layout="split" confirmClose={false} onBack={() => {}} onAction={() => {}} onClose={() => {}} onConfirmClose={() => {}}
  onViewed={() => {}} onLayout={() => {}} {...overrides} />);

// changes.spec.md story 6a: the Changes tab and its review page.
describe("Changes tab", () => {
  it("lists each group with its rows: state, number, branch, origin, checks, diff and time", () => {
    const merged = { ...change, id: "build:evie/0000-old", state: "merged" as const, title: "Stabilize sidebar resize test", updated_at: "2026-10-05T10:00:00Z" };
    const html = renderToStaticMarkup(<ChangeList groups={groupChanges([change, merged], "github", now)} kind="github" now={now} onSelect={() => {}} />);
    for (const text of ["Change", "Checks", "Diff", "Updated", "In progress", "Merged and closed", "Checks running", "#142", "evie/7f3c1a2b-flaky",
      "from flaky-test-fix", "Running", "+11 −4", "1m ago", "Stabilize sidebar resize test", "Oct 5"]) {
      expect(html).toContain(text);
    }
    expect(renderToStaticMarkup(<ChangeList groups={[]} kind="local" now={now} onSelect={() => {}} />)).toContain("No Changes yet.");
  });

  it("shows the Changes tab only with a Repository, with what waits on the owner as its badge", () => {
    const header = (showChanges: boolean, review: number) => renderToStaticMarkup(<WorkspaceHeader tab="changes" waiting={0} review={review} showChanges={showChanges} onTab={() => {}} />);
    expect(header(false, 0)).not.toContain("workspace-tab-changes");
    const html = header(true, 2);
    expect(html).toContain("workspace-tab-changes");
    expect(html).toContain("2 Changes wait on you");
    expect(html.indexOf("Runs")).toBeLessThan(html.indexOf("Changes"));
  });
});

describe("review page", () => {
  it("heads the Change with its state, branch, counts and the action that moves it on", () => {
    const html = review();
    for (const text of ["← Changes", "Fix race in archived-session restore test", 'href="https://github.com/davidadel66/notes/pull/142"', "Checks running",
      "evie/7f3c1a2b-flaky → main", "+11 −4", "3 files", "1 check still running", "Merge when checks pass", "Close"]) {
      expect(html).toContain(text);
    }
    const ready = review({ detail: { ...detail, change: { ...change, checks: [{ name: "unit", state: "passed" }] } } });
    expect(ready).toContain("Approve and merge");
    expect(ready).toContain("Checks passed");
    const merged = review({ detail: { ...detail, change: { ...change, state: "merged" } } });
    expect(merged).not.toContain("Approve and merge");
    expect(merged).not.toContain(">Close<");
    const confirm = review({ confirmClose: true });
    expect(confirm).toContain("Close it and remove Evie&#x27;s branch and worktree?");
    expect(confirm).toContain("Keep it");
    expect(review({ problem: "pull request #142 conflicts" })).toContain('role="alert"');
  });

  it("lists files with Viewed marks, checks, where it came from, the worktree and Evie's notes", () => {
    const html = review({ viewed: { "ui/src/restore.test.ts": "b1" } });
    for (const text of ["Files", "1 of 3 viewed", "restore.test.ts", "Checks", "unit", "Passed", "e2e", "Running", "Came from", "flaky-test-fix › patch ›",
      "Worktree", "/Users/me/code/notes/.evie/worktrees/7f3c1a2b", "Evie’s notes", "The restore test raced the archive.", "It now waits for idle.",
      "Side by side", "Unified", "Binary file.", "Review withholds this file"]) {
      expect(html).toContain(text);
    }
    // A viewed file's diff is folded away.
    expect(html).not.toContain("waitForIdle");
  });

  it("draws a file side by side or unified, marking what changed in a changed line", () => {
    const file = detail.files[0];
    const split = renderToStaticMarkup(<FileDiff file={file} layout="split" viewed={false} onViewed={() => {}} />);
    expect(split).toContain("@@ -41,3 +41,4 @@ describe");
    expect(split).toContain("grid-cols-[44px_minmax(0,1fr)_44px_minmax(0,1fr)]");
    expect(split).toMatch(/const timeout = <mark[^>]*>1<\/mark>00;/);
    expect(split).toMatch(/const timeout = <mark[^>]*>20<\/mark>00;/);
    const unified = renderToStaticMarkup(<FileDiff file={file} layout="unified" viewed={false} onViewed={() => {}} />);
    expect(unified).toContain("grid-cols-[44px_44px_1fr]");
    expect(unified).toContain("waitForIdle();");
    expect(unified).toContain("−");
  });
});
