import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react"

export type MenuItem = {
  key: string
  label: string
  // A 16px icon or a swatch before the label.
  leading?: ReactNode
  // The item in use now, such as the theme the page is in.
  current?: boolean
  choose(): void
}

// A menu that floats over the page, as the design system draws it: a
// surface with a 1px line, 10px corners and the floating shadow, holding
// 40px items, the current one raised. It closes on Escape, on a click
// outside it and after a choice, and focus goes back to its button.
export function Menu({ label, button, items }: { label: string; button: ReactNode; items: MenuItem[] }) {
  const [open, setOpen] = useState(false)
  const opener = useRef<HTMLButtonElement>(null)
  const menu = useRef<HTMLDivElement>(null)

  const close = () => {
    setOpen(false)
    opener.current?.focus()
  }

  useEffect(() => {
    if (!open) return
    // The current item takes the focus, so the arrow keys start from it.
    menu.current?.querySelector<HTMLElement>('[aria-checked="true"]')?.focus()
    const outside = (event: PointerEvent) => {
      const target = event.target as Node
      if (!menu.current?.contains(target) && !opener.current?.contains(target)) setOpen(false)
    }
    document.addEventListener("pointerdown", outside)
    return () => document.removeEventListener("pointerdown", outside)
  }, [open])

  // Escape closes the menu; the arrow keys move between its items.
  function onKeyDown(event: KeyboardEvent) {
    if (event.key === "Escape") return close()
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return
    event.preventDefault()
    const choices = [...(menu.current?.querySelectorAll<HTMLElement>('[role="menuitemradio"]') ?? [])]
    const at = choices.indexOf(document.activeElement as HTMLElement)
    choices[(at + (event.key === "ArrowDown" ? 1 : -1) + choices.length) % choices.length]?.focus()
  }

  return (
    <div className="relative">
      <button
        ref={opener}
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={label}
        onClick={() => setOpen(!open)}
        className="flex h-touch-min items-center gap-2 rounded-md px-3 text-body text-text-muted hover:text-text"
      >
        {button}
      </button>
      {open && (
        <div
          ref={menu}
          role="menu"
          aria-label={label}
          onKeyDown={onKeyDown}
          className="absolute top-full right-0 z-10 mt-1 flex min-w-menu flex-col rounded-md border border-line bg-surface p-2 shadow-float"
        >
          {items.map((item) => (
            <button
              key={item.key}
              type="button"
              role="menuitemradio"
              aria-checked={item.current === true}
              onClick={() => {
                item.choose()
                close()
              }}
              className="flex h-menu-item items-center gap-2 rounded-md px-3 text-left text-body text-text aria-checked:bg-surface-raised"
            >
              {item.leading}
              {item.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
