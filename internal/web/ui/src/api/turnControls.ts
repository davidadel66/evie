// Turn controls that travel beside the chat stream: stopping the running turn
// and compacting the conversation's context. Both are bound to the displayed
// conversation, so a stale tab can never act on a different one.

import { ApiError } from "./stream";

type ErrorBody = { code?: unknown; error?: unknown; classification?: unknown };

async function post(path: string, sessionId?: string): Promise<{ status: number; body: Record<string, unknown> & ErrorBody }> {
  const res = await fetch(path, {
    method: "POST",
    // Exactly this content type: the Go guard requires it.
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sessionId }),
  });
  let body: Record<string, unknown> = {};
  try {
    const parsed: unknown = await res.json();
    if (parsed && typeof parsed === "object") body = parsed as Record<string, unknown>;
  } catch {
    // An empty or non-JSON body falls through to the status line below.
  }
  return { status: res.status, body };
}

function refusal(status: number, body: ErrorBody, fallback: string): ApiError {
  return new ApiError(status, typeof body.error === "string" ? body.error : `${fallback} (${status})`);
}

/** cancelTurn asks the server to stop the conversation's running turn. The
 *  turn itself reports the interruption on its own stream; "idle" means there
 *  was nothing left to stop, usually because the turn just finished. */
export async function cancelTurn(sessionId?: string): Promise<"stopping" | "idle"> {
  const { status, body } = await post("/api/cancel", sessionId);
  if (status === 202) return "stopping";
  if (status === 409 && body.code === "no_active_turn") return "idle";
  throw refusal(status, body, "stop failed");
}

export type CompactOutcome =
  | { outcome: "compacted"; eventId: string }
  | { outcome: "nothing_to_compact" }
  | { outcome: "failed"; classification: string }
  | { outcome: "refused"; code: string; message: string };

/** compactSession runs the agent's manual compaction for the conversation. */
export async function compactSession(sessionId?: string): Promise<CompactOutcome> {
  const { status, body } = await post("/api/compact", sessionId);
  if (status === 200 && body.outcome === "compacted" && typeof body.eventId === "string") {
    return { outcome: "compacted", eventId: body.eventId };
  }
  if (status === 200 && body.outcome === "nothing_to_compact") return { outcome: "nothing_to_compact" };
  if (body.code === "compaction_failed" && typeof body.classification === "string") {
    return { outcome: "failed", classification: body.classification };
  }
  if ((status === 409 || status === 503) && typeof body.code === "string" && typeof body.error === "string") {
    return { outcome: "refused", code: body.code, message: body.error };
  }
  throw refusal(status, body, "compaction failed");
}

export type ComposerNotice = { tone: "info" | "warning"; text: string };

const failureLabels: Record<string, string> = {
  provider_error: "provider error",
  provider_response_invalid: "invalid summary",
  context_overflow: "context overflow",
  local_failure: "local error",
};

/** compactionNotice is the one line the composer shows for an outcome. */
export function compactionNotice(result: CompactOutcome): ComposerNotice {
  switch (result.outcome) {
    case "compacted":
      return { tone: "info", text: "Context compacted." };
    case "nothing_to_compact":
      return { tone: "info", text: "Nothing eligible for compaction yet." };
    case "failed": {
      const label = failureLabels[result.classification] ?? result.classification.replace(/_/g, " ");
      return { tone: "warning", text: `Compaction failed (${label}). The conversation is unchanged.` };
    }
    case "refused":
      if (result.code === "turn_in_progress") return { tone: "warning", text: "Finish or stop the current turn before compacting." };
      return { tone: "warning", text: result.message };
  }
}
