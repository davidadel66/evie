import { afterEach, expect, test } from "bun:test"
import { chmod, lstat, mkdir, mkdtemp, readdir, readFile, rm, stat, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { loadSettings } from "../src/index.ts"
import { replaceFile, withKey, writeKey } from "../src/settings.ts"

let home: string | undefined
afterEach(async () => {
  if (home) await rm(home, { recursive: true, force: true })
  home = undefined
})

async function settingsWith(file: string | undefined, environment: Record<string, string> = {}) {
  home = await mkdtemp(join(tmpdir(), "evie-home-"))
  if (file !== undefined) await writeFile(join(home, ".env"), file)
  return loadSettings({ EVIE_HOME: home, ...environment })
}

test("a setting in the data folder's .env file is used", async () => {
  const settings = await settingsWith("OPENROUTER_API_KEY=from-the-file\nEVIE_MODEL=some/model\n")

  expect(settings.OPENROUTER_API_KEY).toBe("from-the-file")
  expect(settings.EVIE_MODEL).toBe("some/model")
})

test("a setting in the real environment wins over the file", async () => {
  const settings = await settingsWith("EVIE_MODEL=from-the-file\n", { EVIE_MODEL: "from-the-environment" })

  expect(settings.EVIE_MODEL).toBe("from-the-environment")
})

test("Evie starts without a .env file", async () => {
  const settings = await settingsWith(undefined, { EVIE_MODEL: "some/model" })

  expect(settings.EVIE_MODEL).toBe("some/model")
})

test("comments, blank lines, quotes and a leading export are understood", async () => {
  const settings = await settingsWith(
    ['# a comment', "", 'QUOTED="two words"', "SINGLE='x y'", "export EXPORTED=yes", "SPACED = padded ", "not a setting"].join("\n"),
  )

  expect(settings).toMatchObject({ QUOTED: "two words", SINGLE: "x y", EXPORTED: "yes", SPACED: "padded" })
})

test("settings from the file are not put into the process's own environment", async () => {
  await settingsWith("EVIE_TEST_ONLY_IN_FILE=secret\n")

  expect(process.env.EVIE_TEST_ONLY_IN_FILE).toBeUndefined()
})

// ---- Changing a key in the file ------------------------------------------------------------

test("a key set in the file's text replaces its line where it is, drops a second one, and leaves every other line as it was", () => {
  const text = ["# my keys", "export OPENROUTER_API_KEY=old-one", "", "BRAVE_API_KEY='brave'", "OPENROUTER_API_KEY=older-still", "not a setting"].join("\n")

  expect(withKey(text, "OPENROUTER_API_KEY", "made-up-new")).toBe(
    ["# my keys", "export OPENROUTER_API_KEY=made-up-new", "", "BRAVE_API_KEY='brave'", "not a setting"].join("\n"),
  )
})

test("a key not in the file yet goes on a line of its own at the end, and one cleared is taken out", () => {
  expect(withKey("BRAVE_API_KEY=brave", "OPENROUTER_API_KEY", "made-up")).toBe("BRAVE_API_KEY=brave\nOPENROUTER_API_KEY=made-up\n")
  expect(withKey("", "OPENROUTER_API_KEY", "made-up")).toBe("OPENROUTER_API_KEY=made-up\n")
  expect(withKey("# keys\nOPENROUTER_API_KEY=made-up\nBRAVE_API_KEY=brave\n", "OPENROUTER_API_KEY", null)).toBe("# keys\nBRAVE_API_KEY=brave\n")
})

test("a key's line keeps its export, and a file written with CRLF line endings keeps them", () => {
  expect(withKey("export OPENROUTER_API_KEY=old-one\n", "OPENROUTER_API_KEY", "made-up-new")).toBe("export OPENROUTER_API_KEY=made-up-new\n")
  expect(withKey("# keys\r\nBRAVE_API_KEY=brave\r\n", "OPENROUTER_API_KEY", "made-up")).toBe("# keys\r\nBRAVE_API_KEY=brave\r\nOPENROUTER_API_KEY=made-up\r\n")
  expect(withKey("OPENROUTER_API_KEY=old\r\nBRAVE_API_KEY=brave\r\n", "OPENROUTER_API_KEY", "made-up")).toBe("OPENROUTER_API_KEY=made-up\r\nBRAVE_API_KEY=brave\r\n")
  expect(withKey("OPENROUTER_API_KEY=old\r\nBRAVE_API_KEY=brave\r\n", "OPENROUTER_API_KEY", null)).toBe("BRAVE_API_KEY=brave\r\n")
})

// ---- Replacing a settings file -----------------------------------------------------------------

async function folder() {
  const made = await mkdtemp(join(tmpdir(), "evie-files-"))
  const previous = home
  home = made
  if (previous) await rm(previous, { recursive: true, force: true })
  return made
}

test("a key is written through a temporary file of its own name, made new, so one planted at a known name is never written through, and none is left after", async () => {
  const at = await folder()
  const elsewhere = join(at, "elsewhere")
  await writeFile(elsewhere, "untouched")
  await symlink(elsewhere, join(at, ".env.new"))

  await writeKey(at, "OPENROUTER_API_KEY", "made-up")

  expect(await readFile(elsewhere, "utf8")).toBe("untouched")
  expect(await readFile(join(at, ".env"), "utf8")).toBe("OPENROUTER_API_KEY=made-up\n")
  expect((await readdir(at)).sort()).toEqual([".env", ".env.new", "elsewhere"])
})

test("a replacement that fails leaves no temporary file behind", async () => {
  const at = await folder()
  // A folder where the file should be, which a file cannot replace.
  await mkdir(join(at, "AGENTS.md"))
  await writeFile(join(at, "AGENTS.md", "inside"), "")

  await expect(replaceFile(join(at, "AGENTS.md"), "text")).rejects.toThrow()

  expect(await readdir(at)).toEqual(["AGENTS.md"])
})

test("a link to a file not there yet is written where it leads, and a link that leads round in a circle is refused", async () => {
  const at = await folder()
  await mkdir(join(at, "dotfiles"))
  await symlink(join(at, "dotfiles", "evie.env"), join(at, ".env"))

  await writeKey(at, "OPENROUTER_API_KEY", "made-up")

  expect((await lstat(join(at, ".env"))).isSymbolicLink()).toBe(true)
  expect(await readFile(join(at, "dotfiles", "evie.env"), "utf8")).toBe("OPENROUTER_API_KEY=made-up\n")

  await symlink(join(at, "b"), join(at, "a"))
  await symlink(join(at, "a"), join(at, "b"))
  await expect(replaceFile(join(at, "a"), "text")).rejects.toThrow(`${join(at, "a")} is a link that leads round in a circle, so it was not written.`)
})

test("a file replaced keeps the mode it had, and .env is made readable by the owner only", async () => {
  const at = await folder()
  await writeFile(join(at, "AGENTS.md"), "before")
  await chmod(join(at, "AGENTS.md"), 0o640)
  await writeFile(join(at, ".env"), "")
  await chmod(join(at, ".env"), 0o644)

  await replaceFile(join(at, "AGENTS.md"), "after")
  await writeKey(at, "OPENROUTER_API_KEY", "made-up")

  expect((await stat(join(at, "AGENTS.md"))).mode & 0o777).toBe(0o640)
  expect((await stat(join(at, ".env"))).mode & 0o777).toBe(0o600)
})
