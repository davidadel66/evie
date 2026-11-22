// Adapted from shadcn/ui's input (MIT licence, notice in
// LICENSES/shadcn-ui-MIT.txt): a text box on the page, marked by a 1px line,
// not a surface, since it does not float. Its text is the field size,
// 16px at phone width, so that iOS does not zoom in when it is tapped.
import type { ComponentProps } from "react"
import { cn } from "../../lib/utils.ts"

function Input({ className, type, ...props }: ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn("h-control w-full min-w-0 rounded-md border border-line bg-transparent px-3 text-field text-text placeholder:text-text-muted", className)}
      {...props}
    />
  )
}

export { Input }
