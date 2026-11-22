import { expect, test } from "bun:test"
import { createElement, Suspense, type ReactNode } from "react"
import { renderToString } from "react-dom/server"
import { later, lazyFrom, Screen, whenIdle } from "../web/src/later.ts"
import { anyOwnIcon, ICON_SET, keepLooks } from "../web/src/project-look.ts"

// What the page loads later: a screen opened rarely or later is built into
// a file of its own, fetched once it is wanted or once the page is idle,
// and drawn at once from then on.

test("a module loaded later is fetched once, however often it is asked for, and is there to draw at once after", async () => {
  let fetched = 0
  const screen = later(async () => {
    fetched++
    return { name: "settings" }
  })
  expect(screen.loaded()).toBeUndefined()

  const [first, second] = await Promise.all([screen.load(), screen.load()])
  screen.preload()

  expect(fetched).toBe(1)
  expect(first).toBe(second)
  expect(screen.loaded()).toEqual({ name: "settings" })
})

test("those waiting for a module are told when it arrives", async () => {
  const screen = later(async () => ({ name: "palette" }))
  let told = 0
  const stop = screen.subscribe(() => told++)

  await screen.load()
  stop()
  await screen.load()

  expect(told).toBe(1)
})

test("a module that could not be fetched is fetched again the next time it is asked for", async () => {
  let fetched = 0
  const screen = later(async () => {
    fetched++
    if (fetched === 1) throw new Error("offline")
    return { name: "card" }
  })

  expect(screen.load()).rejects.toThrow("offline")
  await Bun.sleep(0)
  screen.preload()
  await Bun.sleep(0)

  expect(fetched).toBe(2)
  expect(screen.loaded()).toEqual({ name: "card" })
})

test("a screen whose file is already here is drawn in the same render, without a frame of its fallback", async () => {
  const module = later(async () => ({ Settings: () => createElement("p", null, "The settings") }))
  const Settings = lazyFrom(module, (loaded) => loaded.Settings)
  const page = () => renderToString(createElement(Suspense, { fallback: createElement("p", null, "Loading") }, createElement(Settings)))

  await module.load()

  expect(page()).toContain("The settings")
  expect(page()).not.toContain("Loading")
})

test("a screen asked for before its file is here waits for it in its fallback", () => {
  const module = later(() => new Promise<{ Palette: () => ReactNode }>(() => {}))
  const Palette = lazyFrom(module, (loaded) => loaded.Palette)

  const page = renderToString(createElement(Suspense, { fallback: createElement("p", null, "Loading") }, createElement(Palette)))

  expect(page).toContain("Loading")
})

test("a screen is not on the page until it is wanted, shows its fallback until its file is here, and from then on is there, wanted or not", async () => {
  const module = later(async () => ({ Palette: () => createElement("p", null, "The palette") }))
  const Palette = lazyFrom(module, (loaded) => loaded.Palette)
  const page = (wanted: boolean) => renderToString(createElement(Screen, { loads: module, wanted, fallback: createElement("p", null, "Loading"), children: createElement(Palette) }))

  expect(page(false)).toBe("")
  expect(page(true)).toContain("Loading")
  await module.load()
  expect(page(true)).toContain("The palette")
  expect(page(false)).toContain("The palette")
})

test("a screen whose file could not be fetched leaves the page as it is, rather than failing it", () => {
  const module = later<{ Card: () => ReactNode }>(() => Promise.reject(new Error("offline")))
  const Card = lazyFrom(module, (loaded) => loaded.Card)
  module.preload()

  const page = renderToString(createElement("main", null, createElement(Screen, { loads: module, wanted: true, children: createElement(Card) })))

  expect(page).toBe("<main></main>")
})

test("work put off until the page is idle runs then, unless called off first", async () => {
  const ran: string[] = []
  whenIdle(() => ran.push("prefetch"))
  const off = whenIdle(() => ran.push("called off"))
  off()

  await Bun.sleep(400)

  expect(ran).toEqual(["prefetch"])
})

test("the drawings of every icon but the folder are needed only once a project has an icon of its own, and are fetched then", async () => {
  expect(anyOwnIcon([{ id: "p-folder" }, { id: "p-picked-folder", icon: "folder", colour: "rust" }])).toBe(false)
  expect(anyOwnIcon([{ id: "p-folder" }, { id: "p-leaf", icon: "leaf", colour: "moss" }])).toBe(true)

  keepLooks([{ id: "p-folder" }, { id: "p-leaf", icon: "leaf", colour: "moss" }])
  await Bun.sleep(50)

  expect(ICON_SET.loaded()?.PROJECT_ICONS.map((icon) => icon.name)).toContain("leaf")
})
