// Adapted from shadcn/ui's button (MIT licence, notice in
// LICENSES/shadcn-ui-MIT.txt), with the design system's kinds of button in
// place of shadcn's: primary, secondary, quiet and icon. Every button is at
// least 40px tall, and shows the focus ring when focused from the keyboard.
import { cva, type VariantProps } from "class-variance-authority"
import type { ComponentProps } from "react"
import { cn } from "../../lib/utils.ts"

const buttonVariants = cva("inline-flex shrink-0 items-center justify-center gap-2 rounded-md whitespace-nowrap disabled:bg-transparent disabled:text-text-muted [&>svg]:shrink-0", {
  variants: {
    variant: {
      // One on a screen at most.
      primary: "h-control bg-text px-4 text-label text-bg",
      secondary: "h-control bg-surface-raised px-4 text-label text-text",
      // For actions inside rows and headers.
      quiet: "h-control px-3 text-body text-text-muted hover:text-text",
      // A 20px icon in a square, named by an aria-label.
      icon: "size-control text-text-muted hover:text-text [&>svg]:size-icon-md",
    },
  },
  defaultVariants: { variant: "secondary" },
})

function Button({ className, variant, type = "button", ...props }: ComponentProps<"button"> & VariantProps<typeof buttonVariants>) {
  return <button data-slot="button" type={type} className={cn(buttonVariants({ variant }), className)} {...props} />
}

export { Button, buttonVariants }
