import type { ListedProject, ProjectColour } from "../../src/core/types.ts"
import { DEFAULT_ICON } from "./project-icons.ts"
import { projectColour } from "./session-list.ts"
import type { Sessions } from "./sessions.ts"

// How a project is drawn: its icon, by its Phosphor name, and its colour,
// one of the design system's six. Every tile on the page takes them from
// here, so that a pick shows on all of them at once.
export type Look = { icon: string; colour: ProjectColour }

// The icon and colour the owner picked, and until then a folder in the
// colour the project's id gives it, as the session list has always drawn it.
export function lookOf(project: Pick<ListedProject, "id" | "icon" | "colour">): Look {
  return { icon: project.icon ?? DEFAULT_ICON, colour: project.colour ?? projectColour(project.id) }
}

// Each listed project's look, by its id, as the latest list of projects
// gives it, for a tile that knows only the project's id. A project not
// listed yet is drawn as if nothing were picked.
const looks = new Map<string, Look>()
const listeners = new Set<() => void>()
// The latest pick of each project whose pick has not ended, by its number.
// While one is out, a list of projects does not draw over it: a list can be
// older than the pick, or than the server's answer to it.
const picking = new Map<string, number>()
let picks = 0

function draw(project: string, look: Look): boolean {
  const kept = looks.get(project)
  if (kept?.icon === look.icon && kept.colour === look.colour) return false
  looks.set(project, look)
  return true
}
const tell = () => {
  for (const listener of [...listeners]) listener()
}

// Keeps the looks of a list of projects, telling the tiles of those that
// changed, but for a project being picked.
export function keepLooks(projects: readonly Pick<ListedProject, "id" | "icon" | "colour">[]) {
  let changed = false
  for (const project of projects) if (!picking.has(project.id) && draw(project.id, lookOf(project))) changed = true
  if (changed) tell()
}

// Draws a project with a look the owner has just picked, before the server
// has it, and gives back the pick's number, to end it with.
export function startPick(project: string, look: Look): number {
  const pick = ++picks
  picking.set(project, pick)
  if (draw(project, look)) tell()
  return pick
}

// Ends a pick. When no later pick of the project has been made, it is
// drawn as `listed`, a list of projects asked for once the server had
// answered the pick, says: as picked, or as it was before a refused pick.
export function endPick(project: string, pick: number, listed: Pick<ListedProject, "id" | "icon" | "colour"> | undefined) {
  if (picking.get(project) !== pick) return
  picking.delete(project)
  if (listed !== undefined && draw(project, lookOf(listed))) tell()
}

// Keeps the looks of every list of projects the page is given, until the
// function returned is called.
export function followLooks(sessions: Sessions): () => void {
  keepLooks(sessions.state().projects)
  return sessions.subscribe(() => keepLooks(sessions.state().projects))
}

// A project's look as last kept: the same object until it changes.
export function keptLook(project: string): Look {
  let look = looks.get(project)
  if (look === undefined) looks.set(project, (look = lookOf({ id: project })))
  return look
}

export function onLooks(listener: () => void): () => void {
  listeners.add(listener)
  return () => void listeners.delete(listener)
}
