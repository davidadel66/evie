// Reading a gated tool's args well enough to show David what he's approving.
// Two tools are gated today (internal/tools/registry.go): edit_file and
// edit_db. Anything else gets pretty-printed JSON — correct, if plain.

export type ApprovalView =
  | { shape: "diff"; subject: string; oldText: string; newText: string }
  | { shape: "statement"; subject: string; statement: string }
  | { shape: "json"; subject: string; json: string };

export function readApprovalArgs(name: string, args: string): ApprovalView {
  const parsed = parseObject(args);

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
