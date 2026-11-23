import { afterEach, expect, setSystemTime, test } from "bun:test"
import { mkdir, mkdtemp, realpath, rm, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { show } from "../src/index.ts"
import { say, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"
import { serveEvie, type Served } from "./support/served.ts"

// The route that gives a Storage file to the page, to draw it: a file in a
// project's .evie folder, or one kept for a session in the data folder,
// only to a signed-in client, and only of a kind evie shows, sent so that
// nothing in it can run as evie's own page.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  setSystemTime()
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function serve(script: ScriptedReply[] = []) {
  const served = await serveEvie([scriptedProvider(script).plugin])
  cleanups.push(served.stop)
  const evie = join(await realpath(served.run.projectFolder), ".evie")
  const put = async (path: string, bytes: Uint8Array | string) => {
    await mkdir(join(path, ".."), { recursive: true })
    await writeFile(path, bytes)
  }
  return { served, evie, put, project: served.run.project!.id }
}

// Asks the route for a file, as the page does, signed in with the device's
// token unless told otherwise.
function fetchFile(served: Served, asked: Record<string, string>, options: { token?: string | null; origin?: string } = {}) {
  const token = options.token === undefined ? served.token : options.token
  return fetch(`${served.server.url}/storage?${new URLSearchParams(asked)}`, {
    headers: { ...(token !== null && { Authorization: `Bearer ${token}` }), ...(options.origin !== undefined && { Origin: options.origin }) },
  })
}

const PNG = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52, 0, 0, 0, 4, 0, 0, 0, 3])
const SEALED = "default-src 'none'; frame-ancestors 'self'; sandbox"

test("a file in a project's Storage is given to a signed-in client, as its kind, and told to run nothing; a client not signed in is refused", async () => {
  const { served, evie, put, project } = await serve()
  await put(join(evie, "shown", "2026-11-22", "chart.png"), PNG)
  await put(join(evie, "files", "guide.md"), "# Guide\n")
  await put(join(evie, "files", "logo.svg"), '<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>')
  await put(join(evie, "files", "card.pdf"), "%PDF-1.7\n%%EOF\n")

  const chart = await fetchFile(served, { project, path: "shown/2026-11-22/chart.png" })
  expect(chart.status).toBe(200)
  expect(Object.fromEntries(["content-type", "x-content-type-options", "content-security-policy", "cache-control"].map((name) => [name, chart.headers.get(name)]))).toEqual({
    "content-type": "image/png",
    "x-content-type-options": "nosniff",
    "content-security-policy": SEALED,
    "cache-control": "private, no-cache",
  })
  expect([...new Uint8Array(await chart.arrayBuffer())]).toEqual([...PNG])
  expect((await fetchFile(served, { project, path: "files/guide.md" })).headers.get("content-type")).toBe("text/markdown; charset=utf-8")
  expect(await (await fetchFile(served, { project, path: "files/guide.md" })).text()).toBe("# Guide\n")
  const logo = await fetchFile(served, { project, path: "files/logo.svg" })
  expect([logo.headers.get("content-type"), logo.headers.get("content-security-policy")]).toEqual(["image/svg+xml", SEALED])
  // A PDF is sealed but for the sandbox, which can keep a browser's own PDF view from drawing it.
  const card = await fetchFile(served, { project, path: "files/card.pdf" })
  expect([card.headers.get("content-type"), card.headers.get("x-content-type-options"), card.headers.get("content-security-policy")]).toEqual(["application/pdf", "nosniff", "default-src 'none'; frame-ancestors 'self'"])

  expect((await fetchFile(served, { project, path: "files/guide.md" }, { token: null })).status).toBe(401)
  expect((await fetchFile(served, { project, path: "files/guide.md" }, { token: "not-a-token" })).status).toBe(401)
  expect((await fetchFile(served, { project, path: "files/guide.md" }, { origin: "https://elsewhere.example" })).status).toBe(403)
})

test("nothing outside the project's .evie folder is given: a path that climbs out, a full path, a link that leads out, its own git repository, and a kind evie does not show", async () => {
  const { served, evie, put, project } = await serve()
  const outside = await realpath(await mkdtemp(join(tmpdir(), "evie-outside-")))
  cleanups.push(() => rm(outside, { recursive: true, force: true }))
  await put(join(outside, "secret.png"), PNG)
  await put(join(served.run.projectFolder, "loose.md"), "# Not in Storage\n")
  await put(join(evie, ".git", "notes.md"), "# Git's own\n")
  await put(join(evie, "files", "notes.txt"), "plain text")
  await put(join(evie, "files", "fake.png"), "<html>not a picture</html>")
  await symlink(join(outside, "secret.png"), join(evie, "away.png"))
  await symlink(join(evie, ".git", "notes.md"), join(evie, "into-git.md"))
  await symlink(outside, join(evie, "elsewhere"))

  for (const path of ["../loose.md", "files/../../loose.md", join(outside, "secret.png"), "away.png", "elsewhere/secret.png", ".git/notes.md", "into-git.md", "files/notes.txt", "files/fake.png", "files/gone.png", "files"]) {
    const refused = await fetchFile(served, { project, path })
    expect([path, refused.status]).toEqual([path, 404])
  }
  expect((await fetchFile(served, { project: "no-such-project", path: "files/guide.md" })).status).toBe(404)
  expect((await fetchFile(served, { path: "files/guide.md" })).status).toBe(400)
})

