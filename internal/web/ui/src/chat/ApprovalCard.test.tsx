import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ApprovalCard } from "./ApprovalCard";
import type { Item } from "../store/reducer";

describe("memory approval applicability", () => {
  for (const [scope, label] of [["global", "Everywhere"], ["workspace:w", "Workspace"], ["session:s", "This conversation"]]) {
    it(`shows ${label} with the full proposed relationship and evidence`, () => {
      const tool: Extract<Item, { kind: "tool" }> = { kind: "tool", key: "test", id: "call", name: "memory_remember_literal", args: JSON.stringify({ scope: { scope_key: scope }, subject: { canonical_name: "David" }, predicate: { label: "prefers" }, literal: { value: "concise answers" }, source: { evidence: "Please remember I prefer concise answers." } }), startedAt: 0, approval: { state: "pending", reqId: "live" } };
      const html = renderToStaticMarkup(<ApprovalCard tool={tool} onAnswer={() => undefined} />);
      for (const text of [label, "David · prefers · concise answers", "Please remember I prefer concise answers.", "Approve", "Decline"]) expect(html).toContain(text);
      const saved = renderToStaticMarkup(<ApprovalCard tool={{ ...tool, approval: { state: "approved", reqId: "" } }} onAnswer={() => { throw new Error("historical approval executed"); }} />);
      expect(saved).toContain("Approved"); expect(saved).not.toContain("<button");
    });
  }
});
