import { useEffect, useRef, useState, useSyncExternalStore, type KeyboardEvent, type ReactNode } from "react"
import type { SessionSummary } from "../../src/core/types.ts"
import { toggleCard, useCardShown } from "./card-shown.ts"
import { Dialog, DialogContent, DialogTitle } from "./components/ui/dialog.tsx"
import type { Connection } from "./connection.ts"
import { holdConversation, type Conversation, type ConversationState } from "./conversation.ts"
import { CaretLeft, CaretRight, GearSix, MagnifyingGlass, Plus, SidebarLeft, SidebarRight, StopFill } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { toggleList, useListShown } from "./list-shown.ts"
import { ask } from "./page-requests.ts"
import { rankGroups, recentSessions, type Group, type Searched } from "./palette.ts"
import { ProjectGlyph } from "./ProjectIcon.tsx"
import { select, useSelected } from "./selection.ts"
import { ageWords, titleOf } from "./session-list.ts"
import type { Sessions } from "./sessions.ts"
import { addressOfWorkspace, workspaceIn } from "./workspace.ts"
import { Swatch } from "./Settings.tsx"
import { label, onMac, type Command } from "./shortcuts.ts"
import { applyTheme, rememberedTheme, THEMES } from "./theme.ts"

// The command palette, mod+K, as T3 Code's: a search over what can be done
// and where to go. It opens on the actions and the recent sessions; what
// is typed searches the actions, every session and every project, best
// first. A row shows its shortcut, and some open a list of their own, such
// as the themes, which Backspace in an empty search leaves. Up and Down
// move, Enter runs the row raised, and Escape closes it.

// A row: what it is called and known by, what leads it and follows it, and
// what it does, or the list it opens. One that moves the focus elsewhere,
// as to the name being renamed, is run once the palette has closed, so
// that the palette does not give the focus back after it.
type Row = Searched & {
  id: string
  icon?: ReactNode
  detail?: string
  shortcut?: Command
  run?: () => void
  opens?: { name: string; groups: Group<Row>[] }
  takesFocus?: boolean
}

