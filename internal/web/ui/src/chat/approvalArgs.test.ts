import { describe, expect, it } from "vitest";
import { readApprovalArgs } from "./approvalArgs";

describe("readApprovalArgs", () => {
  it("reads edit_file into a diff", () => {
    const view = readApprovalArgs(
      "edit_file",
      JSON.stringify({
        path: "/tmp/a.yaml",
        old_string: "ramp: none",
        new_string: "ramp: { minutes: 90 }",
      }),
    );
    expect(view).toEqual({
      shape: "diff",
      subject: "/tmp/a.yaml",
      oldText: "ramp: none",
      newText: "ramp: { minutes: 90 }",
    });
  });

  it("reads edit_db into a statement", () => {
    const view = readApprovalArgs(
      "edit_db",
      JSON.stringify({ db: "finance", statement: "DELETE FROM x WHERE id=1" }),
    );
    expect(view).toEqual({
      shape: "statement",
      subject: "finance",
      statement: "DELETE FROM x WHERE id=1",
    });
  });

  it("falls back to JSON for an unknown gated tool", () => {
    const view = readApprovalArgs("some_future_tool", '{"a":1}');
    expect(view.shape).toBe("json");
    expect(view.shape === "json" && view.json).toContain('"a": 1');
  });

  it("falls back to JSON when edit_file args are incomplete", () => {
    // A malformed call must stay reviewable rather than render an empty diff.
    const view = readApprovalArgs("edit_file", '{"path":"/tmp/a"}');
    expect(view.shape).toBe("json");
  });

  it("shows unparseable args verbatim rather than hiding them", () => {
    const view = readApprovalArgs("edit_file", "{not json");
    expect(view).toEqual({ shape: "json", subject: "", json: "{not json" });
  });
});

it("keeps the subject, relationship, value and polarity visible in memory approvals", () => {
  for (const predicate of ["prefers", "avoids"]) {
    const view = readApprovalArgs("memory_remember_literal", JSON.stringify({ scope: { scope_key: "global" }, subject: { canonical_name: "David" }, predicate: { label: predicate }, literal: { value: "coffee" }, polarity: "denied", source: { evidence: "My statement" } }));
    expect(view).toMatchObject({ shape: "memory", subject: `Not: David · ${predicate} · coffee`, scopeKey: "global", evidence: "My statement" });
  }
});

it("says when a proposed memory is not in the owner's words", () => {
  const view = readApprovalArgs("memory_remember_literal", JSON.stringify({ scope: { scope_key: "global" }, subject: { canonical_name: "owner", anchor_kind: "owner" }, predicate: { label: "preferred payee" }, literal: { value: "Acme Offshore" }, source: { authority: "evie_proposed", evidence: "" } }));
  expect(view).toMatchObject({ shape: "memory", subject: "You · preferred payee · Acme Offshore", evieProposed: true, evidence: "" });
  const owned = readApprovalArgs("memory_remember_literal", JSON.stringify({ scope: { scope_key: "global" }, subject: { canonical_name: "owner", anchor_kind: "owner" }, predicate: { label: "favorite color" }, literal: { value: "teal" }, source: { authority: "owner_statement", evidence: "My favorite color is teal." } }));
  expect(owned).toMatchObject({ evieProposed: false, evidence: "My favorite color is teal." });
});

it("lists the Entities an entity memory reuses or creates", () => {
  const view = readApprovalArgs("memory_remember_entity", JSON.stringify({
    scope: { scope_key: "global" }, predicate: { label: "loves" }, claim: { subject_entity_id: "s", object_entity_id: "o" },
    entities: [{ entity_id: "s", canonical_name: "Sarah" }, { entity_id: "o", canonical_name: "Chess" }],
    source: { authority: "owner_statement", evidence: "Sarah also loves chess." },
    identities: [
      { role: "subject", entity_id: "1f3c9a2e-0000-4000-8000-000000000001", canonical_name: "Sarah", entity_type: "person", reused: true, selected_by: "alias", aliases: ["Sarah", "Sis"], example_claim: "Sarah — plays: Tennis", same_name: 1 },
      { role: "object", entity_id: "o", canonical_name: "Chess", entity_type: "game", reused: false, selected_by: "create", aliases: ["chess"] },
    ],
  }));
  expect(view.shape === "memory" && view.identities).toEqual([
    "Reuses existing Sarah (person, 1f3c9a2e) · matched by name · also called Sarah, Sis · e.g. Sarah — plays: Tennis · 1 other entity shares this name",
    "Creates new Chess (game)",
  ]);
});
