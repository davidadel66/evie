import { FolderDuotone } from "./icons.tsx"
import { projectColour, type ProjectColour } from "./session-list.ts"

// The classes of each project colour: the icon in the colour, and its
// tile, an 18% tint of it. Written out whole, so that Tailwind finds each.
const COLOUR: Record<ProjectColour, string> = {
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

// A project's icon as the small tile beside its name in a session row.
// Until the owner can pick a project's icon and colour, every project has a
// folder, in a colour chosen from its id. The tile's corners are a quarter
// of its size, so that it never becomes round.
export function ProjectIcon({ project }: { project: string }) {
  const colour = projectColour(project)
  return (
    <span aria-hidden="true" className={`flex size-tile-row shrink-0 items-center justify-center rounded-tile ${TILE[colour]} ${COLOUR[colour]}`}>
      <FolderDuotone className="size-tile-row-icon" />
    </span>
  )
}

// A project's icon alone, in its colour, as a menu item leads with it: the
// menu sizes it.
export function ProjectGlyph({ project }: { project: string }) {
  return <FolderDuotone className={COLOUR[projectColour(project)]} />
}
