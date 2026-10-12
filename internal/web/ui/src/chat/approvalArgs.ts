// Reading a gated tool's args well enough to show David what he's approving.
// Two tools are gated today (internal/tools/registry.go): edit_file and
// edit_db. Anything else gets pretty-printed JSON — correct, if plain.

/** A memory's kind and whether it always applies (the standing tier). */
export type MemoryClassification = { kind: string; standing: boolean };

export type ApprovalView =
  | { shape: "memory"; subject: string; statement: string; scopeKey: string; evidence: string; evieProposed: boolean; identities: string[]; json: string; classification?: MemoryClassification; standingChange?: { from: MemoryClassification; to: MemoryClassification } }
  | { shape: "diff"; subject: string; oldText: string; newText: string }
  | { shape: "statement"; subject: string; statement: string }
  | { shape: "json"; subject: string; json: string; evieProposed?: boolean; classification?: MemoryClassification };

const kindLabels: Record<string, string> = { preference: "Preference", instruction: "Instruction", profile_fact: "Profile fact", workspace_brief: "Workspace brief", fact: "Fact" };

/** kindLabel names a memory kind for the approval card. */
export function kindLabel(kind: string): string {
  return kindLabels[kind] ?? kind;
}

function readClassification(value: unknown): MemoryClassification | undefined {
  const classification = object(value);
  if (!classification || typeof classification.standing !== "boolean" || !str(classification.kind)) return undefined;
  return { kind: str(classification.kind), standing: classification.standing };
}

/** approvalAlternative says whether approving with the card's "Always
 *  applies" switch at alwaysApplies approves the prepared alternative (the
 *  same memory with standing switched) rather than the proposal as shown.
 *  An untouched switch (undefined) approves the proposal as shown. */
export function approvalAlternative(name: string, args: string, alwaysApplies: boolean | undefined): boolean {
  const view = readApprovalArgs(name, args);
  const classification = view.shape === "memory" || view.shape === "json" ? view.classification : undefined;
  return classification !== undefined && alwaysApplies !== undefined && alwaysApplies !== classification.standing;
}

