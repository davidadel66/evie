import { createCodePlugin, type HighlightResult, type ThemeInput } from "@streamdown/code";

// A deliberately small syntax palette: teal identifies names/functions,
// amber marks language structure, green is data, and gray recedes comments.
// Keeping punctuation and variables near the body color avoids rainbow code.
export const evieCodeTheme: ThemeInput = {
  name: "evie-dark",
  type: "dark",
  colors: {
    "editor.background": "#0b0e10",
    "editor.foreground": "#c9cfcb",
  },
  tokenColors: [
    {
      scope: ["comment", "punctuation.definition.comment"],
      settings: { foreground: "#8b9491", fontStyle: "italic" },
    },
    {
      scope: ["keyword", "storage.type", "storage.modifier"],
      settings: { foreground: "#d9a04a" },
    },
    {
      scope: ["string", "string.quoted", "string.regexp"],
      settings: { foreground: "#a8d4b5" },
    },
    {
      scope: ["constant.numeric", "constant.language", "constant.character"],
      settings: { foreground: "#e8c98a" },
    },
    {
      scope: [
        "entity.name.function",
        "entity.name.type",
        "entity.name.class",
        "support.function",
        "support.type",
        "support.class",
      ],
      settings: { foreground: "#6fd0be" },
    },
    {
      scope: ["variable", "variable.parameter", "punctuation", "keyword.operator"],
      settings: { foreground: "#c9cfcb" },
    },
  ],
};

export const code = createCodePlugin({ themes: [evieCodeTheme, evieCodeTheme] });

// File inspection follows the editor palette in David's reference. Keep it
// separate from fenced code in assistant messages.
export const fileCodeTheme: ThemeInput = {
  name: "evie-file-dark",
  type: "dark",
  colors: { "editor.background": "#181818", "editor.foreground": "#d4d4d4" },
  tokenColors: [
    { scope: ["comment", "punctuation.definition.comment"], settings: { foreground: "#8b8b8b" } },
    { scope: ["keyword", "storage.type", "storage.modifier"], settings: { foreground: "#f47076" } },
    { scope: ["string", "string.regexp", "entity.name.import", "punctuation.definition.string"], settings: { foreground: "#85d77a" } },
    { scope: ["entity.name", "support.type", "support.class", "support.function", "storage.type.numeric", "storage.type.string", "storage.type.boolean", "storage.type.error", "storage.type.rune"], settings: { foreground: "#c586ff" } },
    { scope: ["variable", "variable.parameter"], settings: { foreground: "#ee994d" } },
    { scope: ["constant.numeric", "constant.language", "keyword.operator"], settings: { foreground: "#79c9ee" } },
    { scope: ["punctuation"], settings: { foreground: "#a0a0a0" } },
  ],
};

// The dependency's cache samples source text. Equal-length interior edits can
// collide, so file evidence must validate token contents before displaying it.
export function exactHighlightTokens(result: HighlightResult | undefined, source: string) {
  if (result?.tokens.map((line) => line.map((token) => token.content).join("")).join("\n") !== source) return undefined;
  return result.tokens;
}
