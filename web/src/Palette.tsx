import { useEffect, useRef, useState, useSyncExternalStore, type KeyboardEvent, type ReactNode } from "react"
import type { SessionSummary } from "../../src/core/types.ts"
import { toggleCard, useCardShown } from "./card-shown.ts"
import { Dialog, DialogContent, DialogTitle } from "./components/ui/dialog.tsx"
import type { Connection } from "./connection.ts"
import { holdConversation, type Conversation, type ConversationState } from "./conversation.ts"
import { Article, CaretLeft, CaretRight, GearSix, MagnifyingGlass, Plus, SidebarLeft, StopFill } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { toggleList, useListShown } from "./list-shown.ts"
import { ask, newSession } from "./page-requests.ts"
import { paletteActions, rankGroups, recentSessions, type Group, type PaletteAction, type Searched } from "./palette.ts"
import { ProjectGlyph } from "./ProjectIcon.tsx"
import { openSettings, select, useAddress } from "./selection.ts"
import { ageWords, titleOf } from "./session-list.ts"
import type { Sessions } from "./sessions.ts"
import { addressOfWorkspace, workspaceIn } from "./workspace.ts"
import { lookedAt } from "./address.ts"
import { toggleDiff, useDiffShown } from "./diff-shown.ts"
import { FolderPath } from "./FolderPath.tsx"
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
  // A project's folder, shown cut at its start, so that its end shows.
  folder?: string
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
        className="top-header-height max-w-palette translate-y-0 gap-0 overflow-hidden p-0 max-wide:inset-x-4"
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
  const hash = useAddress()
  const listShown = useListShown()
  const cardShown = useCardShown()
  const diffShown = useDiffShown()
  // What the owner is looking at: in the session view, its session, one of
  // its subagents' conversations or a draft; in a workspace, its side
  // chat's session or a new one. A session is as the list has it.
  const workspace = workspaceIn(hash) !== undefined
  const look = lookedAt(hash)
  const session = look !== undefined && "session" in look ? list.sessions.find((each) => each.session === look.session) : undefined
  const own = session !== undefined && look !== undefined && "session" in look && !look.subagent
  const conversation = useHeld(connection, own ? session.session : undefined)

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

  // Each action's row, of those that apply now.
  const shownNow = (panel: { wide: boolean; column: boolean; sheet: boolean }) => (panel.wide ? panel.column : panel.sheet)
  const ROWS: Record<PaletteAction, () => Row | undefined> = {
    new: () => ({ id: "new", title: "New session", keywords: ["chat", "draft", "start"], icon: <Plus />, shortcut: "new session", run: newSession }),
    "new in": () => ({
      id: "new in",
      title: "New session in a project…",
      keywords: ["chat", "draft", "start", "project"],
      icon: <Plus />,
      opens: { name: "New session in", groups: projectsTo((project) => select({ draft: project })) },
    }),
    previous: () => ({ id: "previous", title: "Previous session", keywords: ["above", "up"], shortcut: "previous session", run: () => void ask("previous session") }),
    next: () => ({ id: "next", title: "Next session", keywords: ["below", "down"], shortcut: "next session", run: () => void ask("next session") }),
    settings: () => ({ id: "settings", title: "Open settings", keywords: ["preferences", "keys", "plugins", "instructions", "notifications"], icon: <GearSix />, takesFocus: true, run: () => openSettings() }),
    theme: () => ({
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
    }),
    list: () => ({ id: "list", title: shownNow(listShown) ? "Hide the session list" : "Show the session list", keywords: ["sidebar", "sessions"], icon: <SidebarLeft />, shortcut: "session list", run: toggleList }),
    card: () => ({ id: "card", title: cardShown.open ? "Hide session details" : "Show session details", keywords: ["panel", "project", "card", "subagents", "jobs"], icon: <Article />, shortcut: "card", run: toggleCard }),
    diff: () => ({ id: "diff", title: diffShown.open ? "Close the diff" : "Open the diff", keywords: ["changes", "diffs", "files"], shortcut: "diff", run: toggleDiff }),
    settle: () =>
      session && {
        id: "settle",
        title: session.state === "settled" ? "Bring back this session" : "Settle this session",
        keywords: ["archive", "settled"],
        run: () => void sessions.settle(session.session, session.state !== "settled"),
      },
    move: () => session && { id: "move", title: "Move to…", keywords: ["project", "session"], opens: { name: "Move to", groups: projectsTo((project) => void sessions.move(session.session, project), session.project) } },
    rename: () => ({ id: "rename", title: "Rename this session", keywords: ["name", "title"], shortcut: "rename", takesFocus: true, run: () => void ask("rename") }),
    compact: () => conversation && { id: "compact", title: "Compact now", keywords: ["context", "summary"], run: () => void conversation.conversation.run("compact", "") },
    stop: () => conversation && { id: "stop", title: "Stop", keywords: ["cancel", "interrupt"], icon: <StopFill />, shortcut: "stop", run: () => conversation.conversation.stop() },
    model: () => ({ id: "model", title: "Pick a model", keywords: ["model"], shortcut: "model", takesFocus: true, run: () => void ask("pick model") }),
  }
  const actions = paletteActions({
    workspace,
    looking: look === undefined ? "none" : "draft" in look ? "draft" : look.subagent ? "subagent" : "session",
    state: session?.state,
    turnRunning: conversation?.state.turnRunning === true,
    stopping: conversation?.state.stopping === true,
    compacting: conversation?.state.running.includes("compact") === true,
  }).flatMap((action) => ROWS[action]() ?? [])

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
              folder: project.folder,
              icon: <ProjectGlyph project={project.id} />,
              run: () => (location.hash = addressOfWorkspace({ workspace: project.id })),
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
      <div className="flex shrink-0 items-center gap-2 border-b border-line px-3 py-2">
        {view === undefined ? (
          <MagnifyingGlass className="size-icon-sm shrink-0 text-text-muted" />
        ) : (
          <button type="button" onClick={back} aria-label="Back" className="-ml-1 flex h-control-sm shrink-0 items-center gap-1 rounded-md px-1 text-caption text-text-muted hover:text-text">
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
          className="h-control min-w-0 flex-1 bg-transparent text-field text-text outline-none placeholder:text-text-muted"
        />
      </div>
      <div ref={listBox} id="palette-rows" role="listbox" aria-label="Results" className="flex max-h-palette-list min-h-0 flex-col overflow-y-auto p-2">
        {rows.length === 0 && <p className="flex min-h-control-sm items-center px-2 text-caption text-text-muted">{view === undefined ? "No matching command, session or project." : "Nothing matches."}</p>}
        {groups.map((group) => (
          <div key={group.name} role="group" aria-label={group.name} className="flex flex-col">
            <p className="px-2 pt-2 pb-1 text-caption text-text-muted">{group.name}</p>
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
                  className={cn("flex h-control-sm shrink-0 cursor-default items-center gap-2 rounded-sm px-2 [&>svg]:size-icon-sm [&>svg]:shrink-0", place === highlight && "bg-surface-raised")}
                >
                  {row.icon ?? <span aria-hidden="true" className="size-icon-sm shrink-0" />}
                  <span className="min-w-0 truncate text-control text-text">{row.title}</span>
                  {row.detail !== undefined && row.detail !== "" && <span className="min-w-0 flex-1 truncate text-caption text-text-muted">{row.detail}</span>}
                  {row.folder !== undefined && <FolderPath folder={row.folder} className="flex-1 text-text-muted" />}
                  <span className="ml-auto flex shrink-0 items-center gap-2 text-caption text-text-muted">
                    {row.shortcut !== undefined && <kbd className="hidden font-sans pointer-fine:inline">{label(row.shortcut, mac)}</kbd>}
                    {row.opens !== undefined && <CaretRight className="size-icon-sm" />}
                  </span>
                </div>
              )
            })}
          </div>
        ))}
      </div>
      <p className="hidden shrink-0 gap-4 border-t border-line px-3 py-2 text-caption text-text-muted pointer-fine:flex">
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
