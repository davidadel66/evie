// The three themes the owner can pick, all dark and moody. Dusk is evie's
// own, and the default.
export const THEMES = [
  { id: "dusk", name: "Dusk" },
  { id: "moss", name: "Moss" },
  { id: "oxblood", name: "Oxblood" },
] as const

export type Theme = (typeof THEMES)[number]["id"]

// Where this browser keeps the theme. The page's own script in index.html
// reads it too, to set the theme before the page is first drawn.
const KEY = "evie.theme"

// The theme this browser was last set to, or Dusk. A browser can refuse to
// keep anything, as in a private window, and then the page opens in Dusk.
export function rememberedTheme(): Theme {
  try {
    const kept = localStorage.getItem(KEY)
    return THEMES.find((theme) => theme.id === kept)?.id ?? "dusk"
  } catch {
    return "dusk"
  }
}

// Puts the page in a theme, and remembers it in this browser.
export function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme
  try {
    localStorage.setItem(KEY, theme)
  } catch {
    // Not remembered: the next visit opens in Dusk.
  }
}
