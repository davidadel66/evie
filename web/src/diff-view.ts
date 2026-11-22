import type { CodeViewDiffItem, FileDiffMetadata } from "@pierre/diffs"
import type { CodeViewReactOptions } from "@pierre/diffs/react"
import { processFile, registerCustomTheme, trimPatchContext } from "@pierre/diffs"
import type { FileDiff } from "../../src/core/types.ts"
import { sidesOf } from "./diff.ts"

// The one module that knows T3 Code's own diff library, @pierre/diffs
// (Apache-2.0), with which evie draws a diff as T3 Code draws it, so that
// the library can be swapped by changing this module alone: DiffView.tsx,
// the diff's screen, takes the library's viewer from here. The page loads
// both only when the right panel first shows a diff, since the library and
// the syntax colours it brings, Shiki's, are most of the page's weight.

export { CodeView, type CodeViewHandle } from "@pierre/diffs/react"

// How many unchanged lines show around each change, as T3 Code's diff
// shows them; the rest are folded, counted, and opened when asked.
const AROUND = 3

// A file's diff as the library draws it, from the diff the server sends,
// which carries the whole file around its changes: cut back to the lines
// around each change, and given the file before and after, so that the
// lines folded between can be opened. Undefined for a file with no diff.
export function diffOf(file: FileDiff): FileDiffMetadata | undefined {
  if (file.patch === undefined || file.patch === "") return undefined
  const sides = sidesOf(file.patch)
  const named = `--- ${file.path}\n+++ ${file.path}\n${trimPatchContext(file.patch, AROUND)}`
  return processFile(named, {
    ...(sides !== undefined && { oldFile: { name: file.path, contents: sides.before }, newFile: { name: file.path, contents: sides.after } }),
  })
}

// The syntax colours, as the owner's drawing has them: tints of evie's
// project colours, named by the tokens in tokens.css, so they follow its
// themes, given to the library as a TextMate theme for Shiki.
const THEME = "evie"
const colour = (token: string) => `var(--${token})`
const scopes = (foreground: string, scope: string[], fontStyle?: string) => ({ scope, settings: { foreground, ...(fontStyle !== undefined && { fontStyle }) } })
registerCustomTheme(
  THEME,
  () =>
    Promise.resolve({
      name: THEME,
      type: "dark" as const,
      colors: { "editor.foreground": colour("text"), "editor.background": colour("bg") },
      tokenColors: [
        { settings: { foreground: colour("text"), background: colour("bg") } },
        scopes(colour("text-muted"), ["comment", "punctuation.definition.comment", "punctuation", "meta.brace", "punctuation.definition.link", "punctuation.definition.metadata", "punctuation.definition.string"]),
        scopes(colour("syntax-mustard"), ["markup.heading", "entity.name.section", "punctuation.definition.heading", "entity.name.function", "support.function", "entity.other.attribute-name"], "bold"),
        scopes(colour("syntax-mustard"), ["entity.name.function", "support.function", "entity.other.attribute-name", "meta.function-call"]),
        scopes(colour("syntax-rust"), ["keyword", "storage", "storage.type", "storage.modifier", "punctuation.definition.list", "markup.list.unnumbered punctuation", "beginning.punctuation.definition.list", "entity.name.tag", "keyword.operator.new"]),
        scopes(colour("syntax-slate"), ["string.other.link", "markup.link", "entity.name.type", "entity.name.class", "support.type", "support.class", "entity.other.inherited-class"]),
        scopes(colour("syntax-moss"), ["string", "markup.underline.link", "string.quoted"]),
        scopes(colour("syntax-plum"), ["markup.inline.raw", "markup.raw", "markup.fenced_code", "constant", "constant.numeric", "constant.language", "constant.character", "variable.other.constant", "support.constant"]),
        scopes(colour("text"), ["variable", "variable.parameter", "meta.paragraph", "markup.list"]),
        { scope: ["markup.bold"], settings: { fontStyle: "bold" } },
        { scope: ["markup.italic"], settings: { fontStyle: "italic" } },
      ],
    }),
  "textmate",
)

