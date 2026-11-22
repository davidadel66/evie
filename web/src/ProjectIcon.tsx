import { FolderDuotone } from "./icons.tsx"
import { projectColour, type ProjectColour } from "./session-list.ts"

// The classes of each project colour's tile: the icon in the colour, on an
// 18% tint of it. Written out whole, so that Tailwind finds each class.
const TILE: Record<ProjectColour, string> = {
  moss: "bg-project-moss/18 text-project-moss",
  mustard: "bg-project-mustard/18 text-project-mustard",
  maroon: "bg-project-maroon/18 text-project-maroon",
  rust: "bg-project-rust/18 text-project-rust",
  slate: "bg-project-slate/18 text-project-slate",
  plum: "bg-project-plum/18 text-project-plum",
}

// A project's icon as the small tile beside its name in a session row or a
// menu. Until the owner can pick a project's icon and colour, every project
// has a folder, in a colour chosen from its id. At 18px the tile's corners
// are the small radius: the default would make it round.
export function ProjectIcon({ project }: { project: string }) {
  return (
    <span aria-hidden="true" className={`flex size-tile-row shrink-0 items-center justify-center rounded-sm ${TILE[projectColour(project)]}`}>
      <FolderDuotone className="size-tile-row-icon" />
    </span>
  )
}