test("a file kept for a session with no project is given by its full path, only for that session, until the session is deleted", async () => {
  setSystemTime(new Date(2026, 10, 22, 9, 30))
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  const script: ScriptedReply[] = []
  const served = await serveEvie([scriptedProvider(script).plugin, show({ dataFolder: home })], { home })
  cleanups.push(served.stop)
  const made = await realpath(await mkdtemp(join(tmpdir(), "evie-made-")))
  cleanups.push(() => rm(made, { recursive: true, force: true }))
  await writeFile(join(made, "chart.png"), PNG)
  const { session } = await served.run.evie.send({ type: "open_session" })
  const other = (await served.run.evie.send({ type: "open_session" })).session
  script.push({ text: [], calls: [{ name: "show", input: { paths: [join(made, "chart.png")] } }] }, { text: ["Here it is."] })
  await say(served.run, "Show me", session)
  const kept = join(await realpath(served.run.home), "shown", session, "2026-11-22", "chart.png")
  expect(stored(served.run.events).findLast((event) => event.type === "tool_result")?.body.text).toContain(JSON.stringify(kept))

  const given = await fetchFile(served, { session, path: kept })
  expect([given.status, given.headers.get("content-type"), given.headers.get("content-security-policy")]).toEqual([200, "image/png", SEALED])
  expect((await fetchFile(served, { session: other, path: kept })).status).toBe(404)
  expect((await fetchFile(served, { session, path: join(made, "chart.png") })).status).toBe(404)
  expect((await fetchFile(served, { session, path: join(served.run.home, "evie.db") })).status).toBe(404)
  expect((await fetchFile(served, { session, path: kept }, { token: null })).status).toBe(401)

  await served.run.evie.send({ type: "delete_session", session })
  expect((await fetchFile(served, { session, path: kept })).status).toBe(404)
})

test("a preview pass serves an HTML mockup and its relative companion files without a cookie, sandboxed to its own folder, and stops working if its device token is taken back", async () => {
  const { served, evie, put, project } = await serve()
  await put(join(evie, "shown", "2026-11-22", "card", "card.html"), '<!doctype html><link rel="stylesheet" href="styles/main.css"><script src="app.js"></script>')
  await put(join(evie, "shown", "2026-11-22", "card", "styles", "main.css"), "body { color: #e8e4dc; }")
  await put(join(evie, "shown", "2026-11-22", "card", "app.js"), "console.log('hi')")
  await put(join(evie, "shown", "2026-11-22", "other.txt"), "sibling outside bundle")

  // Minting requires sign-in.
  const denied = await fetch(`${served.server.url}/preview?${new URLSearchParams({ project, path: "shown/2026-11-22/card/card.html" })}`, { method: "POST" })
  expect(denied.status).toBe(401)

  const minted = await fetch(`${served.server.url}/preview?${new URLSearchParams({ project, path: "shown/2026-11-22/card/card.html" })}`, {
    method: "POST",
    headers: { Authorization: `Bearer ${served.token}` },
  })
  expect(minted.status).toBe(200)
  const { url } = (await minted.json()) as { url: string }
  expect(url).toMatch(/^\/preview\/[A-Za-z0-9_-]+\/card\.html$/)

  // Entry HTML loads with NO token/cookie, with strict sandbox + pass-scoped CSP.
  const html = await fetch(`${served.server.url}${url}`)
  expect(html.status).toBe(200)
  expect(html.headers.get("content-type")).toBe("text/html; charset=utf-8")
  const csp = html.headers.get("content-security-policy") ?? ""
  expect(csp).toContain("sandbox allow-scripts allow-forms")
  expect(csp).toContain("frame-ancestors 'self'")
  const base = url.slice(0, url.lastIndexOf("/") + 1)
  expect(csp).toContain(`script-src 'unsafe-inline' 'unsafe-eval' ${base}`)

  // Companion CSS and JS load from the same pass with no cookie.
  const css = await fetch(`${served.server.url}${base}styles/main.css`)
  expect([css.status, css.headers.get("content-type")]).toEqual([200, "text/css; charset=utf-8"])
  expect(await css.text()).toBe("body { color: #e8e4dc; }")
  const js = await fetch(`${served.server.url}${base}app.js`)
  expect([js.status, js.headers.get("content-type")]).toEqual([200, "text/javascript; charset=utf-8"])

  // Cannot climb out of the bundle folder.
  expect((await fetch(`${served.server.url}${base}..%2Fother.txt`)).status).toBe(404)
  expect((await fetch(`${served.server.url}${base}missing.js`)).status).toBe(404)
  // A non-previewable file cannot mint a pass.
  const notPreviewable = await fetch(`${served.server.url}/preview?${new URLSearchParams({ project, path: "shown/2026-11-22/other.txt" })}`, {
    method: "POST",
    headers: { Authorization: `Bearer ${served.token}` },
  })
  expect(notPreviewable.status).toBe(404)
})
