import { useEffect, useSyncExternalStore } from "react"
import type { ProjectColour } from "../../src/core/types.ts"
import { cn } from "./lib/utils.ts"
import { DEFAULT_ICON, FOLDER, ICON_SET, keptLook, onLooks, type Look } from "./project-look.ts"

// The classes of each project colour: the icon in the colour, its tile, an
// 18% tint of it, and the line on the edge of the selected workspace's tile.
// Written out whole, so that Tailwind finds each.
export const COLOUR: Record<ProjectColour, string> = {
  moss: "text-project-moss",
  mustard: "text-project-mustard",
  maroon: "text-project-maroon",
  rust: "text-project-rust",
  slate: "text-project-slate",
  plum: "text-project-plum",
}
const TILE: Record<ProjectColour, string> = {
  moss: "bg-project-moss/18",
  mustard: "bg-project-mustard/18",
  maroon: "bg-project-maroon/18",
  rust: "bg-project-rust/18",
  slate: "bg-project-slate/18",
  plum: "bg-project-plum/18",
}
const EDGE: Record<ProjectColour, string> = {
  moss: "border-project-moss",
  mustard: "border-project-mustard",
  maroon: "border-project-maroon",
  rust: "border-project-rust",
  slate: "border-project-slate",
  plum: "border-project-plum",
}
// A swatch of each colour, as the icon picker offers them.
export const SWATCH: Record<ProjectColour, string> = {
  moss: "bg-project-moss",
  mustard: "bg-project-mustard",
  maroon: "bg-project-maroon",
  rust: "bg-project-rust",
  slate: "bg-project-slate",
  plum: "bg-project-plum",
}

// A project's look, redrawn when the owner picks another, on any tile.
export function useLook(project: string): Look {
  return useSyncExternalStore(onLooks, () => keptLook(project))
}

// A project icon by its name, from the set the owner picks from, in the
// colour of the text around it; one not in the set is drawn as a folder.
// Any but the folder waits for the set's drawings (see ICON_SET), drawn
// empty, at its size, until they are here; drawn while they are not, as
// after they could not be fetched, it asks for them again.
export function IconGlyph({ icon, className }: { icon: string; className?: string }) {
  const set = useSyncExternalStore(ICON_SET.subscribe, ICON_SET.loaded)
  const own = icon !== DEFAULT_ICON
  useEffect(() => {
    if (own && set === undefined) ICON_SET.preload()
  })
  const drawn = !own ? FOLDER : set === undefined ? undefined : (set.PROJECT_ICONS.find((each) => each.name === icon) ?? FOLDER)
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" fill="currentColor" aria-hidden="true" {...(className !== undefined && { className })}>
      {drawn?.paths.map((path, at) => (
        <path key={at} d={path.d} {...(path.opacity !== undefined && { opacity: path.opacity })} />
      ))}
    </svg>
  )
}

// The sizes of a project's tile: beside its name in a session row, in the
// session details, in the workspace header, and in the workspace strip.
const SIZES = {
  row: "size-tile-row [&>svg]:size-tile-row-icon",
  detail: "size-icon-sm [&>svg]:size-icon-xxs",
  header: "size-tile-header [&>svg]:size-icon-sm",
  strip: "size-tile-strip [&>svg]:size-icon-md",
}

// A project's icon on its tile: the icon in the project's colour on an 18%
// tint of it. The tile's corners are a quarter of its size, so that it
// never becomes round. In the strip, the selected workspace's has a line
// in its colour on its own edge. `look` draws it as it would be, as the picker shows a choice.
export function ProjectTile({ project, size, selected = false, look }: { project: string; size: keyof typeof SIZES; selected?: boolean; look?: Look }) {
  const kept = useLook(project)
  const { icon, colour } = look ?? kept
  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex shrink-0 items-center justify-center rounded-tile",
        SIZES[size],
        TILE[colour],
        COLOUR[colour],
        selected && `border-(length:--indicator-width) ${EDGE[colour]}`,
      )}
    >
      <IconGlyph icon={icon} />
    </span>
  )
}

// A project's icon as the small tile beside its name in a session row.
export function ProjectIcon({ project }: { project: string }) {
  return <ProjectTile project={project} size="row" />
}

// A project's icon alone, in its colour, as a menu item leads with it: the
// menu sizes it.
export function ProjectGlyph({ project }: { project: string }) {
  const { icon, colour } = useLook(project)
  return <IconGlyph icon={icon} className={COLOUR[colour]} />
}

// The tile of a session with no project, as the Session view drawing has
// it: as big as a project's, empty, with a dashed outline.
export function NoProjectTile() {
  return <span aria-hidden="true" className="size-tile-row shrink-0 rounded-tile border border-dashed border-text-muted" />
}

// A project's small tile and its name, or for none the empty tile and "No
// project", as a session row's first line shows them, as words of the
// row: the whole row is the link that opens the session.
export function ProjectName({ project, name }: { project: string | null; name: string | undefined }) {
  return (
    <span className="flex min-w-0 flex-1 items-center gap-2">
      {project === null ? <NoProjectTile /> : <ProjectIcon project={project} />}
      <span className="min-w-0 truncate">{project === null ? "No project" : name}</span>
    </span>
  )
}
