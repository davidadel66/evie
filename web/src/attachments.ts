import type { Attachment } from "../../src/core/types.ts"
import { PATHS } from "../../src/server/wire.ts"

// The files attached to the owner's messages, as the web app sends and
// shows them, apart from its screens.

// An attachment as the page shows it: with the address its image is
// fetched from, when it is an image a browser draws, and never runs.
export type ShownAttachment = Attachment & { address?: string }

const DRAWN = ["image/png", "image/jpeg", "image/gif", "image/webp"]

export function shownAttachment(session: string, attachment: Attachment): ShownAttachment {
  if (!DRAWN.includes(attachment.type)) return attachment
  return { ...attachment, address: `${PATHS.attachments}?${new URLSearchParams({ session, path: attachment.path })}` }
}

// How long a file may take to reach the server and be kept, in
// milliseconds: long enough for 10 MB on a slow line, and no longer, so
// that a stalled upload does not hold the messages sent after it.
const UPLOAD_WITHIN = 2 * 60_000

// Sends a file attached to a message to the server, signed in as the
// WebSocket is: with the browser's cookie, or a device's token when one is
// given. Gives back the attachment, as the message records it, or fails
// with why, in words for the owner: the server's own, when it refused it.
// It is given up after `within` milliseconds.
export async function uploadFile(url: string, token: string | undefined, session: string, file: File, within = UPLOAD_WITHIN): Promise<Attachment> {
  const timeout = AbortSignal.timeout(within)
  let answer: Response
  try {
    answer = await fetch(new URL(`${PATHS.attachments}?${new URLSearchParams({ session, name: file.name })}`, url), {
      method: "POST",
      body: file,
      headers: token === undefined ? {} : { Authorization: `Bearer ${token}` },
      signal: timeout,
    })
  } catch {
    if (timeout.aborted) throw new Error(`${file.name} could not be sent: evie's server did not take it in time. Send it again.`)
    throw new Error(`${file.name} could not be sent: evie's server could not be reached. Send it again once it is connected.`)
  }
  if (!answer.ok) throw new Error((await answer.text().catch(() => "")) || `${file.name} could not be sent: evie's server answered ${answer.status}.`)
  return (await answer.json()) as Attachment
}
