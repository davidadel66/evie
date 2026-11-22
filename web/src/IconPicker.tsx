import { useState, type KeyboardEvent, type ReactNode } from "react"
import type { ProjectColour } from "../../src/core/types.ts"
import { Input } from "./components/ui/input.tsx"
import { Popover, PopoverContent, PopoverTrigger } from "./components/ui/popover.tsx"
import type { Connection } from "./connection.ts"
import { cn } from "./lib/utils.ts"
import { searchIcons } from "./project-icons.ts"
import { keepLooks, type Look } from "./project-look.ts"
import { COLOUR, IconGlyph, SWATCH, useLook } from "./ProjectIcon.tsx"
import { PROJECT_COLOURS } from "./session-list.ts"
import type { Sessions } from "./sessions.ts"
import { pickIcon } from "./workspace-actions.ts"

// The picker of a project's icon and colour, opened from the workspace
// header's tile and name: the six colours, then a search over the icons
// that suit a project. A colour applies at once and leaves the picker open,
// to pick the icon in it; an icon applies and closes it. Every tile on the
// page shows the pick straight away, and goes back if the server refuses it.
export function IconPicker({ connection, sessions, project, children }: { connection: Connection; sessions: Sessions; project: { id: string; name: string }; children: ReactNode }) {
  const look = useLook(project.id)
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  const [problem, setProblem] = useState<string>()
  const shown = searchIcons(query)

  async function pick(next: Look, close: boolean) {
    const before = look
    setProblem(undefined)
    keepLooks([{ id: project.id, ...next }])
    if (close) setOpen(false)
    const refused = await pickIcon(connection, sessions, project.id, next)
    if (refused === undefined) return
    keepLooks([{ id: project.id, ...before }])
    setProblem(refused)
    setOpen(true)
  }
  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key !== "Enter" || shown[0] === undefined) return
    event.preventDefault()
    void pick({ icon: shown[0].name, colour: look.colour }, true)
  }

  return (
    <Popover
      open={open}
      onOpenChange={(now) => {
        setOpen(now)
        if (now) setQuery("")
      }}
    >
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent aria-label={`Icon and colour of ${project.name}`} className="w-icon-picker gap-2">
        <div role="radiogroup" aria-label="Colour" className="flex gap-1">
          {PROJECT_COLOURS.map((colour: ProjectColour) => (
            <button
              key={colour}
              type="button"
              role="radio"
              aria-checked={colour === look.colour}
              aria-label={named(colour)}
              title={named(colour)}
              onClick={() => colour !== look.colour && void pick({ icon: look.icon, colour }, false)}
              className="flex size-control items-center justify-center rounded-md aria-checked:bg-surface-raised"
            >
              <span aria-hidden="true" className={cn("size-icon-md rounded-full", SWATCH[colour])} />
            </button>
          ))}
        </div>
        <Input type="search" autoFocus aria-label="Search icons" placeholder="Search icons" value={query} onChange={(event) => setQuery(event.target.value)} onKeyDown={onKeyDown} />
        {shown.length === 0 ? (
          <p className="px-3 py-2 text-caption text-text-muted">No matching icon.</p>
        ) : (
          <ul aria-label="Icons" className="grid max-h-icon-grid grid-cols-6 gap-1 overflow-y-auto">
            {shown.map((icon) => (
              <li key={icon.name}>
                <button
                  type="button"
                  aria-label={named(icon.name)}
                  aria-pressed={icon.name === look.icon}
                  title={named(icon.name)}
                  onClick={() => void pick({ icon: icon.name, colour: look.colour }, true)}
                  className={cn("flex size-control items-center justify-center rounded-md hover:bg-surface-raised aria-pressed:bg-surface-raised", COLOUR[look.colour])}
                >
                  <IconGlyph icon={icon.name} className="size-icon-md" />
                </button>
              </li>
            ))}
          </ul>
        )}
        {problem !== undefined && (
          <p role="status" className="px-3 text-caption break-words text-text-muted">
            {problem}
          </p>
        )}
      </PopoverContent>
    </Popover>
  )
}

// A colour's or an icon's name as a label says it: "Cooking pot".
const named = (name: string) => `${name.charAt(0).toUpperCase()}${name.slice(1).replaceAll("-", " ")}`
