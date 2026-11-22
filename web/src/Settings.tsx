import { useState, type ReactNode } from "react"
import { Button } from "./components/ui/button.tsx"
import { Dialog, DialogClose, DialogContent, DialogTitle, DialogTrigger } from "./components/ui/dialog.tsx"
import { GearSix, X } from "./icons.tsx"
import { applyTheme, rememberedTheme, THEMES, type Theme } from "./theme.ts"

// The settings: a gear that opens a small window of them, a section each.
// For now there is one, the theme.
export function Settings() {
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="icon" aria-label="Settings">
          <GearSix />
        </Button>
      </DialogTrigger>
      <DialogContent>
        <div className="flex items-center justify-between gap-4">
          <DialogTitle>Settings</DialogTitle>
          {/* The button's own padding brings its icon to the window's edge. */}
          <DialogClose asChild>
            <Button variant="icon" aria-label="Close" className="-mr-2">
              <X />
            </Button>
          </DialogClose>
        </div>
        <Section title="Theme">
          <ThemeChoice />
        </Section>
      </DialogContent>
    </Dialog>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section aria-label={title} className="flex flex-col gap-2">
      <h3 className="text-title text-text">{title}</h3>
      {children}
    </section>
  )
}

// Dusk, Moss or Oxblood, each with its swatch, the one in use raised.
// Choosing one puts the page in it at once, and this browser remembers it.
function ThemeChoice() {
  const [theme, setTheme] = useState<Theme>(rememberedTheme)
  return (
    <div role="radiogroup" aria-label="Theme" className="flex flex-col">
      {THEMES.map((each) => (
        <button
          key={each.id}
          type="button"
          role="radio"
          aria-checked={each.id === theme}
          onClick={() => {
            applyTheme(each.id)
            setTheme(each.id)
          }}
          className="flex h-control items-center gap-2 rounded-md px-3 text-left text-body text-text aria-checked:bg-surface-raised"
        >
          <Swatch theme={each.id} />
          {each.name}
        </button>
      ))}
    </div>
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
