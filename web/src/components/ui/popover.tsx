// Adapted from shadcn/ui's popover (MIT licence, notice in
// LICENSES/shadcn-ui-MIT.txt): only the parts the web app uses, styled to
// the design system's Menu, as the project icon picker is. It floats, so it
// has a surface: a 1px line, 10px corners, the floating shadow and 8px
// padding. Radix closes it on Escape and on a click outside, and gives the
// focus back to the button that opened it.
import * as PopoverPrimitive from "@radix-ui/react-popover"
import type { ComponentProps } from "react"
import { cn } from "../../lib/utils.ts"

function Popover(props: ComponentProps<typeof PopoverPrimitive.Root>) {
  return <PopoverPrimitive.Root data-slot="popover" {...props} />
}

function PopoverTrigger(props: ComponentProps<typeof PopoverPrimitive.Trigger>) {
  return <PopoverPrimitive.Trigger data-slot="popover-trigger" {...props} />
}

// Kept within the room the page has, and scrolled inside when it holds more.
function PopoverContent({ className, sideOffset = 4, align = "start", ...props }: ComponentProps<typeof PopoverPrimitive.Content>) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        data-slot="popover-content"
        sideOffset={sideOffset}
        align={align}
        collisionPadding={16}
        className={cn(
          "z-50 flex max-h-(--radix-popover-content-available-height) flex-col overflow-y-auto rounded-md border border-line bg-surface p-2 text-body text-text shadow-float",
          className,
        )}
        {...props}
      />
    </PopoverPrimitive.Portal>
  )
}

export { Popover, PopoverContent, PopoverTrigger }