// The library's own drawing, in its shadow root, set to evie's tokens and
// to the sizes of T3 Code's diff, as the owner's drawing has them: code in
// Geist Mono, 13px on a 20px line; a 32px header over each file, in 12px
// Geist, with its counts in 11px mono, deletions first; a 4px bar on each
// line changed, dashed for one removed, its line number's column tinted
// more strongly than the line; 24px rows that say how many unchanged lines
// are folded, in 11px Geist; and no line between files. Adapted from T3
// Code's StyledDiffCodeView and diffRendering (MIT licence, notice in
// LICENSES/t3code-MIT.txt).
const LOOK = `
:host, [data-diffs-header], [data-diff], [data-file], [data-error-wrapper], [data-virtualizer-buffer] {
  --diffs-font-family: "Geist Mono", ui-monospace, monospace !important;
  --diffs-header-font-family: "Geist", system-ui, sans-serif !important;
  --diffs-font-size: 13px;
  --diffs-line-height: 20px;
  --diffs-bg: var(--bg) !important;
  --diffs-dark-bg: var(--bg) !important;
  --diffs-light-bg: var(--bg) !important;
  --diffs-dark: var(--text);
  --diffs-token-dark-bg: transparent;
  --diffs-addition-color-override: var(--diff-added);
  --diffs-deletion-color-override: var(--diff-removed);
  --diffs-modified-color-override: var(--diff-changed);
  --diffs-fg-number-override: var(--text-muted);
  --diffs-fg-number-addition-override: color-mix(in srgb, var(--diff-added) 75%, var(--text));
  --diffs-fg-number-deletion-override: color-mix(in srgb, var(--diff-removed) 75%, var(--text));
  --diffs-bg-context-override: var(--bg);
  --diffs-bg-hover-override: color-mix(in srgb, var(--bg) 94%, var(--text));
  --diffs-bg-separator-override: color-mix(in srgb, var(--bg) 96%, var(--text));
  --diffs-bg-buffer-override: color-mix(in srgb, var(--bg) 90%, var(--text));
  --diffs-min-number-column-width: 3ch;
  background-color: var(--bg) !important;
  color: var(--text) !important;
}
/* A line added or removed lies on 11% of its colour, its number's column on
   18%, and the number is 75% of it, mixed with the text. They are mixed
   here, where the stylesheet's minifier does not reach, since it keeps only
   the first such mix among the tokens. */
:is([data-line], [data-no-newline])[data-line-type="change-addition"] { --diffs-computed-diff-line-bg: color-mix(in srgb, var(--diff-added) 11%, var(--bg)) !important; }
:is([data-line], [data-no-newline])[data-line-type="change-deletion"] { --diffs-computed-diff-line-bg: color-mix(in srgb, var(--diff-removed) 11%, var(--bg)) !important; }
:is([data-column-number], [data-gutter-buffer])[data-line-type="change-addition"] { --diffs-computed-diff-line-bg: color-mix(in srgb, var(--diff-added) 18%, var(--bg)) !important; }
:is([data-column-number], [data-gutter-buffer])[data-line-type="change-deletion"] { --diffs-computed-diff-line-bg: color-mix(in srgb, var(--diff-removed) 18%, var(--bg)) !important; }
[data-indicators="bars"] [data-column-number][data-line-type="change-deletion"]::before {
  background-image: repeating-linear-gradient(to bottom, var(--diff-removed) 0 3px, transparent 3px 5px) !important;
  background-size: auto !important;
}
[data-indicators="bars"] [data-line][data-line-type="change-deletion"]::before { content: "" / "Removed"; }
[data-indicators="bars"] [data-line][data-line-type="change-addition"]::before { content: "" / "Added"; }

[data-diffs-header] {
  position: sticky !important;
  top: 0;
  z-index: 4;
  background-color: var(--bg) !important;
  border-bottom-color: transparent !important;
  align-items: center !important;
  gap: 6px !important;
  font-family: "Geist", system-ui, sans-serif !important;
  font-size: 12px !important;
  line-height: 16px !important;
  height: 32px !important;
  min-height: 32px !important;
  padding-block: 0 !important;
  padding-inline: 8px 12px !important;
  cursor: pointer;
  color: var(--text) !important;
}
[data-diffs-header]:hover { box-shadow: inset 3px 0 color-mix(in srgb, var(--text) 24%, transparent); }
[data-diffs-header] [data-header-content] { align-items: center !important; gap: 6px !important; line-height: 16px !important; }
[data-diffs-header] :is([data-title], [data-prev-name]) { line-height: 16px !important; font-family: "Geist", system-ui, sans-serif !important; }
[data-diffs-header] :is([data-additions-count], [data-deletions-count]) { display: none !important; }
[data-diffs-header] :is([data-change-icon], [data-rename-icon]) { display: block; flex-shrink: 0; width: 16px; height: 16px; }
[data-change-icon="new"] { color: var(--diff-added) !important; }
[data-change-icon="deleted"] { color: var(--diff-removed) !important; }
[data-change-icon="change"], [data-change-icon="rename-pure"], [data-change-icon="rename-changed"] { color: var(--diff-changed) !important; }
[data-file-info] { background-color: var(--bg) !important; border-block-color: transparent !important; color: var(--text) !important; }

:is([data-separator="line-info"], [data-separator="line-info-basic"]) {
  height: 24px !important;
  margin-block: 0 !important;
  --diffs-bg-separator: color-mix(in srgb, var(--bg) 96%, var(--text));
}
:is([data-separator="line-info"], [data-separator="line-info-basic"]) [data-separator-content] {
  color: var(--text-muted) !important;
  font-family: "Geist", system-ui, sans-serif !important;
  font-size: 11px !important;
  line-height: 16px !important;
  text-decoration: none !important;
}
:is([data-separator="line-info"], [data-separator="line-info-basic"]) [data-expand-button] { color: var(--text-muted) !important; }
:is([data-separator="line-info"], [data-separator="line-info-basic"])[data-expand-index] :is([data-separator-content], [data-expand-button]):hover {
  color: var(--text) !important;
  background-color: color-mix(in srgb, var(--bg) 92%, var(--text)) !important;
}
[data-split][data-overflow="wrap"] { --diffs-code-grid: var(--diffs-grid-number-column-width) minmax(0, 1fr); }
[data-diff], [data-file] { transition: opacity 200ms ease-out; }
@starting-style { [data-diff], [data-file] { opacity: 0; } }
@media (prefers-reduced-motion: reduce) { [data-diff], [data-file] { transition: none; } }
`

