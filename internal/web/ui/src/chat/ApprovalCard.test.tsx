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

  it("says plainly that an Evie-proposed value is not in the owner's words", () => {
    const tool: Extract<Item, { kind: "tool" }> = { kind: "tool", key: "test", id: "call", name: "memory_remember_literal", args: JSON.stringify({ scope: { scope_key: "global" }, subject: { canonical_name: "owner", anchor_kind: "owner" }, predicate: { label: "preferred payee" }, literal: { value: "Acme Offshore" }, source: { authority: "evie_proposed", evidence: "" } }), startedAt: 0, approval: { state: "pending", reqId: "live" } };
    const html = renderToStaticMarkup(<ApprovalCard tool={tool} onAnswer={() => undefined} />);
    expect(html).toContain("Evie’s proposal");
    expect(html).toContain("not in your message");
    expect(html).not.toContain("<blockquote");
  });

  it("labels the bound span as the owner's words and shows Entity reuse", () => {
    const tool: Extract<Item, { kind: "tool" }> = { kind: "tool", key: "test", id: "call", name: "memory_remember_entity", args: JSON.stringify({ scope: { scope_key: "global" }, predicate: { label: "loves" }, claim: { subject_entity_id: "s", object_entity_id: "o" }, entities: [{ entity_id: "s", canonical_name: "Sarah" }, { entity_id: "o", canonical_name: "Chess" }], source: { authority: "owner_statement", evidence: "Sarah also loves chess." }, identities: [{ role: "subject", entity_id: "1f3c9a2e-0000-4000-8000-000000000001", canonical_name: "Sarah", entity_type: "person", reused: true, selected_by: "alias", aliases: ["Sarah"], example_claim: "Sarah — plays: Tennis" }] }), startedAt: 0, approval: { state: "pending", reqId: "live" } };
    const html = renderToStaticMarkup(<ApprovalCard tool={tool} onAnswer={() => undefined} />);
    for (const text of ["Your words", "Sarah also loves chess.", "Reuses existing Sarah (person, 1f3c9a2e)", "e.g. Sarah — plays: Tennis"]) expect(html).toContain(text);
  });
});
