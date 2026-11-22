import { afterEach, expect, setSystemTime, test } from "bun:test"
import { mkdir, mkdtemp, readdir, readFile, realpath, rm, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { ATTACHMENT_LIMIT, type Attachment } from "../src/core/types.ts"
import { files, permissions } from "../src/index.ts"
import { say, startEvie, stored, type Run } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"
import { serveEvie, type Served } from "./support/served.ts"

// Files the owner attaches to a message: kept in the project's .evie
// folder, or in the data folder for a session with no project, recorded on
// the message, and shown to the model as images when it takes them, or by
// their path for it to read.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  setSystemTime()
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function start(script: ScriptedReply[] = [{ text: ["Seen."] }], images?: (model: string) => boolean): Promise<Run & { model: ReturnType<typeof scriptedProvider> }> {
  const model = scriptedProvider(script, { ...(images && { images }) })
  const run = await startEvie([model.plugin])
  cleanups.push(run.stop)
  return { ...run, model }
}

const bytes = (text: string) => new TextEncoder().encode(text)

test("a file attached in a project is kept in its .evie folder, under the day, and the message records it", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const run = await start()

  const attached = await run.evie.attachments.save(run.session, "notes.txt", "text/plain", bytes("Plant the beans.\n"))
  const folder = await realpath(run.projectFolder)
  expect(attached).toEqual({ path: join(folder, ".evie", "attachments", "2026-11-22", "notes.txt"), name: "notes.txt", type: "text/plain", size: 17 })
  expect(await readFile(attached.path, "utf8")).toBe("Plant the beans.\n")

  await run.evie.send({ type: "send_message", session: run.session, text: "Read this", attachments: [attached] })
  await Bun.sleep(0)
  const message = stored(run.events).find((event) => event.type === "user_message")
  expect(message?.body).toEqual({ text: "Read this", attachments: [attached] })
})

test("a message can carry only files kept for its session: any other path is refused, and so is one kept but since changed", async () => {
  const run = await start()
  const kept = await run.evie.attachments.save(run.session, "plan.txt", "text/plain", bytes("Beds.\n"))
  const outside = join(await realpath(run.home), "evie.db")
  await writeFile(join(run.projectFolder, "loose.txt"), "Not attached.\n")
  const loose = join(await realpath(run.projectFolder), "loose.txt")

  for (const given of [
    { ...kept, path: outside },
    { ...kept, path: loose },
    { ...kept, path: `${kept.path}/../../../../loose.txt` },
    { ...kept, path: `${kept.path}.gone` },
    { ...kept, size: 99 },
  ]) {
    await expect(run.evie.send({ type: "send_message", session: run.session, text: "Read this", attachments: [given] })).rejects.toThrow(
      `${given.name} is not a file kept for this session, so it cannot go with the message. Attach it again.`,
    )
  }
  const link = join(kept.path, "..", "link.txt")
  await symlink(loose, link)
  await expect(run.evie.send({ type: "send_message", session: run.session, text: "Read this", attachments: [{ ...kept, path: link, size: 14 }] })).rejects.toThrow("is not a file kept")
  await expect(run.evie.send({ type: "send_message", session: run.session, text: "Read this", attachments: "plan.txt" as never })).rejects.toThrow(
    "`attachments` are the files kept for the session that go with the message, as the attachments route gave them back.",
  )
  expect(stored(run.events).some((event) => event.type === "user_message")).toBe(false)
})

test("nothing is written through a link: an attachments folder that leads elsewhere is refused, and a .evie that does keeps them in the data folder", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const run = await start()
  const outside = await realpath(await mkdtemp(join(tmpdir(), "evie-outside-")))
  cleanups.push(() => rm(outside, { recursive: true, force: true }))
  await mkdir(join(run.projectFolder, ".evie"))
  await symlink(outside, join(run.projectFolder, ".evie", "attachments"))

  await expect(run.evie.attachments.save(run.session, "plan.txt", "text/plain", bytes("x"))).rejects.toThrow(
    "This project's .evie/attachments folder leads through a link to somewhere else, so nothing is attached there. Remove the link, and attach it again.",
  )
  expect(await readdir(outside)).toEqual([])

  await rm(join(run.projectFolder, ".evie"), { recursive: true })
  await symlink(outside, join(run.projectFolder, ".evie"))
  const kept = await run.evie.attachments.save(run.session, "plan.txt", "text/plain", bytes("x"))
  expect(kept.path).toBe(join(await realpath(run.home), "attachments", run.session, "2026-11-22", "plan.txt"))
  expect(await readdir(outside)).toEqual([])
})

