// A switch for a setting that is on or off, as T3 Code's settings have
// (MIT licence, notice in LICENSES/t3code-MIT.txt): a round track, filled
// in the main button's colour while on, with its knob at the end it is
// switched to. Named by its own label, it says whether it is on as a
// switch does.
import type { ComponentProps } from "react"
import { cn } from "../../lib/utils.ts"

function Switch({ checked, onCheckedChange, className, ...props }: Omit<ComponentProps<"button">, "onChange"> & { checked: boolean; onCheckedChange(checked: boolean): void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      data-slot="switch"
      onClick={() => onCheckedChange(!checked)}
      className={cn(
        "relative inline-flex h-switch-height w-switch-width shrink-0 items-center rounded-full p-0.5 transition-colors motion-reduce:transition-none pointer-coarse:after:absolute pointer-coarse:after:-inset-3",
        checked ? "bg-text" : "bg-surface-raised",
        className,
      )}
      {...props}
    >
      <span aria-hidden="true" className={cn("block size-switch-knob rounded-full transition-transform motion-reduce:transition-none", checked ? "translate-x-switch-travel bg-bg" : "bg-text-muted")} />
    </button>
  )
}

export { Switch }
