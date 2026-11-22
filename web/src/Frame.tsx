import { useState } from "react"
import { ConnectionLight } from "./ConnectionLight.tsx"
import type { ConnectionState } from "./connection.ts"
import { Menu } from "./Menu.tsx"
import { applyTheme, rememberedTheme, THEMES, type Theme } from "./theme.ts"
import { Words } from "./Words.tsx"

// The page once signed in: the session list, the conversation and the
// project card, in three columns, which stack at phone width. Each holds,
// for now, a note of what will go there.
export function Frame({ state }: { state: ConnectionState }) {
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
        <p className="px-7 pt-2 pb-6 text-caption text-text-muted">Your sessions will be listed here, with the ones that need you at the top.</p>
      </aside>

      <main className="flex flex-1 flex-col items-center justify-center gap-2 bg-bg px-7 py-8 text-center">
        <h1 className="text-page-title text-text">No session open</h1>
        <p className="max-w-sign-in-column text-body text-text-muted">The conversation will be here: your messages, evie's replies and the steps it took.</p>
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

// The theme picker: Dusk, Moss or Oxblood, remembered in this browser.
function ThemeMenu() {
  const [theme, setTheme] = useState<Theme>(rememberedTheme)
  const name = THEMES.find((each) => each.id === theme)!.name
  return (
    <Menu
      label={`Theme: ${name}`}
      button={
        <>
          <Swatch theme={theme} />
          {name}
        </>
      }
      items={THEMES.map((each) => ({
        key: each.id,
        label: each.name,
        leading: <Swatch theme={each.id} />,
        current: each.id === theme,
        choose() {
          applyTheme(each.id)
          setTheme(each.id)
        },
      }))}
    />
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
