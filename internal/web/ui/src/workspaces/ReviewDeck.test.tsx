import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { WorkflowInterruption } from "../api/workflows";
import { ReviewDeck } from "./ReviewDeck";
import { reviewAnswer, reviewSummary, reviewable, seenCategories, type ReviewLine } from "./reviewLines";

// The review deck (run detail amendment, 2026-11-02): a review question's
// lines as cards, and an answer that is only the exceptions.

const lines: ReviewLine[] = [
  {id: "a", fields: {id: "a", merchant_name: "Wex Health", name: "*1845 REPAYMENT WEX HEALTH", net_cents: -7800, date: "2026-10-14", category: "Health", confidence: 0.6, reason: "A health benefits administrator.", rule: true}},
  {id: "b", fields: {id: "b", merchant_name: "Sava's", name: "TST*SAVAS", net_cents: -11734, date: "2026-08-22", category: "Dining", confidence: 0.9, reason: "A restaurant.", rule: true}},
  {id: "c", fields: {id: "c", merchant_name: "Fandango", name: "Fandango", net_cents: -5600, date: "2026-08-13", category: "Entertainment", confidence: 0.9, reason: "Movie tickets.", rule: false}},
];
const schema = {type: "object", properties: {reject: {type: "array"}, change: {type: "array"}, no_rule: {type: "array"}}};
const question = (request: Record<string, unknown>, response_schema: Record<string, unknown> = schema): WorkflowInterruption =>
  ({run_id: "r", seq: 1, workspace_id: "w", workflow: "finance-review", kind: "question", step: "review", checkpoint: 6, message: "Review.", request, response_schema, opened_at: "2026-11-02T09:30:00Z"});

describe("which questions get a deck", () => {
  it("is one whose input lists objects with ids and whose answer names reject, change or no_rule", () => {
    expect(reviewable(question({input: {lines: lines.map((l) => l.fields)}}))?.lines.map((l) => l.id)).toEqual(["a", "b", "c"]);
    expect(reviewable(question({input: {lines: lines.map((l) => l.fields)}}, {type: "object", properties: {choice: {enum: ["x"]}}}))).toBeUndefined();
    expect(reviewable(question({input: {lines: [1, 2]}}))).toBeUndefined();
    expect(reviewable(question({input: {}}))).toBeUndefined();
    expect(reviewable({...question({}), kind: "effect_reconciliation"})).toBeUndefined();
  });
});

describe("the answer the deck builds", () => {
  it("lists only the exceptions, and leaves out empty lists", () => {
    expect(reviewAnswer(lines, {a: {kind: "keep"}, b: {kind: "keep"}, c: {kind: "keep"}}, new Set())).toEqual({});
    expect(reviewAnswer(lines, {a: {kind: "reject"}, b: {kind: "change", category: "Groceries"}, c: {kind: "keep"}}, new Set(["a", "b"])))
      .toEqual({reject: ["a"], change: [{id: "b", category: "Groceries"}], no_rule: ["b"]});
  });
  it("sums up the decisions and knows the categories in play", () => {
    expect(reviewSummary(lines, {a: {kind: "reject"}, b: {kind: "change", category: "Groceries"}, c: {kind: "keep"}}, new Set(["b"]))).toBe("1 kept, 1 rejected, 1 changed · 1 without a rule");
    expect(seenCategories(lines)).toEqual(["Dining", "Entertainment", "Health"]);
  });
});

describe("the deck's first card", () => {
  const html = renderToStaticMarkup(<ReviewDeck lines={lines} busy={false} onAnswer={() => {}} onClose={() => {}} />);
  it("shows the merchant, amount, date, category with its confidence, the reason and the rule", () => {
    expect(html).toContain("1 of 3");
    expect(html).toContain("Wex Health");
    expect(html).toContain("−$78.00");
    expect(html).toContain("Oct 14");
    expect(html).toMatch(/<button[^>]*>Health<\/button>/);
    expect(html).toContain("60% sure");
    expect(html).toContain("A health benefits administrator.");
    expect(html).toMatch(/aria-pressed="false"[^>]*>Merchant rule</);
    expect(html).not.toContain("Sava");
  });
  it("offers Reject, Skip and Keep with their keys", () => {
    expect([...html.matchAll(/aria-label="(Reject|Skip|Keep)" title="[^"]*\((←|↑|→)\)"/g)].map((m) => [m[1], m[2]])).toEqual([["Reject", "←"], ["Skip", "↑"], ["Keep", "→"]]);
    expect(html).toContain("0 kept");
  });
});
