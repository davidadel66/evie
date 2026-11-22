import { expect, test } from "bun:test"
import type { SettingsView } from "../src/core/types.ts"
import { addressAsOpened, selectedIn, settingsIn, withSettings } from "../web/src/address.ts"
import { searchSettings, settingItems } from "../web/src/settings-page.ts"
import { workspaceIn } from "../web/src/workspace.ts"

// The settings page's rules, apart from its screen, as T3 Code's settings:
// its own address, beside whatever is open under it, and the search over
// every setting.

test("the settings have an address of their own beside what is open: #settings opens on its first page, #settings=<page> on that page", () => {
  expect(settingsIn("")).toBeUndefined()
  expect(settingsIn("#session=s1")).toBeUndefined()
  expect(settingsIn("#settings")).toEqual({ page: "appearance" })
  expect(settingsIn("#settings=keys")).toEqual({ page: "keys" })
  expect(settingsIn("#session=s1&settings=plugins")).toEqual({ page: "plugins" })
  // A page there is not opens the first.
  expect(settingsIn("#settings=nothing")).toEqual({ page: "appearance" })

  // What is open under them stays open.
  expect(selectedIn("#session=s1&settings=keys")).toEqual({ session: "s1" })
  expect(workspaceIn("#workspace=p1&tab=storage&settings=keys")).toEqual({ workspace: "p1", tab: "storage" })
})

test("opening the settings, or another of their pages, keeps the rest of the address, and closing them takes out only them", () => {
  expect(withSettings("#session=s1", "keys")).toBe("#session=s1&settings=keys")
  expect(withSettings("#session=s1&settings=keys", "projects")).toBe("#session=s1&settings=projects")
  expect(withSettings("", "appearance")).toBe("#settings=appearance")
  expect(withSettings("#session=s1&settings=keys", undefined)).toBe("#session=s1")
  expect(withSettings("#settings=keys", undefined)).toBe("")
})

test("as the page opens, the settings stay open in its address, with the sign-in code taken out", () => {
  expect(addressAsOpened("#session=s1&settings=keys&code=abc")).toBe("/#session=s1&settings=keys")
  expect(addressAsOpened("#settings=keys&code=abc")).toBe("/#settings=keys")
  expect(addressAsOpened("#workspace=p1&tab=storage&settings=plugins")).toBe("/#workspace=p1&tab=storage&settings=plugins")
})

// The settings as a server gives them, with made-up keys, plugins and projects.
const VIEW: SettingsView = {
  keys: [
    { name: "OPENROUTER_API_KEY", about: "Reaches the models, through OpenRouter.", inFile: true, environment: false, pending: false },
    { name: "GITHUB_TOKEN", about: "Opens pull requests.", inFile: false, environment: false, pending: false },
  ],
  keysPrivate: true,
  plugins: [
    { name: "openrouter", listed: true, loaded: true, added: false },
    { name: "/plugins/weather.ts", listed: true, loaded: true, added: true },
  ],
  instructions: "",
  projects: [{ id: "p1", name: "Garden planner", folder: "/code/garden-planner", trusted: true, pending: false }],
  restart: false,
  started: 1,
}
const titles = (query: string) => searchSettings(settingItems(VIEW, true), query).map((item) => `${item.title} (${item.page})`)

test("the search finds every setting, those of the browser and those the server keeps, each with the page it is on; nothing typed finds nothing", () => {
  expect(titles("")).toEqual([])
  expect(titles("theme")).toEqual(["Theme (appearance)"])
  expect(titles("github")).toEqual(["GITHUB_TOKEN (keys)"])
  expect(titles("weather")).toEqual(["/plugins/weather.ts (plugins)"])
  expect(titles("garden")).toEqual(["Garden planner (projects)"])
  expect(titles("agents.md")).toEqual(["Your instructions (instructions)"])
  // Before the server has said, its own settings are not there to find.
  expect(searchSettings(settingItems(undefined, true), "github")).toEqual([])
})

test("a title that is what was typed comes first, then one that starts with it, then one with it in it, then a match in what the setting is about or its page", () => {
  expect(titles("openrouter")).toEqual(["openrouter (plugins)", "OPENROUTER_API_KEY (keys)"])
  expect(titles("noti")).toEqual(["Notifications and sound (notifications)", "Notices in the page (notifications)"])
  expect(titles("page")).toEqual(["Notices in the page (notifications)"])
  // "Keys" names a page, whose settings are found by it.
  expect(titles("keys")).toEqual(["OPENROUTER_API_KEY (keys)", "GITHUB_TOKEN (keys)"])
})

test("every word typed must be found, in any case and without minding accents", () => {
  expect(titles("dusk THÈME")).toEqual(["Theme (appearance)"])
  expect(titles("sound system")).toEqual(["Notifications and sound (notifications)"])
  expect(titles("sound nothing")).toEqual([])
})

test("a plugin file can be added only on the computer evie runs on, so only there is Add a plugin file found", () => {
  expect(searchSettings(settingItems(VIEW, true), "add a plugin").map((item) => item.title)).toEqual(["Add a plugin file"])
  expect(searchSettings(settingItems(VIEW, false), "add a plugin")).toEqual([])
})
