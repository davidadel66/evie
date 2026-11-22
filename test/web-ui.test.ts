import { expect, test } from "bun:test"
import { readdir, readFile } from "node:fs/promises"
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
  // A size can be set again for a width, as the controls are at phone width.
  const named = [...new Set([...styles.matchAll(/--spacing-([a-z][a-z-]*):/g)].map((match) => match[1]!))]

  expect<string[]>([...NAMED_SIZES].sort()).toEqual(named.sort())
})

test("every text box in the web app takes the field size for its text, which is 16px at phone width", async () => {
  const folder = join(import.meta.dir, "..", "web", "src")
  const files = (await readdir(folder, { recursive: true })).filter((file) => file.endsWith(".tsx"))
  const boxes: { where: string; tag: string }[] = []
  for (const file of files) {
    const source = await readFile(join(folder, file), "utf8")
    // Each text box is written as one self-closing element.
    for (const match of source.matchAll(/<(input|textarea|Input)\b[\s\S]*?\/>/g)) {
      if (/type="(checkbox|file)"|\bhidden\b/.test(match[0])) continue
      boxes.push({ where: `${file}: ${match[0].slice(0, 60)}`, tag: match[0] })
    }
  }

  expect(boxes.length).toBeGreaterThan(8)
  for (const { where, tag } of boxes) {
    const classes = tag.match(/className=(?:"([^"]*)"|\{cn\("([^"]*)")/)
    const own = classes?.[1] ?? classes?.[2] ?? ""
    // Input, the shared part, has the size itself; given classes must not change it.
    const size = own.match(/\btext-(caption|body|prose|label|title|page-title|figure|code)\b/)
    expect({ where, size: size?.[0] }).toEqual({ where, size: undefined })
    if (!tag.startsWith("<Input")) expect({ where, field: own.includes("text-field") }).toEqual({ where, field: true })
  }
})
