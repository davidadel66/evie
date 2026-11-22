// Adapted from shadcn/ui's sheet (MIT licence, notice in
// LICENSES/shadcn-ui-MIT.txt): only the side the web app uses, the right,
// styled to the design system. It floats over the page, so it has a
// surface, with a 1px line on its open side and the floating shadow, and
// slides in from the right, as T3 Code's right panel does at narrow
// widths. Radix closes it on Escape and on a tap outside, keeps the focus
// inside it while it is open, and gives the focus back to the button that
// opened it.
import * as SheetPrimitive from "@radix-ui/react-dialog"
import type { ComponentProps } from "react"
import { cn } from "../../lib/utils.ts"

function Sheet(props: ComponentProps<typeof SheetPrimitive.Root>) {
  return <SheetPrimitive.Root data-slot="sheet" {...props} />
}

// The layer under it, which takes a tap outside, is clear, as the dialog's is.
function SheetContent({ className, ...props }: ComponentProps<typeof SheetPrimitive.Content>) {
  return (
    <SheetPrimitive.Portal>
      <SheetPrimitive.Overlay data-slot="sheet-overlay" className="fixed inset-0 z-50" />
      <SheetPrimitive.Content
        data-slot="sheet-content"
        aria-describedby={undefined}
        className={cn(
          "fixed inset-y-0 right-0 z-50 flex w-card-sheet flex-col border-l border-line bg-surface text-body text-text shadow-float data-[state=closed]:animate-slide-out data-[state=open]:animate-slide-in motion-reduce:animate-none",
          className,
        )}
        {...props}
      />
    </SheetPrimitive.Portal>
  )
}

function SheetTitle({ className, ...props }: ComponentProps<typeof SheetPrimitive.Title>) {
  return <SheetPrimitive.Title data-slot="sheet-title" className={className} {...props} />
}

export { Sheet, SheetContent, SheetTitle }