// ---- What the model is shown ----------------------------------------------------------

// Four bytes standing for a picture; what is in them does not matter here.
const PICTURE = new Uint8Array([137, 80, 78, 71])

test("an image goes to a model that takes images as an image in the owner's message, and any other file by its path, for the model to read", async () => {
  const run = await start([{ text: ["A seedling."] }], (model) => model === "test/model")
  const photo = await run.evie.attachments.save(run.session, "photo.png", "image/png", PICTURE)
  const plan = await run.evie.attachments.save(run.session, "plan.txt", "text/plain", bytes("Beds.\n"))

  await say(run, "What is this?", run.session, [photo, plan])

  expect(run.model.requests[0]?.messages).toEqual([
    {
      role: "user",
      text:
        "What is this?\n\n" +
        `[Attached: photo.png (image/png, 4 bytes), kept at ${photo.path}. It is shown below.]\n` +
        `[Attached: plan.txt (text/plain, 6 bytes), kept at ${plan.path}. Read it with your tools to see what it holds.]`,
      images: [{ type: "image/png", data: Buffer.from(PICTURE).toString("base64") }],
    },
  ])
})

test("a model that does not take images is given an image's path instead, and so are the images before it once the session switches to one", async () => {
  const run = await start([{ text: ["A seedling."] }, { text: ["Still a seedling."] }], (model) => model === "test/model")
  const photo = await run.evie.attachments.save(run.session, "photo.png", "image/png", PICTURE)
  await say(run, "", run.session, [photo])
  await run.evie.send({ type: "set_model", session: run.session, model: "test/words-only" })

  await say(run, "And now?")

  const notShown = `[Attached: photo.png (image/png, 4 bytes), kept at ${photo.path}. It cannot be shown to you as an image here, so only its path is given.]`
  expect(run.model.requests[0]?.messages[0]).toEqual({ role: "user", text: `[Attached: photo.png (image/png, 4 bytes), kept at ${photo.path}. It is shown below.]`, images: [{ type: "image/png", data: "iVBORw==" }] })
  expect(run.model.requests[1]?.messages).toEqual([
    { role: "user", text: notShown },
    { role: "assistant", text: "A seedling." },
    { role: "user", text: "And now?" },
  ])
})

test("with no project, the model may read the files attached in its own session, and not another session's; deleting the session deletes them", async () => {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  const script: ScriptedReply[] = []
  const run = await startEvie([scriptedProvider(script).plugin, files(), permissions({ mode: "auto", dataFolder: home })], { home })
  cleanups.push(run.stop)
  const { session } = await run.evie.send({ type: "open_session" })
  const other = (await run.evie.send({ type: "open_session" })).session
  const plan = await run.evie.attachments.save(session, "plan.txt", "text/plain", bytes("Beds.\n"))
  const read = async (from: string) => {
    script.push({ text: [], calls: [{ name: "read", input: { path: plan.path } }] }, { text: ["Read."] })
    await say(run, "Read it", from)
    return stored(run.events).findLast((event) => event.type === "tool_result")!.body.text
  }

  expect(await read(session)).toBe("1: Beds.")
  expect(await read(other)).toStartWith("Blocked by the guard:")

  await run.evie.send({ type: "delete_session", session })
  expect(await Bun.file(plan.path).exists()).toBe(false)
  expect(await readdir(join(home, "attachments"))).toEqual([])
})

// ---- The upload route ----------------------------------------------------------------

async function serve() {
  const served = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(served.stop)
  return served
}

// Sends a file to the server's attachments route, as the web app does.
function upload(served: Served, session: string, name: string, body: string | Uint8Array, options: { token?: string | null; type?: string; origin?: string } = {}) {
  const token = options.token === undefined ? served.token : options.token
  const query = new URLSearchParams({ session, name })
  return fetch(`${served.server.url}/attachments?${query}`, {
    method: "POST",
    body,
    headers: {
      ...(token !== null && { Authorization: `Bearer ${token}` }),
      ...(options.type !== undefined && { "Content-Type": options.type }),
      ...(options.origin !== undefined && { Origin: options.origin }),
    },
  })
}