// The files with a diff as the library's viewer lists them, each by its
// path, folded or not, and made once for each file the server sent.
const drawn = new WeakMap<FileDiff, FileDiffMetadata | null>()
export function viewerItems(files: readonly FileDiff[], folded: ReadonlySet<string>): CodeViewDiffItem<undefined>[] {
  return files.flatMap((file) => {
    if (!drawn.has(file)) drawn.set(file, diffOf(file) ?? null)
    const fileDiff = drawn.get(file)
    if (fileDiff === null || fileDiff === undefined) return []
    const collapsed = folded.has(file.path)
    return [{ id: file.path, type: "diff", fileDiff, collapsed, version: (file.patch?.length ?? 0) * 2 + (collapsed ? 1 : 0) }]
  })
}

// How the viewer draws them, as T3 Code's diff panel: stacked or side by
// side, long lines wrapped or scrolled sideways, a bar on each line changed
// and no highlight within lines, each file's header kept in sight, in
// evie's colours, with the sizes the look above sets.
export function viewerOptions(given: { layout: "stacked" | "split"; wrap: boolean; drawn(): void }): CodeViewReactOptions<undefined, undefined> {
  return {
    theme: THEME,
    themeType: "dark",
    diffStyle: given.layout === "split" ? "split" : "unified",
    diffIndicators: "bars",
    lineDiffType: "none",
    overflow: given.wrap ? "wrap" : "scroll",
    hunkSeparators: "line-info-basic",
    stickyHeaders: true,
    unsafeCSS: LOOK,
    onPostRender: given.drawn,
    itemMetrics: { diffHeaderHeight: 32, hunkSeparatorHeight: 24, spacing: 0, paddingTop: 0, paddingBottom: 8 },
    layout: { paddingTop: 0, paddingBottom: 0, gap: 0 },
  }
}
