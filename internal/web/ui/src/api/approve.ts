// The out-of-band half of the protocol: an approval answer travels on its own
// request while the chat stream stays open, blocked in the Go approver.

import { ApiError } from "./stream";

/** answerApproval resolves one pending approval. Returns true when the server
 *  accepted it (204), false on 404 — the id already expired or was answered,
 *  which is a state the UI corrects rather than an error it reports. */
export async function answerApproval(
  id: string,
  approve: boolean,
): Promise<boolean> {
  const res = await fetch("/api/approve", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id, approve }),
  });

  if (res.status === 404) return false;
  if (!res.ok) throw new ApiError(res.status, `approval failed (${res.status})`);
  return true;
}
