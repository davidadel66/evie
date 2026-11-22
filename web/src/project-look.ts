import type { ListedProject, ProjectColour } from "../../src/core/types.ts"
import { DEFAULT_ICON } from "./project-icons.ts"
import { projectColour } from "./session-list.ts"

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

// Keeps the looks of a list of projects, telling the tiles of those that changed.
export function keepLooks(projects: readonly Pick<ListedProject, "id" | "icon" | "colour">[]) {
  let changed = false
  for (const project of projects) {
    const look = lookOf(project)
    const kept = looks.get(project.id)
    if (kept?.icon === look.icon && kept.colour === look.colour) continue
    looks.set(project.id, look)
    changed = true
  }
  if (changed) for (const listener of [...listeners]) listener()
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
