import { afterEach, expect, test } from "bun:test"
import { mkdtemp, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { loadSettings } from "../src/index.ts"

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
