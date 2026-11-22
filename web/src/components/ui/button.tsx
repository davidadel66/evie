// Adapted from shadcn/ui's button (MIT licence, notice in
// LICENSES/shadcn-ui-MIT.txt), with the design system's kinds of button in
// place of shadcn's: primary, secondary, quiet and icon. Every button is
// 32px tall, as T3 Code's, unless it is given a smaller size, with 14px
// words and 16px icons (icon-control, which an icon in a button names),
// all a step larger below 640px, as T3 Code's are. It shows the focus ring
// when focused from the keyboard. On a touch screen it
// reaches an unseen 44px round itself, as T3 Code's buttons do, so that a
// finger finds it although it looks small.
import { cva, type VariantProps } from "class-variance-authority"
import type { ComponentProps } from "react"
import { cn } from "../../lib/utils.ts"

const buttonVariants = cva(
  "relative inline-flex shrink-0 items-center justify-center gap-2 rounded-md whitespace-nowrap disabled:bg-transparent disabled:text-text-muted pointer-coarse:after:absolute pointer-coarse:after:size-full pointer-coarse:after:min-h-touch-min pointer-coarse:after:min-w-touch-min [&>svg]:shrink-0",
  {
    variants: {
      variant: {
        // One on a screen at most.
        primary: "h-control bg-text px-3 text-control font-semibold text-bg",
        secondary: "h-control bg-surface-raised px-3 text-control font-semibold text-text",
        // For actions inside rows and headers.
        quiet: "h-control px-3 text-control text-text-muted hover:text-text",
        // An icon in a square, named by an aria-label.
        icon: "size-control text-text-muted hover:text-text [&>svg]:size-icon-control",
      },
    },
    defaultVariants: { variant: "secondary" },
  },
)

function Button({ className, variant, type = "button", ...props }: ComponentProps<"button"> & VariantProps<typeof buttonVariants>) {
  return <button data-slot="button" type={type} className={cn(buttonVariants({ variant }), className)} {...props} />
}

export { Button, buttonVariants }
