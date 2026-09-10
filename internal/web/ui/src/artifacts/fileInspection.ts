import type { Item } from "../store/reducer";

type Tool = Extract<Item, { kind: "tool" }>;
export type FileSelection = { sessionId: string; key: string };
export type FileInspection = {
  key: string;
  path: string;
  status: "Read" | "Edited" | "Requested" | "Awaiting approval" | "Approved; awaiting result" | "Declined" | "Expired" | "Failed";
  approvalState?: NonNullable<Tool["approval"]>["state"];
  tool?: { name: string; args: string; result?: string; isErr?: boolean };
  content:
    | { kind: "code"; text: string; coverage: "full" | "excerpt" }
    | { kind: "change"; before: string; after: string; isNew: boolean; coverage: "full" | "replacement" }
    | { kind: "unavailable"; message: string };
};

function fileArguments(tool: Tool): Record<string, unknown> | null {
  try {
    const args: unknown = JSON.parse(tool.args);
    return args && typeof args === "object" && !Array.isArray(args) ? args as Record<string, unknown> : null;
  } catch { return null; }
}

export function toolFilePath(tool: Tool): string | null {
  if (tool.approval?.preview) return tool.approval.preview.path;
  if (tool.name !== "read_file" && tool.name !== "edit_file") return null;
  const path = fileArguments(tool)?.path;
  return typeof path === "string" && path.trim() ? path : null;
}

// read_file wraps every source line in a sequential prefix and a separator.
// The final numbered empty line preserves an original final newline.
export function decodeFileRead(result: string): { text: string; coverage: "full" | "excerpt" } {
  const excerpt = {text: result, coverage: "excerpt" as const};
  if (!result.endsWith("\n")) return excerpt;
  const lines = result.slice(0, -1).split("\n");
  const decoded: string[] = [];
  for (let i = 0; i < lines.length; i++) {
    const prefix = `${String(i + 1).padStart(6)}\t`;
    if (!lines[i].startsWith(prefix)) return excerpt;
    decoded.push(lines[i].slice(prefix.length));
  }
  return {text: decoded.join("\n"), coverage: "full"};
}

export function inspectToolFile(tool: Tool): FileInspection | null {
  const path = toolFilePath(tool);
  if (!path) return null;
  const status = fileStatus(tool);
  const base = {key: tool.key, path, status, approvalState: tool.approval?.state, tool: {name: tool.name, args: tool.args, result: tool.result, isErr: tool.isErr}};
  if (tool.name === "read_file" && !tool.approval?.preview) {
    if (tool.result === undefined || tool.isErr) return {
      ...base, content: {kind: "unavailable", message: tool.result ?? "The file read has not returned yet."},
    };
    return {...base, content: {kind: "code", ...decodeFileRead(tool.result)}};
  }
  const preview = tool.approval?.preview;
  if (preview) return {...base, content: {kind: "change", before: preview.oldText, after: preview.newText, isNew: preview.isNew, coverage: "full"}};
  const args = fileArguments(tool);
  if (typeof args?.old_string === "string" && typeof args.new_string === "string") {
    return {...base, content: {kind: "change", before: args.old_string, after: args.new_string, isNew: false, coverage: "replacement"}};
  }
  return {...base, content: {kind: "unavailable", message: "No file contents or replacement text were recorded for this action."}};
}

function fileStatus(tool: Tool): FileInspection["status"] {
  if (tool.approval?.state === "declined") return "Declined";
  if (tool.approval?.state === "expired") return "Expired";
  if (tool.isErr) return "Failed";
  if (tool.result !== undefined) return tool.name === "read_file" ? "Read" : "Edited";
  if (tool.approval?.state === "pending") return "Awaiting approval";
  if (tool.approval?.state === "approved") return "Approved; awaiting result";
  return "Requested";
}

export function selectedFileInspection(selection: FileSelection | undefined, sessionId: string | undefined, items: Item[]): FileInspection | null {
  if (!selection || selection.sessionId !== sessionId) return null;
  const tool = items.find((item) => item.key === selection.key);
  return tool?.kind === "tool" ? inspectToolFile(tool) : null;
}
