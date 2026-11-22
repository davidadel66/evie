import * as PopoverPrimitive from "@radix-ui/react-popover"
import { Suspense, useEffect, useLayoutEffect, useRef, type ReactNode } from "react"
import { closeFloating, measured, toggleCard, useCardShown } from "./card-shown.ts"
import { Button } from "./components/ui/button.tsx"
import { Sheet, SheetContent, SheetTitle } from "./components/ui/sheet.tsx"
import type { Connection } from "./connection.ts"
import { Article } from "./icons.tsx"
import { useLoaded } from "./later.ts"
import { cn } from "./lib/utils.ts"
import { DetailsCard, SCREENS } from "./screens.ts"
import { useSelected } from "./selection.ts"
import type { Sessions } from "./sessions.ts"
import { label as keyLabel, onMac } from "./shortcuts.ts"

// The session details' place on the page, which the page has from the
// start: the button that opens them, the room kept clear for them beside
// the conversation, and the places they open in. The card itself, in
// ProjectCard.tsx, is loaded later, once the page is idle or the card is
// first opened (see screens.ts); opened before it is here, as when it is
// pinned open as the page loads, it shows once it is, whole.

// The card's place in the conversation's column, which it measures to know
// whether a pinned card has the room to sit there. It is given its place
// by the page: in place, in the column's top right corner; floating, from
// the same corner over the conversation; at phone width, as a sheet. A
// card floating closes when another session is picked.
export function CardPlace({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const shown = useCardShown()
  const selected = useSelected()
  const corner = useRef<HTMLDivElement>(null)
  const here = useLoaded(SCREENS)
  useEffect(() => {
    if (shown.open) SCREENS.preload()
  }, [shown.open])
  const open = shown.open && here
  // A click outside closes it, as does Escape, but a click on the header's
  // button, which toggles it; the focus goes back to that button, unless
  // the click outside took it elsewhere.
  const pointedAway = useRef(false)
  useEffect(() => closeFloating(), [selected])
  useLayoutEffect(() => {
    const column = corner.current?.parentElement
    if (!column) return
    const observer = new ResizeObserver(() => measured(column.clientWidth, column.clientHeight))
    observer.observe(column)
    measured(column.clientWidth, column.clientHeight)
    return () => observer.disconnect()
  }, [])
  const card = (
    <Suspense fallback={null}>
      <DetailsCard connection={connection} sessions={sessions} />
    </Suspense>
  )
  return (
    <>
      <PopoverPrimitive.Root open={shown.place === "popover" && open} onOpenChange={(open) => !open && closeFloating()}>
        <PopoverPrimitive.Anchor asChild>
          <div ref={corner} aria-hidden="true" className="pointer-events-none absolute top-details-top right-3 size-0" />
        </PopoverPrimitive.Anchor>
        <PopoverPrimitive.Portal>
          <PopoverPrimitive.Content
            aria-label="Session details"
            side="bottom"
            align="end"
            sideOffset={0}
            collisionPadding={12}
            onOpenAutoFocus={() => (pointedAway.current = false)}
            onPointerDownOutside={() => (pointedAway.current = true)}
            onInteractOutside={(event) => event.target instanceof Element && event.target.closest("[data-card-button]") !== null && event.preventDefault()}
            onCloseAutoFocus={(event) => {
              event.preventDefault()
              if (!pointedAway.current) document.querySelector<HTMLElement>("[data-card-button]")?.focus()
            }}
            className="z-40 flex max-h-(--radix-popover-content-available-height) w-details-card max-w-[calc(100vw-var(--spacing-6))] flex-col overflow-y-auto rounded-lg border border-text/10 text-detail text-text shadow-float details-glass"
          >
            {card}
          </PopoverPrimitive.Content>
        </PopoverPrimitive.Portal>
      </PopoverPrimitive.Root>
      {(shown.place === "inline" || shown.place === "over") && open && (
        // Pinned without the room beside the conversation, it stays open over it, with the floating shadow.
        <aside
          aria-label="Session details"
          className={cn(
            "absolute top-details-top right-3 z-20 flex max-h-details-height w-details-card max-w-[calc(100%-var(--spacing-6))] flex-col overflow-y-auto rounded-lg border border-text/10 text-detail text-text details-glass",
            shown.place === "over" && "shadow-float",
          )}
        >
          {card}
        </aside>
      )}
      <Sheet open={shown.place === "sheet" && open} onOpenChange={(open) => !open && closeFloating()}>
        <SheetContent aria-label="Session details" className="overflow-y-auto text-detail">
          <SheetTitle className="sr-only">Session details</SheetTitle>
          {card}
        </SheetContent>
      </Sheet>
    </>
  )
}

// The button in the conversation's header that opens and closes the card,
// raised while it is open.
export function CardButton() {
  const shown = useCardShown()
  return (
    <Button
      variant="icon"
      data-card-button=""
      aria-label="Show or hide session details"
      aria-pressed={shown.open}
      title={`Session details (${keyLabel("card", onMac())})`}
      onClick={toggleCard}
      className={cn("size-control-sm", shown.open && "bg-surface-raised text-text")}
    >
      <Article />
    </Button>
  )
}

// What is under the conversation's header, kept clear of the card while it
// is pinned open beside it, even while the card is on its way.
export function CardLane({ children }: { children: ReactNode }) {
  const shown = useCardShown()
  return <div className={cn("flex min-h-0 flex-1 flex-col", shown.place === "inline" && shown.open && "pr-details-clear")}>{children}</div>
}
