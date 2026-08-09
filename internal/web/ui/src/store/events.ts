// The wire vocabulary, mirroring internal/web/events.go exactly. If a field
// name changes there, it changes here — these types are the contract.

export type ServerEvent =
  | { type: "delta"; text: string }
  | { type: "reasoning"; text: string }
  | { type: "reasoning_done" }
  | { type: "assistant_done"; content: string }
  | { type: "tool_call"; id: string; name: string; args: string }
  | { type: "tool_result"; id: string; content: string; isError: boolean }
  | { type: "approval_request"; id: string; name: string; args: string }
  | { type: "error"; message: string }
  | { type: "turn_done" };

/** Event names we handle. Anything else on the wire is ignored on purpose:
 *  the whiteboard feature adds board_start/delta/end and critic_note to this
 *  same stream, and an older UI must not choke on them. */
const KNOWN = new Set([
  "delta",
  "reasoning",
  "reasoning_done",
  "assistant_done",
  "tool_call",
  "tool_result",
  "approval_request",
  "error",
  "turn_done",
]);

/** parseEvent turns one raw SSE block into a typed event, or null if the name
 *  is unknown or the payload isn't the JSON object we expect. */
export function parseEvent(name: string, data: string): ServerEvent | null {
  if (!KNOWN.has(name)) return null;
  let payload: unknown;
  try {
    payload = JSON.parse(data);
  } catch {
    return null;
  }
  if (typeof payload !== "object" || payload === null) return null;
  return { type: name, ...payload } as ServerEvent;
}
