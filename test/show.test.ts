import { afterEach, expect, setSystemTime, test } from "bun:test"
import { mkdir, mkdtemp, readdir, readFile, realpath, rm, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { permissions } from "../src/index.ts"
import { show } from "../src/plugins/show/index.ts"
import { say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

// The show tool: evie shows the owner a file it made, which is kept in the
// workspace's Storage, and the tool tells the model where and as what.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  setSystemTime()
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function start() {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  const script: ScriptedReply[] = []
  const run = await startEvie([scriptedProvider(script).plugin, show({ dataFolder: home })], { home })
  cleanups.push(run.stop)
  const folder = await realpath(run.projectFolder)
  // Has the model call show with these paths, in a session, and gives back
  // what the tool said, as the model reads it.
  async function call(paths: unknown, session = run.session) {
    script.push({ text: [], calls: [{ name: "show", input: { paths } }] }, { text: ["Here it is."] })
    await say(run, "Show me", session)
    return stored(run.events).findLast((event) => event.type === "tool_result")!.body.text
  }
  const put = async (name: string, bytes: Uint8Array | string) => {
    await mkdir(join(folder, name, ".."), { recursive: true })
    await writeFile(join(folder, name), bytes)
    return join(folder, name)
  }
  return { run, home, folder, call, put }
}

const bytes = (text: string) => new TextEncoder().encode(text)
// The openings of files, as each kind starts: all that is read of them.
function png(width = 4, height = 3): Uint8Array {
  const made = new Uint8Array(33)
  made.set([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52])
  new DataView(made.buffer).setUint32(16, width)
  new DataView(made.buffer).setUint32(20, height)
  return made
}
const jpeg = () => new Uint8Array([0xff, 0xd8, 0xff, 0xe0, 0, 4, 0, 0, 0xff, 0xc0, 0, 11, 8, 0, 3, 0, 4, 1, 1, 0x11, 0, 0xff, 0xd9])
const pdf = () => bytes("%PDF-1.7\n%âã\n1 0 obj << /Type /Catalog >> endobj\n%%EOF\n")
const svg = () => bytes('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><circle r="4"/></svg>\n')

test("a file's kind is read from its bytes, and a text kind's from its name: pictures, SVG, PDF and Markdown", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const { call, put } = await start()
  await put("out/chart.png", png())
  await put("out/photo", jpeg())
  await put("design/logo.svg", svg())
  await put("out/card.pdf", pdf())
  await put("docs/guide.md", "# Guide\n\nHow the cards look.\n")

  const said = await call(["out/chart.png", "out/photo", "design/logo.svg", "out/card.pdf", "docs/guide.md"])

  expect(said).toBe(
    [
      "Shown to the owner under your reply, in this order:",
      '- ".evie/shown/2026-11-22/chart.png" (image/png, 33 bytes), copied from out/chart.png',
      '- ".evie/shown/2026-11-22/photo" (image/jpeg, 23 bytes), copied from out/photo',
      '- ".evie/shown/2026-11-22/logo.svg" (image/svg+xml, 82 bytes), copied from design/logo.svg',
      '- ".evie/shown/2026-11-22/card.pdf" (application/pdf, 57 bytes), copied from out/card.pdf',
      '- ".evie/shown/2026-11-22/guide.md" (text/markdown, 29 bytes), copied from docs/guide.md',
      "Each copy stays as it is now. To show a later version, call show again.",
    ].join("\n"),
  )
})

test("each call keeps a new copy under the day, so an earlier reply keeps the version it showed; a name taken gets a number", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const { call, put, folder } = await start()
  await put("out/card.md", "# First\n")
  await put("drafts/card.md", "# Draft\n")
  const shownAt = (name: string) => join(folder, ".evie", "shown", "2026-11-22", name)

  expect(await call(["out/card.md"])).toStartWith('Shown to the owner under your reply:\n- ".evie/shown/2026-11-22/card.md" (text/markdown, 8 bytes)')
  await put("out/card.md", "# Second\n")
  expect(await call(["out/card.md", "drafts/card.md"])).toBe(
    [
      "Shown to the owner under your reply, in this order:",
      '- ".evie/shown/2026-11-22/card-2.md" (text/markdown, 9 bytes), copied from out/card.md',
      '- ".evie/shown/2026-11-22/card-3.md" (text/markdown, 8 bytes), copied from drafts/card.md',
      "Each copy stays as it is now. To show a later version, call show again.",
    ].join("\n"),
  )
  expect(await readFile(shownAt("card.md"), "utf8")).toBe("# First\n")
  expect(await readFile(shownAt("card-2.md"), "utf8")).toBe("# Second\n")
  expect(await readFile(shownAt("card-3.md"), "utf8")).toBe("# Draft\n")

  setSystemTime(new Date(2026, 10, 23, 8, 0))
  expect(await call(["out/card.md"])).toStartWith('Shown to the owner under your reply:\n- ".evie/shown/2026-11-23/card.md"')
})

