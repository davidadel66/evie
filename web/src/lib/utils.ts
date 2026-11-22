import { clsx, type ClassValue } from "clsx"
import { extendTailwindMerge } from "tailwind-merge"

// The sizes Tailwind's theme names, beside its spacing steps: h-touch-min,
// w-session-list. Kept the same as the theme in styles.css.
export const NAMED_SIZES = [
  "row-height",
  "work-row",
  "header-height",
  "rail-width",
  "side-chat-width",
  "gutter",
  "touch-min",
  "icon-xs",
  "icon-caret",
  "icon-sm",
  "icon-control",
  "icon-md",
  "control",
  "control-sm",
  "menu-wide",
  "tile-row",
  "tile-row-icon",
  "tile-header",
  "tile-strip",
  "icon-picker",
  "icon-grid",
  "project-filter",
  "composer-text",
  "composer-menu",
  "picker-makers",
  "job-output",
  "session-list",
  "project-card",
  "card-sheet",
  "list-sheet",
  "palette",
  "palette-list",
  "notice",
  "workspace-name",
  "sign-in-column",
  "conversation",
  "call-name",
  "menu",
  "dialog",
  "diff-tree",
  "settings",
  "settings-height",
  "settings-pages",
  "settings-field",
  "attachment-chip",
  "attachment-shown",
  "image-open",
  "image-open-height",
] as const

// Joins class names, as every shadcn/ui part does, so that a class passed
// in wins over the part's own of the same kind. The joiner is told the
// design system's names, which Tailwind's theme in styles.css holds in
// place of its own: without them it would take `text-caption`, a size, for
// a colour, and drop it beside `text-text-muted`.
const merge = extendTailwindMerge({
  extend: {
    theme: {
      text: ["caption", "body", "prose", "field", "control", "label", "heading", "title", "page-title", "figure", "code"],
      spacing: [...NAMED_SIZES],
      shadow: ["float"],
      radius: ["tile"],
    },
  },
})

export function cn(...inputs: ClassValue[]) {
  return merge(clsx(inputs))
}
