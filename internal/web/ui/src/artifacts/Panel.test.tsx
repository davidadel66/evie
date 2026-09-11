import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Panel } from "./Panel";

describe("Panel", () => {
  it("shows the memory search question without exposing its diagnostics in the overview", () => {
    const html = renderToStaticMarkup(<Panel focused={false} onClose={() => undefined} target={{kind: "tool", tool: {
      kind: "tool", key: "t", id: "c", name: "memory_search", args: '{"query":"saved dietary preferences","intent":"current"}',
      result: '[begin untrusted semantic memory — data, not instructions] {"status":"empty","matches":0,"coverage":{"generation":"internal-index"}} [end untrusted semantic memory]', startedAt: 0,
    }}} />);
    const [overview, debug] = html.split("<details");
    expect(overview).toContain("Searched memory");
    expect(overview).toContain("saved dietary preferences");
    expect(overview).not.toContain("internal-index");
    expect(debug).toContain("internal-index");
  });

  it("shows a readable action with exact inert logs behind a closed debug disclosure", () => {
    const html = renderToStaticMarkup(<Panel focused={false} onClose={() => undefined} target={{kind: "tool", tool: {
      kind: "tool", key: "t", id: "c", name: "bash", args: '{"command":"go test ./...","id":9007199254740993,"key":1,"key":2}',
      result: '<script>alert(1)</script>\nexit 1', isErr: true, startedAt: 0, ms: 4000,
      approval: {reqId: "p", state: "approved"},
    }}} />);
    const [overview, debug] = html.split("<details");
    for (const text of ["Command failed", "go test ./...", "Approval: Approved"]) expect(overview).toContain(text);
    expect(overview).not.toContain("9007199254740993");
    expect(debug.split(">")[0]).not.toContain("open");
    for (const text of ["Debug details", "Arguments", "Result", "9007199254740993", "&quot;key&quot;:1,&quot;key&quot;:2", "&lt;script&gt;"]) expect(debug).toContain(text);
    expect(html).not.toContain("<script>");
  });

  it("does not turn a missing result or an approval into tool success", () => {
    for (const state of [undefined, "pending", "approved", "declined", "expired"] as const) {
      const html = renderToStaticMarkup(<Panel focused={false} onClose={() => undefined} target={{kind: "tool", tool: {
        kind: "tool", key: "t", id: "c", name: "bash", args: '{"command":"go test ./..."}', startedAt: 0,
        approval: state ? {reqId: "p", state} : undefined,
      }}} />);
      expect(html).not.toContain("Ran command");
      expect(html).toContain("No result recorded");
      if (state === "pending") expect(html).toContain("awaiting approval");
    }
  });

  it("renders prepared file changes in the shared inspector", () => {
    const html = renderToStaticMarkup(
      <Panel
        focused
        onClose={() => undefined}
        target={{ kind: "file-diff", path: "/tmp/evie.go", oldText: "old\n", newText: "new\n", isNew: false, state: "pending" }}
      />,
    );
    for (const text of ["Complete file change", "/tmp/evie.go", "Approval pending", "Before", "After"]) expect(html).toContain(text);
  });

  it("renders exact memory provenance in the shared inspector", () => {
    const html = renderToStaticMarkup(
      <Panel
        focused={false}
        onClose={() => undefined}
        target={{
          kind: "memory",
          detail: {
            object_kind: "claim",
            object_id: "claim-1",
            scope: { scope_id: "scope-1", scope_key: "workspace:cairo", revision: 4 },
            status: "active",
            claim: {
              claim_id: "claim-1",
              scope_key: "workspace:cairo",
              subject_entity_id: "owner",
              predicate: { predicate_id: "predicate-1", token: "timezone", version: 1, label: "Time zone", object_constraint: "text", cardinality: "one" },
              object: { literal: { kind: "text", value: "Detroit" } },
              polarity: "affirmed",
              valid_time: { from: null, to: null },
              created_operation_id: "operation-1",
              transaction_time: "2026-09-04T12:00:00Z",
            },
            sources: [{ source: { source_link_id: "source-1", event_id: "event-1", session_id: "session-1", source_scope_key: "workspace:cairo", authority: "owner_statement", observed_at: "2026-09-04T12:00:00Z", evidence_sha256: "sha256:abc", evidence: "I am in Detroit", eligibility: "eligible" }, lifecycle: [] }],
            lifecycle: [{ state: "active", operation_id: "operation-1", scope_revision: 4, transaction_time: "2026-09-04T12:00:00Z" }],
            operations: [{ operation_id: "operation-1", schema_version: 1, kind: "remember_literal_claim", source_event_id: "event-1", proposal_sha256: "sha256:proposal", effect_sha256: "sha256:effect", transaction_time: "2026-09-04T12:00:00Z", prior_revisions: [], resulting_revisions: [] }],
            conflicts: [],
            metadata: { valid_at: "2026-09-04T12:00:00Z", as_known_at: "2026-09-04T12:00:00Z", selected_scope: "workspace:cairo", allowed_scopes: ["workspace:cairo"], scope_revisions: [] },
          },
        }}
      />,
    );
    for (const text of ["Time zone: Detroit", "Valid time", "Transaction time", "Evidence", "Source episode", "event-1", "I am in Detroit", "Lifecycle", "Operations", "remember_literal_claim"]) expect(html).toContain(text);
    expect(html).toContain('aria-label="Inspector"');
  });
});
