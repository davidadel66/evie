// Adapted from shadcn/ui's dropdown menu (MIT licence, notice in
// LICENSES/shadcn-ui-MIT.txt): only the parts the web app uses, styled to
// the design system's Menu. It floats, so it has a surface: a 1px line,
// 8px corners, the floating shadow and 4px padding, holding 28px items, as
// T3 Code's menus do; the current item is raised. Radix closes it on Escape, on a click
// outside and after a choice, and gives focus back to its button.
import * as DropdownMenuPrimitive from "@radix-ui/react-dropdown-menu"
import type { ComponentProps } from "react"
import { CaretRight } from "../../icons.tsx"
import { cn } from "../../lib/utils.ts"

function DropdownMenu(props: ComponentProps<typeof DropdownMenuPrimitive.Root>) {
  return <DropdownMenuPrimitive.Root data-slot="dropdown-menu" {...props} />
}

function DropdownMenuTrigger(props: ComponentProps<typeof DropdownMenuPrimitive.Trigger>) {
  return <DropdownMenuPrimitive.Trigger data-slot="dropdown-menu-trigger" {...props} />
}

// Kept within the room the page has, and scrolled inside when it holds more.
function DropdownMenuContent({ className, sideOffset = 4, ...props }: ComponentProps<typeof DropdownMenuPrimitive.Content>) {
  return (
    <DropdownMenuPrimitive.Portal>
      <DropdownMenuPrimitive.Content
        data-slot="dropdown-menu-content"
        sideOffset={sideOffset}
        className={cn(
          "z-50 flex max-h-(--radix-dropdown-menu-content-available-height) min-w-menu flex-col overflow-y-auto rounded-md border border-line bg-surface p-1 text-body text-text shadow-float",
          className,
        )}
        {...props}
      />
    </DropdownMenuPrimitive.Portal>
  )
}

// The items' shared look. An icon leading an item is 16px.
const ITEM = "flex h-control-sm shrink-0 cursor-default items-center gap-2 rounded-sm px-2 text-left select-none [&>svg]:size-icon-sm [&>svg]:shrink-0"

// An item that does something when chosen. `current` raises it, as for the
// project a new session starts in when nothing else is picked.
function DropdownMenuItem({ className, current, ...props }: ComponentProps<typeof DropdownMenuPrimitive.Item> & { current?: boolean }) {
  return (
    <DropdownMenuPrimitive.Item
      data-slot="dropdown-menu-item"
      data-current={current === true ? "" : undefined}
      className={cn(ITEM, "data-current:bg-surface-raised", className)}
      {...props}
    />
  )
}

function DropdownMenuRadioGroup(props: ComponentProps<typeof DropdownMenuPrimitive.RadioGroup>) {
  return <DropdownMenuPrimitive.RadioGroup data-slot="dropdown-menu-radio-group" {...props} />
}

// One of a choice, such as a theme: the one chosen now is raised.
function DropdownMenuRadioItem({ className, ...props }: ComponentProps<typeof DropdownMenuPrimitive.RadioItem>) {
  return <DropdownMenuPrimitive.RadioItem data-slot="dropdown-menu-radio-item" className={cn(ITEM, "data-[state=checked]:bg-surface-raised", className)} {...props} />
}

// A heading for the items below it, in a detail's size and colour.
function DropdownMenuLabel({ className, ...props }: ComponentProps<typeof DropdownMenuPrimitive.Label>) {
  return <DropdownMenuPrimitive.Label data-slot="dropdown-menu-label" className={cn("px-2 py-1 text-caption text-text-muted", className)} {...props} />
}

function DropdownMenuSeparator({ className, ...props }: ComponentProps<typeof DropdownMenuPrimitive.Separator>) {
  return <DropdownMenuPrimitive.Separator data-slot="dropdown-menu-separator" className={cn("my-1 h-(--line-width) shrink-0 bg-line", className)} {...props} />
}

// A menu inside a menu, such as the projects to move a session to. Its
// item leads to it with a caret on the right, and opens it on a hover, a
// tap, or the right arrow key; it floats beside the menu, styled as one.
function DropdownMenuSub(props: ComponentProps<typeof DropdownMenuPrimitive.Sub>) {
  return <DropdownMenuPrimitive.Sub data-slot="dropdown-menu-sub" {...props} />
}

function DropdownMenuSubTrigger({ className, children, ...props }: ComponentProps<typeof DropdownMenuPrimitive.SubTrigger>) {
  return (
    <DropdownMenuPrimitive.SubTrigger data-slot="dropdown-menu-sub-trigger" className={cn(ITEM, "data-[state=open]:bg-surface-raised", className)} {...props}>
      {children}
      <CaretRight className="ml-auto text-text-muted" />
    </DropdownMenuPrimitive.SubTrigger>
  )
}

function DropdownMenuSubContent({ className, ...props }: ComponentProps<typeof DropdownMenuPrimitive.SubContent>) {
  return (
    <DropdownMenuPrimitive.Portal>
      <DropdownMenuPrimitive.SubContent
        data-slot="dropdown-menu-sub-content"
        className={cn(
          "z-50 flex max-h-(--radix-dropdown-menu-content-available-height) min-w-menu flex-col overflow-y-auto rounded-md border border-line bg-surface p-1 text-body text-text shadow-float",
          className,
        )}
        {...props}
      />
    </DropdownMenuPrimitive.Portal>
  )
}

export {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
}
