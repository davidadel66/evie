import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { ChangeDetail } from "../api/changes";
import { BuildView } from "./BuildInspection";
import type { BuildRef } from "./chatBuilds";

const now = new Date("2026-10-10T12:00:00Z");
const build: BuildRef = { changeId: "build:evie/2d3c83c8-lexer", branch: "evie/2d3c83c8-lexer", status: "ready", insertions: 3, deletions: 0,
  files: [{ path: "lexer.go", added: 3, deleted: 0 }] };
const detail: ChangeDetail = {
  repository: { kind: "local", base_branch: "main" },
  change: { id: build.changeId, workspace_id: "w", branch: build.branch, base: "main", state: "open", author: "evie", title: "Add a lexer",
    insertions: 3, deletions: 0, updated_at: "2026-10-10T11:59:00Z" },
  files: [{ path: "lexer.go", status: "added", additions: 3, deletions: 0, blob: "b1", patch: "@@ -0,0 +1,3 @@\n+package main\n+\n+func lex() {}\n" }],
};

// changes.spec.md story 6b: a chat's build opens in the Inspector with its
// Change's files and diffs and "Open in Changes"; nothing is approved there.
describe("a build in the Inspector", () => {
  it("shows its Change with its diff and a way to the Changes tab", () => {
    const html = renderToStaticMarkup(<BuildView build={build} detail={detail} viewed={{}} now={now} onOpenChange={() => {}} onViewed={() => {}} />);
    for (const text of ["Add a lexer", "Needs your review", "evie/2d3c83c8-lexer → main", "+3 −0", "Open in Changes", "lexer.go", "func lex() {}"]) {
      expect(html).toContain(text);
    }
    expect(html).not.toContain("Approve and merge");
  });

  it("lists what the worker reported when there is no Change to read", () => {
    const html = renderToStaticMarkup(<BuildView build={build} viewed={{}} now={now} problem="This chat is not in a Workspace." onViewed={() => {}} />);
    expect(html).toContain("evie/2d3c83c8-lexer");
    expect(html).toContain("lexer.go");
    expect(html).toContain("This chat is not in a Workspace.");
    expect(html).not.toContain("Open in Changes");
  });
});
