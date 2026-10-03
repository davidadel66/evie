import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ApprovalCard } from "./ApprovalCard";
import { AlwaysAppliesContext } from "./alwaysApplies";
import { approvalAlternative, readApprovalArgs } from "./approvalArgs";
import { setApprovalAlwaysApplies, type Item } from "../store/reducer";

type Tool = Extract<Item, { kind: "tool" }>;

function rememberTool(kind: string | null, standing: boolean, approval: Tool["approval"] = { state: "pending", reqId: "live" }): Tool {
  const args: Record<string, unknown> = { scope: { scope_key: "global" }, subject: { canonical_name: "owner", anchor_kind: "owner" }, predicate: { label: "answer style" }, literal: { value: "concise answers" }, source: { authority: "owner_statement", evidence: "Remember that my answer style is concise answers." } };
  if (kind) args.classification = { kind, standing, bytes_per_token_milli: 3000 };
  return { kind: "tool", key: "remember", id: "call", name: "memory_remember_literal", args: JSON.stringify(args), startedAt: 0, approval };
}

const render = (tool: Tool, setter?: (reqId: string, value: boolean) => void) =>
  renderToStaticMarkup(<AlwaysAppliesContext.Provider value={setter}><ApprovalCard tool={tool} onAnswer={() => undefined} /></AlwaysAppliesContext.Provider>);

describe("Always applies on the memory approval card", () => {
  it("shows the proposed kind with the switch on for a preference", () => {
    const html = render(rememberTool("preference", true), () => undefined);
    for (const text of ['role="switch"', 'aria-checked="true"', "Always applies", "Preference", "every request"]) expect(html).toContain(text);
    expect(html).not.toContain("disabled");
  });

  it("shows the owner's switch, not the proposal's default, once touched", () => {
    const html = render(rememberTool("preference", true, { state: "pending", reqId: "live", alwaysApplies: false }), () => undefined);
    expect(html).toContain('aria-checked="false"');
    expect(html).toContain("only when it’s relevant");
  });

  it("starts off for a fact and is absent for a pre-standing proposal", () => {
    expect(render(rememberTool("fact", false), () => undefined)).toContain('aria-checked="false"');
    expect(render(rememberTool(null, false), () => undefined)).not.toContain('role="switch"');
  });

  it("leaves no switch on an answered card", () => {
    expect(render(rememberTool("preference", true, { state: "approved", reqId: "" }))).not.toContain('role="switch"');
  });

  it("shows the switch on correction and promotion cards that carry a classification", () => {
    for (const name of ["memory_correct_claim", "memory_promote_claim"]) {
      const tool: Tool = { kind: "tool", key: name, id: "call", name, args: JSON.stringify({ kind: name, classification: { kind: "instruction", standing: true, bytes_per_token_milli: 0 } }), startedAt: 0, approval: { state: "pending", reqId: "live" } };
      const html = render(tool, () => undefined);
      expect(html).toContain('role="switch"');
      expect(html).toContain("Instruction");
    }
  });

  it("states the change when an existing memory is marked standing", () => {
    const tool: Tool = { kind: "tool", key: "standing", id: "call", name: "memory_set_standing", args: JSON.stringify({ kind: "classify_claim", scope: { scope_key: "workspace:general" }, claim_text: "owner — answer style: concise answers", prior: { kind: "fact", standing: false, bytes_per_token_milli: 0 }, classification: { kind: "preference", standing: true, bytes_per_token_milli: 3000 } }), startedAt: 0, approval: { state: "pending", reqId: "live" } };
    const html = render(tool, () => undefined);
    for (const text of ["You — answer style: concise answers", "Preference · Always applies: Off → On", "was fact", "Workspace"]) expect(html).toContain(text);
    expect(html).not.toContain('role="switch"');
  });
});

describe("approving with the switch", () => {
  it("approves the prepared alternative only when the owner changed the switch", () => {
    const tool = rememberTool("preference", true);
    expect(approvalAlternative(tool.name, tool.args, undefined)).toBe(false);
    expect(approvalAlternative(tool.name, tool.args, true)).toBe(false);
    expect(approvalAlternative(tool.name, tool.args, false)).toBe(true);
    const legacy = rememberTool(null, false);
    expect(approvalAlternative(legacy.name, legacy.args, true)).toBe(false);
    expect(readApprovalArgs(tool.name, tool.args)).toMatchObject({ shape: "memory", classification: { kind: "preference", standing: true } });
  });

  it("records the switch only on the pending approval it belongs to", () => {
    const pending = rememberTool("preference", true);
    const other = { ...rememberTool("preference", true, { state: "pending", reqId: "other" }), key: "other" };
    const answered = { ...rememberTool("preference", true, { state: "approved", reqId: "done" }), key: "done" };
    const [changed, untouched, kept] = setApprovalAlwaysApplies([pending, other, answered], "live", false) as Tool[];
    expect(changed.approval?.alwaysApplies).toBe(false);
    expect(untouched.approval?.alwaysApplies).toBeUndefined();
    expect(setApprovalAlwaysApplies([answered], "done", false)[0]).toBe(answered);
    expect(kept.approval?.alwaysApplies).toBeUndefined();
  });
});
