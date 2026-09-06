// Reading a gated tool's args well enough to show David what he's approving.
// Two tools are gated today (internal/tools/registry.go): edit_file and
// edit_db. Anything else gets pretty-printed JSON — correct, if plain.

export type ApprovalView =
  | { shape: "memory"; subject: string; scopeKey: string; evidence: string; json: string }
  | { shape: "diff"; subject: string; oldText: string; newText: string }
  | { shape: "statement"; subject: string; statement: string }
  | { shape: "json"; subject: string; json: string };

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
    const subject = [label(object(parsed.subject)) || label(entities.find(e => e?.entity_id === claim?.subject_entity_id)), predicate?.label, value || label(entities.find(e => e?.entity_id === claim?.object_entity_id))].filter(Boolean).join(" · ");
    if (scope && str(scope.scope_key) && subject) return { shape: "memory", subject: `${polarity === "denied" ? "Not: " : ""}${subject}`, scopeKey: str(scope.scope_key), evidence: str(source?.evidence), json: pretty(args) };
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