test("a signed-in client uploads a file, which is kept in the session's project and given back as the message records it; a client not signed in is refused", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const served = await serve()
  const { session, projectFolder } = served.run

  const refused = await upload(served, session, "plan.txt", "Beds.\n", { token: null })
  expect(refused.status).toBe(401)
  expect((await upload(served, session, "plan.txt", "Beds.\n", { token: "not-a-token" })).status).toBe(401)
  expect((await upload(served, session, "plan.txt", "Beds.\n", { origin: "https://elsewhere.example" })).status).toBe(403)
  expect(await Bun.file(join(projectFolder, ".evie", "attachments", "2026-11-22", "plan.txt")).exists()).toBe(false)

  const taken = await upload(served, session, "plan.txt", "Beds.\n", { type: "text/plain" })
  expect(taken.status).toBe(200)
  const path = join(await realpath(projectFolder), ".evie", "attachments", "2026-11-22", "plan.txt")
  expect(await taken.json()).toEqual({ path, name: "plan.txt", type: "text/plain", size: 6 })
  expect(await readFile(path, "utf8")).toBe("Beds.\n")
})

test("with no project, an upload is kept in the data folder, under the session", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const served = await serve()
  const { session } = await served.run.evie.send({ type: "open_session" })

  const taken = await upload(served, session, "photo.png", new Uint8Array([137, 80, 78, 71]), { type: "image/png" })
  const path = join(await realpath(served.run.home), "attachments", session, "2026-11-22", "photo.png")
  expect(await taken.json()).toEqual({ path, name: "photo.png", type: "image/png", size: 4 })
  expect([...(await Bun.file(path).bytes())]).toEqual([137, 80, 78, 71])
})

test("an upload never replaces a file: a name taken that day gets a number", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const served = await serve()
  const { session } = served.run

  const first = (await (await upload(served, session, "image.png", "one")).json()) as { path: string }
  const second = (await (await upload(served, session, "image.png", "two")).json()) as { path: string; name: string }
  const third = (await (await upload(served, session, "image.png", "three")).json()) as { path: string }

  expect(second.path).toBe(join(first.path, "..", "image-2.png"))
  expect(second.name).toBe("image.png")
  expect(third.path).toBe(join(first.path, "..", "image-3.png"))
  expect(await readFile(first.path, "utf8")).toBe("one")
  expect(await readFile(second.path, "utf8")).toBe("two")
})

test("an upload is refused when its name would leave the attachments folder, or it is larger than the limit, and nothing is kept", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const served = await serve()
  const { session, projectFolder } = served.run

  for (const name of ["../evie.json", "notes/plan.txt", "..\\plan.txt", "..", ".", "", "plan\u0000.txt"]) {
    const refused = await upload(served, session, name, "x")
    expect(refused.status).toBe(400)
    expect(await refused.text()).toBe(`${JSON.stringify(name)} is not a plain file name. A file is attached by its name alone, such as "plan.pdf".`)
  }
  const large = await upload(served, session, "huge.bin", new Uint8Array(ATTACHMENT_LIMIT + 1))
  expect(large.status).toBe(413)
  expect(await large.text()).toBe("huge.bin is larger than the 10 MB a file attached can be.")
  expect(await Bun.file(join(projectFolder, ".evie", "evie.json")).exists()).toBe(false)
  expect(await Bun.file(join(projectFolder, ".evie", "attachments", "2026-11-22", "huge.bin")).exists()).toBe(false)
  expect((await upload(served, "no-such-session", "plan.txt", "x")).status).toBe(400)
})

test("an image attached to a message is given back to a signed-in client to show, and nothing else is", async () => {
  const served = await serve()
  const { session } = served.run
  const photo = (await (await upload(served, session, "photo.png", PICTURE, { type: "image/png" })).json()) as Attachment
  const page = (await (await upload(served, session, "page.html", "<script>alert(1)</script>", { type: "text/html" })).json()) as Attachment
  const unsent = (await (await upload(served, session, "later.png", PICTURE, { type: "image/png" })).json()) as Attachment
  await served.run.evie.send({ type: "send_message", session, text: "", attachments: [photo, page] })
  await Bun.sleep(0)
  const fetchBack = (path: string, token: string | null = served.token) =>
    fetch(`${served.server.url}/attachments?${new URLSearchParams({ session, path })}`, { headers: token === null ? {} : { Authorization: `Bearer ${token}` } })

  const shown = await fetchBack(photo.path)
  expect(shown.status).toBe(200)
  expect(shown.headers.get("content-type")).toBe("image/png")
  expect(shown.headers.get("x-content-type-options")).toBe("nosniff")
  expect([...new Uint8Array(await shown.arrayBuffer())]).toEqual([...PICTURE])

  expect((await fetchBack(photo.path, null)).status).toBe(401)
  // Only an image is given back, so that a page attached cannot run as evie's own.
  expect((await fetchBack(page.path)).status).toBe(404)
  // A file kept but not on any message, and one never kept, are not either.
  expect((await fetchBack(unsent.path)).status).toBe(404)
  expect((await fetchBack(join(served.run.home, "evie.db"))).status).toBe(404)
})
