import type { ToolItem } from "../chat/activityModel";
import { useMemoryPresentation } from "../memory/presentation";
import { readMemoryToolResult } from "./memoryToolResult";
import { MemoryResultView } from "./MemoryResultView";

export function ToolResultSummary({tool}: {tool: ToolItem}) {
  const {ownerName} = useMemoryPresentation();
  const memoryResult = readMemoryToolResult(tool, ownerName);
  if (memoryResult) return <MemoryResultView result={memoryResult} />;
  if (tool.result === undefined) return null;
  const text = tool.result.trim();
  let structured = false;
  try { structured = typeof JSON.parse(text) === "object"; } catch { /* Plain text is already readable. */ }
  if (text.startsWith("[begin untrusted semantic memory") || structured) return <p className="text-muted-text text-xs leading-5">A readable summary is unavailable for this recorded result. The exact result is preserved in the tool&apos;s Debug details.</p>;
  return <div className="space-y-2">
    <h3 className="text-muted-text text-xs">{tool.isErr ? "Error" : "Result"}</h3>
    <p className={`${tool.isErr ? "text-amber-ink" : "text-body"} max-h-64 overflow-auto text-sm leading-6 whitespace-pre-wrap break-words`}>{text.slice(0, 1200) || "The tool returned an empty result."}</p>
    {text.length > 1200 && <p className="text-muted-text text-xs">Preview shortened. The full result is in Debug details.</p>}
  </div>;
}