export function Palette({ connection, sessions, open, close }: { connection: Connection; sessions: Sessions; open: boolean; close(): void }) {
  // The row chosen, run once the palette has closed, and where the focus
  // was as it opened, which it goes back to, but for a row that moves it.
  const chosen = useRef<Row>(undefined)
  const opener = useRef<Element | null>(null)
  return (
    <Dialog open={open} onOpenChange={(now) => !now && close()}>
      <DialogContent
        data-slot="palette"
        aria-label="Command palette"
        onOpenAutoFocus={() => (opener.current = document.activeElement)}
        onCloseAutoFocus={(event) => {
          event.preventDefault()
          const row = chosen.current
          chosen.current = undefined
          if (!row?.takesFocus && opener.current instanceof HTMLElement && opener.current.isConnected) opener.current.focus()
          row?.run?.()
        }}
        className="top-header-height max-w-palette translate-y-0 gap-0 overflow-hidden p-0 max-list:inset-x-4"
      >
        <DialogTitle className="sr-only">Command palette</DialogTitle>
        {open && (
          <Searching
            connection={connection}
            sessions={sessions}
            run={(row) => {
              chosen.current = row
              close()
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function Searching({ connection, sessions, run }: { connection: Connection; sessions: Sessions; run(row: Row): void }) {
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  const selected = useSelected()
  // In a workspace, its view has neither the session list nor the card,
  // and a new session starts in its project.
  const workspace = workspaceIn(location.hash)
  const listShown = useListShown()
  const cardShown = useCardShown()
  // The session selected, as the list has it, and whether its own
  // conversation is in the middle, with its message box, rather than a
  // subagent's.
  const session = selected !== undefined && "session" in selected ? list.sessions.find((each) => each.session === selected.session) : undefined
  const ownConversation = session !== undefined && selected !== undefined && "session" in selected && selected.subagent === undefined
  const conversation = useHeld(connection, ownConversation ? session.session : undefined)

  const [query, setQuery] = useState("")
  // The lists opened from rows, the latest last, each with the search it was opened from.
  const [opened, setOpened] = useState<{ row: Row; query: string }[]>([])
  const [highlight, setHighlight] = useState(0)
  const input = useRef<HTMLInputElement>(null)
  const listBox = useRef<HTMLDivElement>(null)

  const projectName = (id: string | null) => (id === null ? undefined : list.projects.find((project) => project.id === id)?.name)
  const sessionRow = (each: SessionSummary): Row => ({
    id: `session:${each.session}`,
    title: titleOf(each),
    keywords: [projectName(each.project) ?? ""],
    detail: [projectName(each.project), stateWords(each)].filter((part) => part !== undefined).join(" · "),
    ...(each.project !== null && { icon: <ProjectGlyph project={each.project} /> }),
    run: () => select({ session: each.session }),
  })
  const projectsTo = (go: (project: string | null) => void, except?: string | null): Group<Row>[] => [
    {
      name: "Projects",
      items: [
        ...list.projects
          .filter((project) => project.id !== except)
          .map((project): Row => ({ id: `project:${project.id}`, title: project.name, icon: <ProjectGlyph project={project.id} />, run: () => go(project.id) })),
        ...(except !== null ? [{ id: "project:none", title: "No project", run: () => go(null) }] : []),
      ],
    },
  ]

  const actions: Row[] = [
    {
      id: "new",
      title: "New session",
      keywords: ["chat", "draft", "start"],
      icon: <Plus />,
      shortcut: "new session",
      run: () => void (ask("new session") || select({ draft: workspace?.workspace ?? null })),
    },
    {
      id: "new in",
      title: "New session in a project…",
      keywords: ["chat", "draft", "start", "project"],
      icon: <Plus />,
      opens: { name: "New session in", groups: projectsTo((project) => select({ draft: project })) },
    },
    { id: "settings", title: "Open settings", keywords: ["preferences", "keys", "plugins", "instructions", "notifications"], icon: <GearSix />, takesFocus: true, run: () => ask("settings") },
    {
      id: "theme",
      title: "Theme",
      keywords: ["appearance", "colours", "colors", ...THEMES.map((theme) => theme.name)],
      icon: <Swatch theme={rememberedTheme()} />,
      opens: {
        name: "Theme",
        groups: [
          {
            name: "Themes",
            items: THEMES.map((theme) => ({
              id: `theme:${theme.id}`,
              title: theme.name,
              icon: <Swatch theme={theme.id} />,
              ...(theme.id === rememberedTheme() && { detail: "In use" }),
              run: () => applyTheme(theme.id),
            })),
          },
        ],
      },
    },
    ...(workspace !== undefined
      ? []
      : [
          {
            id: "list",
            title: (listShown.wide ? listShown.column : listShown.sheet) ? "Hide the session list" : "Show the session list",
            keywords: ["sidebar", "sessions"],
            icon: <SidebarLeft />,
            shortcut: "session list" as const,
            run: toggleList,
          },
          {
            id: "card",
            title: (cardShown.wide ? cardShown.column : cardShown.sheet) ? "Hide the project card" : "Show the project card",
            keywords: ["panel", "project", "subagents", "jobs"],
            icon: <SidebarRight />,
            shortcut: "card" as const,
            run: toggleCard,
          },
        ]),
    ...(session !== undefined && (session.state === "active" || session.state === "settled")
      ? [
          {
            id: "settle",
            title: session.state === "settled" ? "Bring back this session" : "Settle this session",
            keywords: ["archive", "settled"],
            run: () => void sessions.settle(session.session, session.state !== "settled"),
          },
          { id: "move", title: "Move to…", keywords: ["project", "session"], opens: { name: "Move to", groups: projectsTo((project) => void sessions.move(session.session, project), session.project) } },
        ]
      : []),
    ...(session !== undefined ? [{ id: "rename", title: "Rename this session", keywords: ["name", "title"], shortcut: "rename" as const, takesFocus: true, run: () => ask("rename") }] : []),
    ...(conversation !== undefined && !conversation.state.running.includes("compact")
      ? [{ id: "compact", title: "Compact now", keywords: ["context", "summary"], run: () => void conversation.conversation.run("compact", "") }]
      : []),
    ...(conversation?.state.turnRunning === true && !conversation.state.stopping
      ? [{ id: "stop", title: "Stop", keywords: ["cancel", "interrupt"], icon: <StopFill />, shortcut: "stop" as const, run: () => conversation.conversation.stop() }]
      : []),
    ...(ownConversation || (selected !== undefined && "draft" in selected) || workspace !== undefined
      ? [{ id: "model", title: "Pick a model", keywords: ["model"], shortcut: "model" as const, takesFocus: true, run: () => ask("pick model") }]
      : []),
  ]

  const view = opened.at(-1)?.row.opens
  const groups = (() => {
    if (view !== undefined) return rankGroups(view.groups, query)
    if (query.trim() === "") {
      const recent = recentSessions(list.sessions, session?.session)
      return [{ name: "Actions", items: actions }, ...(recent.length > 0 ? [{ name: "Recent sessions", items: recent.map(sessionRow) }] : [])]
    }
    return rankGroups(
      [
        { name: "Actions", items: actions },
        { name: "Sessions", items: [...list.sessions].sort((a, b) => b.lastActive - a.lastActive).map(sessionRow) },
        {
          name: "Projects",
          items: list.projects.map(
            (project): Row => ({
              id: `project:${project.id}`,
              title: project.name,
              keywords: [project.folder],
              detail: project.folder,
              icon: <ProjectGlyph project={project.id} />,
              run: () => (location.hash = addressOfWorkspace({ workspace: project.id, tab: "sessions" })),
            }),
          ),
        },
      ],
      query,
    )
  })()
  const rows = groups.flatMap((group) => group.items)
  useEffect(() => setHighlight(0), [query, view])
  useEffect(() => {
    listBox.current?.querySelector(`[data-at="${highlight}"]`)?.scrollIntoView({ block: "nearest" })
  }, [highlight])

  function choose(row: Row) {
    if (row.opens !== undefined) {
      setOpened((was) => [...was, { row, query }])
      setQuery("")
      input.current?.focus()
      return
    }
    run(row)
  }
  function back() {
    const last = opened.at(-1)
    if (last === undefined) return
    setOpened((was) => was.slice(0, -1))
    setQuery(last.query)
    input.current?.focus()
  }
  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.nativeEvent.isComposing) return
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault()
      if (rows.length > 0) setHighlight((at) => (at + (event.key === "ArrowDown" ? 1 : rows.length - 1)) % rows.length)
    } else if (event.key === "Enter") {
      event.preventDefault()
      const row = rows[highlight]
      if (row !== undefined) choose(row)
    } else if (event.key === "Backspace" && query === "" && view !== undefined) {
      event.preventDefault()
      back()
    }
  }

  const mac = onMac()
  let at = -1
  return (
    <div className="flex min-h-0 flex-col">
      <div className="flex h-row-height shrink-0 items-center gap-2 border-b border-line px-4">
        {view === undefined ? (
          <MagnifyingGlass className="size-icon-sm shrink-0 text-text-muted" />
        ) : (
          <button type="button" onClick={back} aria-label="Back" className="-ml-2 flex h-control shrink-0 items-center gap-1 rounded-md px-2 text-caption text-text-muted hover:text-text">
            <CaretLeft className="size-icon-sm" />
            {view.name}
          </button>
        )}
        <input
          ref={input}
          type="text"
          role="combobox"
          aria-label={view === undefined ? "Search commands, projects and sessions" : `Search ${view.name.toLowerCase()}`}
          aria-expanded="true"
          aria-controls="palette-rows"
          aria-activedescendant={rows.length > 0 ? `palette-row-${highlight}` : undefined}
          aria-autocomplete="list"
          autoComplete="off"
          spellCheck={false}
          placeholder={view === undefined ? "Search commands, projects and sessions…" : "Search…"}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={onKeyDown}
          className="h-control min-w-0 flex-1 bg-transparent text-body text-text outline-none placeholder:text-text-muted"
        />
      </div>
      <div ref={listBox} id="palette-rows" role="listbox" aria-label="Results" className="flex max-h-palette-list min-h-0 flex-col overflow-y-auto p-2">
        {rows.length === 0 && <p className="flex min-h-control items-center px-3 text-caption text-text-muted">{view === undefined ? "No matching command, session or project." : "Nothing matches."}</p>}
        {groups.map((group) => (
          <div key={group.name} role="group" aria-label={group.name} className="flex flex-col">
            <p className="px-3 pt-2 pb-1 text-caption text-text-muted">{group.name}</p>
            {group.items.map((row) => {
              at += 1
              const place = at
              return (
                <div
                  key={`${group.name}:${row.id}`}
                  id={`palette-row-${place}`}
                  data-at={place}
                  role="option"
                  aria-selected={place === highlight}
                  // The search keeps the focus.
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => choose(row)}
                  onMouseMove={() => place !== highlight && setHighlight(place)}
                  className={cn("flex h-control shrink-0 cursor-default items-center gap-3 rounded-md px-3 [&>svg]:size-icon-sm [&>svg]:shrink-0", place === highlight && "bg-surface-raised")}
                >
                  {row.icon ?? <span aria-hidden="true" className="size-icon-sm shrink-0" />}
                  <span className="min-w-0 truncate text-body text-text">{row.title}</span>
                  {row.detail !== undefined && row.detail !== "" && <span className="min-w-0 flex-1 truncate text-caption text-text-muted">{row.detail}</span>}
                  <span className="ml-auto flex shrink-0 items-center gap-2 text-caption text-text-muted">
                    {row.shortcut !== undefined && <kbd className="font-sans">{label(row.shortcut, mac)}</kbd>}
                    {row.opens !== undefined && <CaretRight className="size-icon-sm" />}
                  </span>
                </div>
              )
            })}
          </div>
        ))}
      </div>
      <p className="flex shrink-0 gap-4 border-t border-line px-4 py-2 text-caption text-text-muted max-list:hidden">
        <span>↑↓ Move</span>
        <span>Enter {rows[highlight]?.opens !== undefined ? "Open" : "Run"}</span>
        {view !== undefined && <span>Backspace Back</span>}
        <span>Esc Close</span>
      </p>
    </div>
  )
}

// A session's state, as a row in the palette says it: in words while it
// needs the owner or works, and otherwise how long ago it was active.
function stateWords(session: SessionSummary): string {
  if (session.state === "needs_you") return "Needs you"
  if (session.state === "working") return "Working"
  return ageWords(session.lastActive, Date.now())
}

// The session's conversation and how it stands, while the palette is
// open, for Stop and Compact now: the conversation the middle column
// holds, held here too, so that it is not opened twice.
function useHeld(connection: Connection, session: string | undefined): { conversation: Conversation; state: ConversationState } | undefined {
  const [held, setHeld] = useState<Conversation>()
  useEffect(() => {
    if (session === undefined) return setHeld(undefined)
    const holding = holdConversation(connection, session)
    setHeld(holding.conversation)
    return holding.release
  }, [connection, session])
  const state = useSyncExternalStore(held?.subscribe ?? NOTHING_TO_HEAR, held?.state ?? NO_STATE)
  return held !== undefined && state !== undefined ? { conversation: held, state } : undefined
}
const NOTHING_TO_HEAR = () => () => {}
const NO_STATE = () => undefined
