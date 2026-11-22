import { expect, test } from "bun:test"
import { readFile } from "node:fs/promises"
import { join } from "node:path"
import { cn, NAMED_SIZES } from "../web/src/lib/utils.ts"

// The helper the copied shadcn/ui parts join their classes with: a class
// given later wins over one of the same kind, and classes of different
// kinds are all kept, with the design system's names for them.

test("joining classes keeps a text size beside a text colour, and the later of two of the same kind", () => {
  expect(cn("text-caption text-text-muted", "text-status-needs")).toBe("text-caption text-status-needs")
  expect(cn("text-label text-text", "text-body")).toBe("text-text text-body")
  expect(cn("px-3 h-touch-min", false && "hidden", "px-4")).toBe("h-touch-min px-4")
  expect(cn("bg-surface-raised rounded-md", "bg-transparent")).toBe("rounded-md bg-transparent")
  expect(cn("h-touch-min w-session-list", "h-control")).toBe("w-session-list h-control")
})

test("the joiner knows every named size in Tailwind's theme", async () => {
  const styles = await readFile(join(import.meta.dir, "..", "web", "src", "styles.css"), "utf8")
  const named = [...styles.matchAll(/--spacing-([a-z][a-z-]*):/g)].map((match) => match[1]!)

  expect<string[]>([...NAMED_SIZES].sort()).toEqual(named.sort())
})
