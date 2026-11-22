import { useState, useSyncExternalStore } from "react"
import { Button } from "./components/ui/button.tsx"
import { DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { ConnectionLight } from "./ConnectionLight.tsx"
import type { ConnectionState } from "./connection.ts"
import { useSelected } from "./selection.ts"
import { titleOf } from "./session-list.ts"
import { SessionList } from "./SessionList.tsx"
import type { Sessions } from "./sessions.ts"
import { applyTheme, rememberedTheme, THEMES, type Theme } from "./theme.ts"
import { Words } from "./Words.tsx"

// The page once signed in: the session list, the conversation and the
// project card, in three columns, which stack at phone width. The
// conversation and the project card hold, for now, a note of what will go
// there.
export function Frame({ state, sessions }: { state: ConnectionState; sessions: Sessions }) {
  return (
    <div className="flex min-h-dvh flex-col wide:h-dvh wide:flex-row">
      <aside aria-label="Sessions" className="flex flex-col bg-bg-side wide:w-session-list wide:shrink-0">
        {/* The theme button's own padding brings its words to the page's 28px edge. */}
        <header className="flex h-header-height shrink-0 items-center gap-3 pr-4 pl-7">
          <span className="text-title text-text">evie</span>
          <ConnectionLight state={state} />
          <div className="ml-auto">
            <ThemeMenu />
          </div>
        </header>
        {"reason" in state && state.reason !== undefined && (
          <p className="px-7 pb-3 text-caption text-status-working">
            <Words text={state.reason} />
          </p>
        )}
        <SessionList sessions={sessions} />
      </aside>

      <main className="flex flex-1 flex-col items-center justify-center gap-2 bg-bg px-7 py-8 text-center">
        <OpenSession sessions={sessions} />
      </main>

      <aside aria-label="Project" className="flex flex-col border-t border-line bg-bg wide:w-project-card wide:shrink-0 wide:border-t-0 wide:border-l">
        <header className="flex h-header-height shrink-0 items-center px-7">
          <h2 className="text-title text-text">Project</h2>
        </header>
        <p className="px-7 pb-6 text-caption text-text-muted">The open session's project will be shown here: its folder, its branch and what has changed.</p>
      </aside>
    </div>
  )
}

// The middle column, for now: the selected session's title, where its
// conversation will be. A session selected that the list does not hold yet
// is one with nothing said in it, as one just started before a reload.
function OpenSession({ sessions }: { sessions: Sessions }) {
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  const selected = useSelected()
  if (selected === undefined) {
    return (
      <>
        <h1 className="text-page-title text-text">No session open</h1>
        <p className="max-w-sign-in-column text-body text-text-muted">The conversation will be here: your messages, evie's replies and the steps it took.</p>
      </>
    )
  }
  if (!list.listed) return null
  const session = list.sessions.find((each) => each.session === selected)
  return (
    <>
      <h1 className="max-w-sign-in-column text-page-title text-text">{session === undefined ? "New session" : titleOf(session)}</h1>
      <p className="max-w-sign-in-column text-body text-text-muted">Its conversation will be shown here.</p>
    </>
  )
}

// The theme picker: Dusk, Moss or Oxblood, remembered in this browser.
function ThemeMenu() {
  const [theme, setTheme] = useState<Theme>(rememberedTheme)
  const name = THEMES.find((each) => each.id === theme)!.name
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="quiet" aria-label={`Theme: ${name}`}>
          <Swatch theme={theme} />
          {name}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuRadioGroup
          value={theme}
          onValueChange={(chosen) => {
            const picked = THEMES.find((each) => each.id === chosen)!.id
            applyTheme(picked)
            setTheme(picked)
          }}
        >
          {THEMES.map((each) => (
            <DropdownMenuRadioItem key={each.id} value={each.id}>
              <Swatch theme={each.id} />
              {each.name}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// A theme in small: its ground and its accent, taken from the theme itself.
function Swatch({ theme }: { theme: Theme }) {
  return (
    <span
      aria-hidden="true"
      data-theme={theme}
      className="size-icon-sm shrink-0 rounded-full border border-line"
      style={{ background: "linear-gradient(135deg, var(--bg) 50%, var(--accent) 50%)" }}
    />
  )
}
