// The lines a review question asks about, and the answer the deck builds
// from the owner's decisions (run detail amendment, 2026-11-02). Pure, so
// the deck component's file exports only the component.

import type { WorkflowInterruption } from "../api/workflows";

export type ReviewLine = { id: string; fields: Record<string, unknown> };
export type ReviewDecision = { kind: "keep" } | { kind: "reject" } | { kind: "change"; category: string };

/** The list a review question asks about, when its answer schema takes reject, change or no_rule lists. */
export function reviewable(interruption: WorkflowInterruption): { key: string; lines: ReviewLine[] } | undefined {
  if (interruption.kind !== "question") return undefined;
  const properties = interruption.response_schema.properties;
  if (!properties || typeof properties !== "object") return undefined;
  const names = Object.keys(properties as Record<string, unknown>);
  if (!names.some((name) => name === "reject" || name === "change" || name === "no_rule")) return undefined;
  const input = interruption.request.input;
  const source = input && typeof input === "object" ? (input as Record<string, unknown>) : interruption.request;
  for (const [key, value] of Object.entries(source)) {
    if (!Array.isArray(value) || value.length === 0) continue;
    const lines = value.filter((item): item is Record<string, unknown> => !!item && typeof item === "object" && typeof (item as Record<string, unknown>).id === "string")
      .map((item) => ({id: item.id as string, fields: item}));
    if (lines.length === value.length) return {key, lines};
  }
  return undefined;
}

/** The step's answer from the owner's decisions: only the exceptions, and only the lists with something in them. */
export function reviewAnswer(lines: ReviewLine[], decisions: Record<string, ReviewDecision>, noRule: Set<string>): Record<string, unknown> {
  const answer: Record<string, unknown> = {};
  const reject = lines.filter((line) => decisions[line.id]?.kind === "reject").map((line) => line.id);
  const change = lines.flatMap((line) => { const d = decisions[line.id]; return d?.kind === "change" ? [{id: line.id, category: d.category}] : []; });
  const dropped = lines.filter((line) => noRule.has(line.id) && decisions[line.id]?.kind !== "reject").map((line) => line.id);
  if (reject.length) answer.reject = reject;
  if (change.length) answer.change = change;
  if (dropped.length) answer.no_rule = dropped;
  return answer;
}

/** "15 kept, 2 rejected, 1 changed · 3 without a rule". */
export function reviewSummary(lines: ReviewLine[], decisions: Record<string, ReviewDecision>, noRule: Set<string>): string {
  const count = (kind: ReviewDecision["kind"]) => lines.filter((line) => decisions[line.id]?.kind === kind).length;
  const parts = [`${count("keep")} kept`];
  if (count("reject")) parts.push(`${count("reject")} rejected`);
  if (count("change")) parts.push(`${count("change")} changed`);
  const dropped = lines.filter((line) => noRule.has(line.id) && decisions[line.id]?.kind !== "reject").length;
  return parts.join(", ") + (dropped ? ` · ${dropped} without a rule` : "");
}

/** The categories the proposals use, for the Change menu; the step's input carries no vocabulary. */
export function seenCategories(lines: ReviewLine[]): string[] {
  return [...new Set(lines.map((line) => line.fields.category).filter((value): value is string => typeof value === "string" && value !== ""))].sort();
}

