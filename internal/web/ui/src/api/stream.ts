// The downstream half of the protocol. EventSource can't POST, so we read the
// fetch body as a stream and parse `event:` / `data:` blocks by hand — the
// mirror image of internal/openrouter/client.go:40-129.

import { parseEvent, type ServerEvent } from "../store/events";

/** Raised for a non-2xx reply, carrying the server's {"error"} text (409 busy,
 *  403 guard, 400 bad body all arrive this way) so the banner can show it. */
export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

/** Raised when the body ends without a turn_done — a dropped connection.
 *  The server always emits turn_done last, even on failure, so a missing one
 *  means the stream died rather than finished. */
export class StreamTruncated extends Error {
  constructor() {
    super("Lost connection to the Evie server");
    this.name = "StreamTruncated";
  }
}

/** streamChat POSTs one message and invokes onEvent for each parsed event, in
 *  wire order, as it arrives. Resolves after turn_done; rejects with ApiError
 *  or StreamTruncated. Unknown event names are skipped silently. */
export async function streamChat(
  message: string,
  onEvent: (ev: ServerEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch("/api/chat", {
    method: "POST",
    // Exactly this content type: the Go guard requires it, because an HTML
    // form can't produce it and bash is ungated.
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ message }),
    signal,
  });

  if (!res.ok) throw new ApiError(res.status, await errorText(res));
  if (!res.body) throw new StreamTruncated();

  const reader = res.body
    .pipeThrough(new TextDecoderStream())
    .getReader();

  // Blocks are separated by a blank line; a chunk can split one anywhere, so
  // whatever follows the last separator stays buffered for the next chunk.
  let buffer = "";
  let sawTurnDone = false;

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += value;

    let sep: number;
    while ((sep = buffer.indexOf("\n\n")) !== -1) {
      const block = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);

      const ev = parseBlock(block);
      if (!ev) continue;
      if (ev.type === "turn_done") sawTurnDone = true;
      onEvent(ev);
    }
  }

  if (!sawTurnDone) throw new StreamTruncated();
}

/** parseBlock reads the `event:`/`data:` lines of one block. Multiple data
 *  lines concatenate, per the SSE spec — our server never emits them, but
 *  following the spec costs one line. */
function parseBlock(block: string): ServerEvent | null {
  let name = "";
  let data = "";
  for (const line of block.split("\n")) {
    if (line.startsWith("event:")) name = line.slice(6).trim();
    else if (line.startsWith("data:")) data += line.slice(5).trim();
  }
  if (!name || !data) return null;
  return parseEvent(name, data);
}

/** errorText pulls the message out of the server's {"error": "..."} body,
 *  falling back to the status line when the body isn't that shape. */
async function errorText(res: Response): Promise<string> {
  try {
    const body = await res.json();
    if (body && typeof body.error === "string") return body.error;
  } catch {
    // fall through
  }
  return `request failed (${res.status})`;
}