test("a file already in the project's Storage is shown where it is, and not copied", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const { call, put, folder } = await start()
  await put(".evie/files/plan.md", "# Beds\n")
  await put("out/chart.png", png())

  expect(await call([".evie/files/plan.md", join(folder, "out", "chart.png")])).toBe(
    [
      "Shown to the owner under your reply, in this order:",
      '- ".evie/files/plan.md" (text/markdown, 7 bytes), already in Storage',
      '- ".evie/shown/2026-11-22/chart.png" (image/png, 33 bytes), copied from out/chart.png',
      "Each copy stays as it is now. To show a later version, call show again.",
    ].join("\n"),
  )
  expect(await call([".evie/shown/2026-11-22/chart.png"])).toBe('Shown to the owner under your reply:\n- ".evie/shown/2026-11-22/chart.png" (image/png, 33 bytes), already in Storage')
  expect(await Bun.file(join(folder, ".evie", "shown", "2026-11-22", "plan.md")).exists()).toBe(false)
  expect(await Bun.file(join(folder, ".evie", "shown", "2026-11-22", "chart-2.png")).exists()).toBe(false)
})

test("with no project, a file shown is kept for the session in the data folder, and deleted with the session", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const { run, home, call } = await start()
  const outside = await realpath(await mkdtemp(join(tmpdir(), "evie-made-")))
  cleanups.push(() => rm(outside, { recursive: true, force: true }))
  await writeFile(join(outside, "chart.png"), png())
  const { session } = await run.evie.send({ type: "open_session" })
  const kept = join(home, "shown", session, "2026-11-22", "chart.png")

  expect(await call([join(outside, "chart.png")], session)).toBe(
    [
      "Shown to the owner under your reply:",
      `- ${JSON.stringify(kept)} (image/png, 33 bytes), copied from ${join(outside, "chart.png")}`,
      "Each copy stays as it is now. To show a later version, call show again.",
    ].join("\n"),
  )
  expect([...(await Bun.file(kept).bytes())]).toEqual([...png()])
  expect(await call([kept], session)).toBe(`Shown to the owner under your reply:\n- ${JSON.stringify(kept)} (image/png, 33 bytes), already in Storage`)
  expect(await call(["chart.png"], session)).toBe('Not shown: This session is not in a project, so there is no folder to take "chart.png" from. Give the full path.')

  await run.evie.send({ type: "delete_session", session })
  expect(await readdir(join(home, "shown"))).toEqual([])
})

test("a project whose .evie folder is a link keeps what it shows for the session in the data folder, and writes nothing through the link", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const { run, home, call, put, folder } = await start()
  const elsewhere = await realpath(await mkdtemp(join(tmpdir(), "evie-elsewhere-")))
  cleanups.push(() => rm(elsewhere, { recursive: true, force: true }))
  await symlink(elsewhere, join(folder, ".evie"))
  await put("out/chart.png", png())

  expect(await call(["out/chart.png"])).toStartWith(`Shown to the owner under your reply:\n- ${JSON.stringify(join(home, "shown", run.session, "2026-11-22", "chart.png"))} (image/png, 33 bytes)`)
  expect(await readdir(elsewhere)).toEqual([])
})

