import type { SettingsView } from "../../src/core/types.ts"

// The settings page's rules, apart from its screen, as T3 Code's settings
// are laid out: its pages, in groups down the left; the search over every
// setting; and what each setting with a default goes back to.

// The pages, in the order the column lists them.
export const SETTINGS_PAGES = ["appearance", "notifications", "instructions", "keys", "plugins", "projects"] as const
export type SettingsPage = (typeof SETTINGS_PAGES)[number]

export const PAGE_NAMES: Record<SettingsPage, string> = {
  appearance: "Appearance",
  notifications: "Notifications",
  instructions: "Instructions",
  keys: "Keys",
  plugins: "Plugins",
  projects: "Projects",
}

// The pages in groups, as T3 Code's column has them: the first group, of
// how the page looks and tells the owner things, has no heading of its
// own, and is set apart by space alone.
export const SETTINGS_GROUPS: readonly { heading: string; headingHidden?: true; pages: readonly SettingsPage[] }[] = [
  { heading: "Preferences", headingHidden: true, pages: ["appearance", "notifications"] },
  { heading: "Agent", pages: ["instructions", "keys", "plugins"] },
  { heading: "Code", pages: ["projects"] },
]

// The page an address names, or the first for a name that is none.
export const settingsPageNamed = (name: string): SettingsPage => SETTINGS_PAGES.find((page) => page === name) ?? SETTINGS_PAGES[0]

// ---- The search -------------------------------------------------------------------------------

// A setting as the search finds it: its title, as its row has it, the page
// it is on, and other words it is known by.
export type SettingItem = { id: string; title: string; page: SettingsPage; terms?: readonly string[] }

// Every setting there is: the browser's own, and once the server has said
// what it keeps, each key, plugin and project, in the order of their pages.
export function settingItems(view: SettingsView | undefined): SettingItem[] {
  return [
    { id: "theme", title: "Theme", page: "appearance", terms: ["colours colors dark dusk moss oxblood look"] },
    { id: "notifications", title: "Notifications and sound", page: "notifications", terms: ["system alert finished failed needs you"] },
    { id: "notices", title: "Notices in the page", page: "notifications", terms: ["in-app toast banner"] },
    { id: "instructions", title: "Your instructions", page: "instructions", terms: ["AGENTS.md prompt how evie works"] },
    ...(view?.keys ?? []).map((key): SettingItem => ({ id: `key:${key.name}`, title: key.name, page: "keys", terms: [key.about ?? "", "secret .env"] })),
    ...(view?.plugins ?? []).map((plugin): SettingItem => ({ id: `plugin:${plugin.name}`, title: plugin.name, page: "plugins", terms: ["loaded"] })),
    ...(view === undefined ? [] : [{ id: "add-plugin", title: "Add a plugin file", page: "plugins", terms: ["path load"] } satisfies SettingItem]),
    ...(view?.projects ?? []).map((project): SettingItem => ({ id: `project:${project.id}`, title: project.name, page: "projects", terms: [project.folder, "trusted"] })),
  ]
}

// The settings that have every word typed, in their title, their page's
// name or the other words they are known by, in any case and without
// minding accents: a title that is what was typed first, then one that
// starts with it, then one with it in it, then one with every word in it,
// then one with it all in another of its words, then the rest; within
// each, in the order of their pages. Nothing typed finds nothing. Adapted
// from T3 Code's settingsSearch.ts (MIT licence, notice in
// LICENSES/t3code-MIT.txt).
export function searchSettings(items: readonly SettingItem[], typed: string): SettingItem[] {
  const query = searchText(typed)
  if (query === "") return []
  const words = query.split(" ")
  return items
    .flatMap((item, at) => {
      const title = searchText(item.title)
      const fields = [title, searchText(PAGE_NAMES[item.page]), ...(item.terms ?? []).map(searchText)]
      if (!words.every((word) => fields.some((field) => field.includes(word)))) return []
      const rank =
        title === query
          ? 5
          : title.startsWith(query)
            ? 4
            : title.includes(query)
              ? 3
              : words.every((word) => title.includes(word))
                ? 2
                : fields.some((field) => field.includes(query))
                  ? 1
                  : 0
      return [{ item, at, rank }]
    })
    .sort((a, b) => b.rank - a.rank || a.at - b.at)
    .map(({ item }) => item)
}

// Text as the search compares it: without accents, in lower case, its
// spaces one apiece.
const searchText = (text: string) =>
  text
    .normalize("NFKD")
    .replace(/\p{M}/gu, "")
    .toLowerCase()
    .replace(/\s+/g, " ")
    .trim()
