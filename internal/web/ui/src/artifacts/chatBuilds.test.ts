import { describe, expect, it } from "vitest";
import type { ToolItem } from "../chat/activityModel";
import { buildsOfTool } from "./chatBuilds";

const tool = (name: string, result?: string): ToolItem => ({ kind: "tool", key: "t1", id: "c1", name, args: "{}", result, startedAt: 0 });

// changes.spec.md story 6b: a chat's build rows name the Changes they made.
describe("builds in a chat", () => {
  it("come from a delegate_build result, one per worker that left a branch", () => {
    const result = JSON.stringify([
      { execution_id: "e1", status: "succeeded", build: { branch: "evie/7d8bea7b-notes", base: "main", files: [{ path: "NOTES.md", added: 1, deleted: 0 }], insertions: 1, deletions: 0 } },
      { execution_id: "e2", status: "failed" },
    ]);
    expect(buildsOfTool(tool("delegate_build", result))).toEqual([
      { changeId: "build:evie/7d8bea7b-notes", branch: "evie/7d8bea7b-notes", status: "succeeded", files: [{ path: "NOTES.md", added: 1, deleted: 0 }], insertions: 1, deletions: 0 },
    ]);
  });

  it("come from a merge proposal, passed or refused", () => {
    const ready = JSON.stringify({ status: "ready", branch: "evie/2d3c83c8-lexer", target: "main", insertions: 3, deletions: 0 });
    expect(buildsOfTool(tool("propose_build_merge", ready))).toMatchObject([{ changeId: "build:evie/2d3c83c8-lexer", status: "ready", insertions: 3 }]);
    const refused = "tool call came back with error " + JSON.stringify({ status: "verify_failed", branch: "evie/2d3c83c8-lexer" });
    expect(buildsOfTool(tool("propose_build_merge", refused))).toMatchObject([{ status: "verify_failed" }]);
  });

  it("are none for other rows, rows still running, or results that are not JSON", () => {
    expect(buildsOfTool(tool("read_file", JSON.stringify({ build: { branch: "x" } })))).toEqual([]);
    expect(buildsOfTool(tool("delegate_build"))).toEqual([]);
    expect(buildsOfTool(tool("delegate_build", "the model gave up"))).toEqual([]);
  });
});