export function readApprovalArgs(name: string, args: string, ownerName = "You"): ApprovalView {
  const parsed = parseObject(args);

  if ((name === "memory_remember_literal" || name === "memory_remember_entity") && parsed) {
    const scope = object(parsed.scope);
    const source = object(parsed.source);
    const literal = object(parsed.literal);
    const claim = object(parsed.claim);
    const predicate = object(parsed.predicate);
    const polarity = str(parsed.polarity) || str(claim?.polarity);
    const value = str(literal?.value);
    const entities = Array.isArray(parsed.entities) ? parsed.entities.map(object) : [];
    const label = (entity: Record<string, unknown> | null | undefined) => entity?.anchor_kind === "owner" && entity?.canonical_name === "owner" ? ownerName : str(entity?.canonical_name);
    const subjectName = label(object(parsed.subject)) || label(entities.find(e => e?.entity_id === claim?.subject_entity_id));
    const objectText = value || label(entities.find(e => e?.entity_id === claim?.object_entity_id));
    const subject = [subjectName, predicate?.label, objectText].filter(Boolean).join(" · ");
    // The memory as one readable line, in the Claim text form the memory
    // list uses: "David — answer style: short answers".
    const statement = subjectName && str(predicate?.label) && objectText ? `${subjectName} — ${str(predicate?.label)}: ${objectText}` : subject;
    // Harness review M5/M6: an Evie-proposed value has no quote from the
    // owner, and each reused or created Entity is named with what tells
    // same-named Entities apart.
    const evieProposed = str(source?.authority) === "evie_proposed";
    const identities = Array.isArray(parsed.identities) ? parsed.identities.map(object).filter((identity) => identity !== null).map(identityLine) : [];
    const classification = readClassification(parsed.classification);
    if (scope && str(scope.scope_key) && subject) return { shape: "memory", subject: `${polarity === "denied" ? "Not: " : ""}${subject}`, statement: `${polarity === "denied" ? "Not: " : ""}${statement}`, scopeKey: str(scope.scope_key), evidence: evieProposed ? "" : str(source?.evidence), evieProposed, identities, json: pretty(args), ...(classification ? { classification } : {}) };
  }

  if (name === "memory_correct_claim" && parsed) {
    // A replacement value not in the owner's words is Evie-proposed (M5).
    const classification = readClassification(parsed.classification);
    return { shape: "json", subject: "", json: pretty(args), evieProposed: str(object(parsed.source)?.authority) === "evie_proposed", ...(classification ? { classification } : {}) };
  }

  if (name === "memory_promote_claim" && parsed) {
    const classification = readClassification(parsed.classification);
    if (classification) return { shape: "json", subject: "", json: pretty(args), classification };
  }

  if (name === "memory_set_standing" && parsed) {
    // Marking an existing memory standing: the change itself is the decision.
    const from = readClassification(parsed.prior);
    const to = readClassification(parsed.classification);
    const text = str(parsed.claim_text);
    const scopeKey = str(object(parsed.scope)?.scope_key);
    if (from && to && text && scopeKey) {
      const subject = text.startsWith("owner — ") ? `${ownerName} — ${text.slice("owner — ".length)}` : text;
      return { shape: "memory", subject, statement: subject, scopeKey, evidence: "", evieProposed: false, identities: [], json: pretty(args), standingChange: { from, to } };
    }
  }

  if (name === "edit_file" && parsed) {
    const path = str(parsed.path);
    const oldText = str(parsed.old_string);
    const newText = str(parsed.new_string);
    // Only claim the diff shape if the fields are actually there; a
    // malformed call must still be reviewable rather than render an empty diff.
    if (path && (oldText || newText)) {
      return { shape: "diff", subject: path, oldText, newText };
    }
  }

  if (name === "edit_db" && parsed) {
    const statement = str(parsed.statement);
    if (statement) {
      return {
        shape: "statement",
        subject: str(parsed.db) || "database",
        statement,
      };
    }
  }

  return { shape: "json", subject: "", json: pretty(args) };
}

function parseObject(args: string): Record<string, unknown> | null {
  try {
    const v: unknown = JSON.parse(args);
    return typeof v === "object" && v !== null
      ? (v as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

function str(v: unknown): string {
  return typeof v === "string" ? v : "";
}

/** pretty re-indents JSON args, falling back to the raw string when the model
 *  sent something unparseable — David still needs to see it. */
function pretty(args: string): string {
  try {
    return JSON.stringify(JSON.parse(args), null, 2);
  } catch {
    return args;
  }
}

function object(v: unknown): Record<string, unknown> | null { return typeof v === "object" && v !== null ? v as Record<string, unknown> : null; }

function identityLine(identity: Record<string, unknown>): string {
  const name = str(identity.canonical_name);
  const type = str(identity.entity_type);
  if (identity.reused !== true) {
    const others = typeof identity.same_name === "number" && identity.same_name > 0 ? ` · ${identity.same_name} existing ${identity.same_name === 1 ? "entity has" : "entities have"} this name` : "";
    return `Creates new ${name} (${type})${others}`;
  }
  const parts = [`Reuses existing ${name} (${type}, ${str(identity.entity_id).split("-")[0]})`];
  parts.push(identity.selected_by === "entity_id" ? "chosen by ID" : "matched by name");
  const aliases = Array.isArray(identity.aliases) ? identity.aliases.filter((alias): alias is string => typeof alias === "string") : [];
  if (aliases.length > 0) parts.push(`also called ${aliases.join(", ")}`);
  if (str(identity.example_claim)) parts.push(`e.g. ${str(identity.example_claim)}`);
  if (typeof identity.same_name === "number" && identity.same_name > 0) parts.push(`${identity.same_name} other ${identity.same_name === 1 ? "entity shares" : "entities share"} this name`);
  return parts.join(" · ");
}