test("a kind not on the list, a file too large, one not there and a folder are refused, saying what to do instead, and then nothing is shown", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const { call, put, folder } = await start()
  await put("out/chart.png", png())
  await put("notes.txt", "Beds.\n")
  await put("fake.png", "<html>not a picture</html>")
  await writeFile(join(folder, "big.pdf"), Buffer.concat([Buffer.from("%PDF-1.7\n"), Buffer.alloc(25 * 1024 * 1024)]))
  await mkdir(join(folder, "out", "pages"), { recursive: true })
  const kinds = "evie shows PNG, JPEG, GIF and WebP pictures, SVG, PDF and Markdown (.md)"

  expect(await call(["notes.txt"])).toBe(`Not shown: notes.txt is not a kind of file evie can show: ${kinds}. Save it as one of those, or tell the owner its path instead.`)
  expect(await call(["fake.png"])).toBe(`Not shown: fake.png is not a kind of file evie can show: ${kinds}. Save it as one of those, or tell the owner its path instead.`)
  expect(await call(["big.pdf"])).toBe("Not shown: big.pdf is 25 MB, more than the 25 MB a file shown can be. Make it smaller, or tell the owner its path instead.")
  expect(await call(["out/gone.png"])).toBe("Not shown: out/gone.png is not there. Check the path, or make the file first.")
  expect(await call(["out/pages"])).toBe("Not shown: out/pages is a folder. Give the path of each file in it to show.")
  expect(await call(["out/chart.png", "notes.txt", "out/gone.png"])).toBe(
    [
      "Not shown: nothing was shown, since some of these files cannot be.",
      `- notes.txt is not a kind of file evie can show: ${kinds}. Save it as one of those, or tell the owner its path instead.`,
      "- out/gone.png is not there. Check the path, or make the file first.",
      "Call show again with the files that can be shown.",
    ].join("\n"),
  )
  for (const given of [[], "out/chart.png", [3]]) {
    expect(await call(given)).toBe('Not shown: `paths` is a list of the files to show, at least one, such as ["out/chart.png"].')
  }
  expect(await Bun.file(join(folder, ".evie", "shown", "2026-11-22", "chart.png")).exists()).toBe(false)
})

test("the guard keeps show out of private folders, as it keeps the file tools, but lets a session show again what it showed", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  const script: ScriptedReply[] = []
  const run = await startEvie([scriptedProvider(script).plugin, show({ dataFolder: home }), permissions({ mode: "auto", dataFolder: home })], { home })
  cleanups.push(run.stop)
  const call = async (paths: string[], session: string) => {
    script.push({ text: [], calls: [{ name: "show", input: { paths } }] }, { text: ["Here it is."] })
    await say(run, "Show me", session)
    return stored(run.events).findLast((event) => event.type === "tool_result")!.body.text
  }
  const { session } = await run.evie.send({ type: "open_session" })
  const other = (await run.evie.send({ type: "open_session" })).session
  const theirs = (await run.evie.attachments.save(other, "photo.png", png())).path
  const made = await realpath(await mkdtemp(join(tmpdir(), "evie-made-")))
  cleanups.push(() => rm(made, { recursive: true, force: true }))
  await writeFile(join(made, "chart.png"), png())

  expect(await call([join(made, "chart.png"), theirs], session)).toStartWith(`Blocked by the guard: ${theirs} is in a private folder.`)
  expect(await call([join(made, "chart.png")], session)).toStartWith("Shown to the owner under your reply:")
  const kept = join(home, "shown", session, "2026-11-22", "chart.png")
  expect(await call([kept], session)).toBe(`Shown to the owner under your reply:\n- ${JSON.stringify(kept)} (image/png, 33 bytes), already in Storage`)
  expect(await call([kept], other)).toStartWith(`Blocked by the guard: ${kept} is in a private folder.`)
})

test("a shown folder that leads through a link elsewhere is refused, saying what to do, and nothing is written through it", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const { call, put, folder } = await start()
  const elsewhere = await realpath(await mkdtemp(join(tmpdir(), "evie-elsewhere-")))
  cleanups.push(() => rm(elsewhere, { recursive: true, force: true }))
  await put("out/chart.png", png())
  await mkdir(join(folder, ".evie"))
  await symlink(elsewhere, join(folder, ".evie", "shown"))

  expect(await call(["out/chart.png"])).toBe("Not shown: The folder shown files are kept in leads through a link to somewhere else, so nothing is shown there. Remove the link, and show it again.")
  expect(await readdir(elsewhere)).toEqual([])
})
