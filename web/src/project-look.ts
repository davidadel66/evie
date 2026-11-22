import type { ListedProject, ProjectColour } from "../../src/core/types.ts"
import { later } from "./later.ts"
import { projectColour } from "./session-list.ts"
import type { Sessions } from "./sessions.ts"

// How a project is drawn: its icon, by its Phosphor name, and its colour,
// one of the design system's six. Every tile on the page takes them from
// here, so that a pick shows on all of them at once.
export type Look = { icon: string; colour: ProjectColour }

// A project icon's drawing: its Phosphor name, the words Phosphor tags it
// with, to search them by, and its paths, drawn on a 256 by 256 grid in the
// colour of the text around it, the lighter half of the duotone at the
// opacity given.
export type ProjectIconData = { name: string; words: readonly string[]; paths: readonly { d: string; opacity?: number }[] }

// The icon a project is drawn with until the owner picks one, and its
// drawing, copied like the rest (see project-icons.ts) from the
// @phosphor-icons/core package, 2.1.1 (MIT licence, notice in
// LICENSES/phosphor-MIT.txt).
export const DEFAULT_ICON = "folder"
export const FOLDER: ProjectIconData = { name: "folder", words: ["folder","directory","directories","files","folders"], paths: [{ d: "M128,80H32V56a8,8,0,0,1,8-8H92.69a8,8,0,0,1,5.65,2.34Z", opacity: 0.2 }, { d: "M216,72H131.31L104,44.69A15.86,15.86,0,0,0,92.69,40H40A16,16,0,0,0,24,56V200.62A15.4,15.4,0,0,0,39.38,216H216.89A15.13,15.13,0,0,0,232,200.89V88A16,16,0,0,0,216,72ZM92.69,56l16,16H40V56ZM216,200H40V88H216Z" }] }

// The drawings of every other icon, 44 KB, in a file of their own that the
// page fetches only once a project has an icon of its own, or the icon
// picker opens. Until they are here, such a project's tile is drawn without
// its icon. A browser that saw a project with an icon of its own fetches
// them at once on its next visit, beside the connection, so that they are
// here by the time the list of sessions is drawn.
export const ICON_SET = later(() => import("./project-icons.ts"))
const OWN_ICONS = "evie.own-icons"

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
  if (anyOwnIcon(projects)) {
    ICON_SET.preload()
    rememberOwnIcons(true)
  }
  if (changed) tell()
}

// Whether any of the projects has an icon of its own, not the folder, and
// so needs the icon set's drawings.
export const anyOwnIcon = (projects: readonly Pick<ListedProject, "id" | "icon" | "colour">[]) => projects.some((project) => lookOf(project).icon !== DEFAULT_ICON)

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
// function returned is called; and fetches the icons' drawings at once
// when this browser last saw a project with an icon of its own.
export function followLooks(sessions: Sessions): () => void {
  if (ownIconsSeen()) ICON_SET.preload()
  keepLooks(sessions.state().projects)
  return sessions.subscribe(() => {
    const list = sessions.state()
    keepLooks(list.projects)
    // Only a list of projects that has been read says that none has one.
    if (list.projectsListed && !anyOwnIcon(list.projects)) rememberOwnIcons(false)
  })
}

// Whether this browser last saw a project with an icon of its own, kept
// where it can be, and not otherwise, as in a private window.
function ownIconsSeen(): boolean {
  try {
    return localStorage.getItem(OWN_ICONS) === "yes"
  } catch {
    return false
  }
}
function rememberOwnIcons(seen: boolean) {
  try {
    if (seen) localStorage.setItem(OWN_ICONS, "yes")
    else localStorage.removeItem(OWN_ICONS)
  } catch {
    // Not kept: the drawings are fetched once the list is read, as on a first visit.
  }
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
