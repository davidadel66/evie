import { folderRequest, type FolderRef } from "./localFolder";

export type TerminalEvent = { type: "ready"; id: string } | { type: "output"; data: string } | { type: "exit"; code?: number };

// Await delivery before reading another chunk: xterm, rather than network
// speed, bounds the amount of output buffered by this client.
export async function readTerminalStream(response: Response, signal: AbortSignal, deliver: (event: TerminalEvent) => Promise<void>): Promise<void> {
  if (!response.ok) {
    const body = await response.json();
    throw new Error(body.error ?? "The terminal could not be opened.");
  }
  if (!response.body) throw new Error("Terminal output is unavailable.");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let pending = "", exited = false, ready = false;
  try {
    while (!signal.aborted) {
      const { value, done } = await reader.read();
      if (done) break;
      pending += decoder.decode(value, { stream: true });
      let newline: number;
      while ((newline = pending.indexOf("\n")) >= 0) {
        const line = pending.slice(0, newline); pending = pending.slice(newline + 1);
        if (signal.aborted) return;
        const event: TerminalEvent = JSON.parse(line);
        if (event.type === "ready" && !ready && typeof event.id === "string") ready = true;
        else if (event.type === "output" && ready && !exited && typeof event.data === "string") { /* byte payload */ }
        else if (event.type === "exit" && ready && !exited) exited = true;
        else throw new Error("Invalid terminal output.");
        await deliver(event);
      }
      if (pending.length > 32768) throw new Error("Terminal output frame is too large.");
    }
    if (!signal.aborted && (!exited || pending.length)) throw new Error("Terminal disconnected. Open a new session to continue.");
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
}

export async function openTerminal(root: FolderRef, size: { columns: number; rows: number }, signal: AbortSignal, deliver: (event: TerminalEvent) => Promise<void>) {
  const response = await fetch("/api/terminal/open", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ...root, ...size }), signal });
  await readTerminalStream(response, signal, deliver);
}

// Input cannot race other input or resizes. A failed write stops the queue;
// replaying it could execute a command twice.
export function terminalControls(root: FolderRef, id: string, signal: AbortSignal, failed: (error: unknown) => void) {
  let queue = Promise.resolve(), stopped = false;
  const send = (action: string, fields: object) => {
    queue = queue.then(async () => {
      if (!stopped && !signal.aborted) await folderRequest(`/api/terminal/${action}`, { ...root, id, ...fields }, signal);
    }).catch(error => { if (!stopped && !signal.aborted) { stopped = true; failed(error); } });
  };
  return {
    input(data: string) {
      // Keep JSON escaping and multibyte UTF-8 under the management body cap.
      // Iterate code points so chunk boundaries never split a surrogate pair.
      let chunk = "";
      for (const point of data) { chunk += point; if (chunk.length >= 500) { send("input", { data: chunk }); chunk = ""; } }
      if (chunk) send("input", { data: chunk });
    },
    resize(columns: number, rows: number) { send("resize", { columns, rows }); },
  };
}
