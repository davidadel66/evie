import type { Commands, Evie } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"
import { endPick, startPick, type Look } from "./project-look.ts"
import type { Sessions } from "./sessions.ts"

// What the workspace view asks of evie's server for the owner, apart from
// its screen: picking a project's icon and colour, and making a workspace.
// Each gives back why the server refused, in its own words.

const clientOf = (connection: Connection): Evie | undefined => {
  const now = connection.state()
  return now.status === "connected" ? now.evie : undefined
}
const NOT_CONNECTED = "Not connected to evie's server."
const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))

// Sets a project's icon and colour, drawing them at once on every tile on
// the page, and lists the projects again. Until that list is in, a list
// of projects does not draw over the pick; then the project is drawn as it
// says, as picked, or as it was before a refused pick, unless a later pick
// has been made meanwhile. Gives back why not, or nothing.
export async function pickIcon(connection: Connection, sessions: Sessions, project: string, look: Look): Promise<string | undefined> {
  const evie = clientOf(connection)
  if (evie === undefined) return NOT_CONNECTED
  const pick = startPick(project, look)
  let refused: string | undefined
  try {
    await evie.send({ type: "set_project_look", project, icon: look.icon, colour: look.colour })
  } catch (cause) {
    refused = wordsOf(cause)
  }
  await sessions.refreshProjects()
  endPick(project, pick, sessions.state().projects.find((each) => each.id === project))
  return refused
}

// Makes a project with `new_project`, then lists the projects again, so
// that the new one is in the strip, and gives back what was made once the
// list has it, or why not.
export async function makeWorkspace(
  connection: Connection,
  sessions: Sessions,
  input: Commands["new_project"]["input"],
): Promise<Commands["new_project"]["output"] | { problem: string }> {
  const evie = clientOf(connection)
  if (evie === undefined) return { problem: NOT_CONNECTED }
  try {
    const made = await evie.send({ type: "new_project", ...input })
    await sessions.refreshProjects()
    return made
  } catch (cause) {
    return { problem: wordsOf(cause) }
  }
}
