// Adapted from shadcn/ui's dialog (MIT licence, notice in
// LICENSES/shadcn-ui-MIT.txt): only the parts the web app uses, styled to
// the design system. It floats, so it has a surface: a 1px line, 16px
// corners, as T3 Code's dialogs have, and the floating shadow, centred
// within 16px of the page's sides.
// Radix closes it on Escape, on a click outside and by its close button,
// keeps the focus inside it while it is open, and gives the focus back to
// the button that opened it.
import * as DialogPrimitive from "@radix-ui/react-dialog"
import type { ComponentProps } from "react"
import { cn } from "../../lib/utils.ts"

function Dialog(props: ComponentProps<typeof DialogPrimitive.Root>) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />
}

function DialogTrigger(props: ComponentProps<typeof DialogPrimitive.Trigger>) {
  return <DialogPrimitive.Trigger data-slot="dialog-trigger" {...props} />
}

function DialogClose(props: ComponentProps<typeof DialogPrimitive.Close>) {
  return <DialogPrimitive.Close data-slot="dialog-close" {...props} />
}

// A dialog with no description is named by its title alone. The layer
// under it, which takes a click outside, is clear: the board gives no
// colour to dim the page with.
function DialogContent({ className, ...props }: ComponentProps<typeof DialogPrimitive.Content>) {
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay data-slot="dialog-overlay" className="fixed inset-0 z-50" />
      <DialogPrimitive.Content
        data-slot="dialog-content"
        aria-describedby={undefined}
        className={cn("fixed inset-x-4 top-1/2 z-50 mx-auto flex max-w-dialog -translate-y-1/2 flex-col gap-4 rounded-lg border border-line bg-surface p-6 text-body text-text shadow-float", className)}
        {...props}
      />
    </DialogPrimitive.Portal>
  )
}

function DialogTitle({ className, ...props }: ComponentProps<typeof DialogPrimitive.Title>) {
  return <DialogPrimitive.Title data-slot="dialog-title" className={cn("text-page-title text-text", className)} {...props} />
}

export { Dialog, DialogClose, DialogContent, DialogTitle, DialogTrigger }
