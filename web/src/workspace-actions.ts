import type { Commands, Evie } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"
import type { Look } from "./project-look.ts"
import type { Sessions } from "./sessions.ts"

// What the workspace view asks of evie's server for the owner, apart from
// its screen: picking a project's icon and colour, making a workspace, and
// the home folder a new one's folder is suggested in. Each gives back why
// the server refused, in its own words.

const clientOf = (connection: Connection): Evie | undefined => {
  const now = connection.state()
  return now.status === "connected" ? now.evie : undefined
}
const NOT_CONNECTED = "Not connected to evie's server."
const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))

// Sets a project's icon and colour, then lists the projects again, so that
// every tile on the page shows them. Gives back why not, or nothing.
export async function pickIcon(connection: Connection, sessions: Sessions, project: string, look: Look): Promise<string | undefined> {
  const evie = clientOf(connection)
  if (evie === undefined) return NOT_CONNECTED
  try {
    await evie.send({ type: "set_project_icon", project, icon: look.icon, colour: look.colour })
    sessions.refreshProjects()
    return undefined
  } catch (cause) {
    return wordsOf(cause)
  }
}

// Makes a project with `new_project`, then lists the projects again, so
// that the new one is in the strip. Gives back what was made, or why not.
export async function makeWorkspace(
  connection: Connection,
  sessions: Sessions,
  input: Commands["new_project"]["input"],
): Promise<Commands["new_project"]["output"] | { problem: string }> {
  const evie = clientOf(connection)
  if (evie === undefined) return { problem: NOT_CONNECTED }
  try {
    const made = await evie.send({ type: "new_project", ...input })
    sessions.refreshProjects()
    return made
  } catch (cause) {
    return { problem: wordsOf(cause) }
  }
}

// The home folder of the computer evie runs on, or undefined when it
// cannot be asked, as while the line is down.
export async function readHome(connection: Connection): Promise<string | undefined> {
  const evie = clientOf(connection)
  if (evie === undefined) return undefined
  return evie.send({ type: "read_home" }).then(
    (home) => home.folder,
    () => undefined,
  )
}
