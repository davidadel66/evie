import type { CodeViewDiffItem, FileDiffContentsLoader, FileDiffMetadata } from "@pierre/diffs"
import { processFile, registerCustomTheme } from "@pierre/diffs"
import type { CodeViewReactOptions } from "@pierre/diffs/react"
import { getOrCreateWorkerPoolSingleton, terminateWorkerPoolSingleton, type WorkerPoolManager } from "@pierre/diffs/worker"
import type { FileDiff } from "../../src/core/types.ts"

// The one module that knows T3 Code's own diff library, @pierre/diffs
// (Apache-2.0), with which evie draws a diff as T3 Code draws it, so that
// the library can be swapped by changing this module alone: DiffView.tsx,
// the diff's screen, takes the library's viewer from here. The page loads
// both only when the right panel first shows a diff, since the library and
// the syntax colours it brings, Shiki's, are most of the page's weight.

export { CodeView, WorkerPoolContext, type CodeViewHandle } from "@pierre/diffs/react"

// A file's diff as the library draws it, from the diff the server sends,
// with three lines around each change: the unchanged lines before and
// between the changes are folded and counted, and opened once the file's
// whole sides are read (sidesLoader). A file added or removed whole, and
// one renamed, are drawn as such. Undefined for a file with no diff.
// The library keeps what it coloured by the diff's key, made from what the
// file holds.
export function diffOf(file: FileDiff): FileDiffMetadata | undefined {
  if (file.patch === undefined || (file.patch === "" && file.from === undefined)) return undefined
  const cacheKey = `${hashOf(`${file.from ?? ""}\0${file.patch}`).toString(36)}:${file.path}`
  const diff = processFile(`--- ${file.from ?? file.path}\n+++ ${file.path}\n${file.patch}`, { cacheKey })
  if (diff === undefined) return undefined
  if (file.status === "added") diff.type = "new"
  else if (file.status === "removed") diff.type = "deleted"
  return diff
}

// What reads a file's whole sides from the server: by its path, and the
// path it had before, for a file renamed.
export type ReadSides = (path: string, from: string | undefined) => Promise<{ before: string | null; after: string | null } | { unavailable: string }>

// What hands the library a file's whole sides, when the owner first opens
// the unchanged lines folded in it, read with `read`. Sides that cannot be
// read say why; sides that no longer fit the diff, as when the file has
// changed since the diff was read, are not drawn, since the library places
// them by the diff's line numbers alone.
export function sidesLoader(read: ReadSides): FileDiffContentsLoader {
  return async (diff) => {
    const from = diff.prevName !== undefined && diff.prevName !== diff.name ? diff.prevName : undefined
    const sides = await read(diff.name, from)
    if ("unavailable" in sides) throw new Error(sides.unavailable)
    const before = sides.before ?? ""
    const after = sides.after ?? ""
    if (!fits(diff, before, after)) throw new Error(`${diff.name} has changed since its diff was read: refresh the diff to see it as it is now.`)
    return { oldFile: { name: from ?? diff.name, contents: before }, newFile: { name: diff.name, contents: after } }
  }
}

// A text's lines, each with its line end, as the library splits a file.
const linesOf = (text: string) => (text === "" ? [] : text.split(/(?<=\n)/))

// Whether a file's sides hold, at each hunk's lines, the lines the diff
// says they do. Adapted from T3 Code's diffFileContents (MIT licence,
// notice in LICENSES/t3code-MIT.txt).
function fits(diff: FileDiffMetadata, before: string, after: string): boolean {
  const [old, now] = [linesOf(before), linesOf(after)]
  const same = (patch: string | undefined, file: string | undefined) => patch !== undefined && file !== undefined && patch.replace(/\r?\n$/, "") === file.replace(/\r?\n$/, "")
  return diff.hunks.every((hunk) => {
    for (let at = 0; at < hunk.deletionCount; at += 1) if (!same(diff.deletionLines[hunk.deletionLineIndex + at], old[hunk.deletionStart - 1 + at])) return false
    for (let at = 0; at < hunk.additionCount; at += 1) if (!same(diff.additionLines[hunk.additionLineIndex + at], now[hunk.additionStart - 1 + at])) return false
    return true
  })
}

// The syntax colours: lighter tints of evie's project colours, named by
// the tokens in tokens.css, so they follow its themes, given to the
// library as a TextMate theme for Shiki. Headings are bold.
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
        scopes(colour("syntax-mustard"), ["markup.heading", "entity.name.section", "punctuation.definition.heading"], "bold"),
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
// to the sizes of T3 Code's diff: code in Geist Mono, 13px on a 20px line;
// a 32px header over each file, which DiffView.tsx draws; a 4px bar on
// each line changed, dashed for one removed, its line number, 12px in a
// 44px column, tinted more strongly than the line; 24px rows that say how
// many unchanged lines are folded, in 11px Geist; and no line between
// files. Adapted from T3 Code's StyledDiffCodeView and diffRendering (MIT
// licence, notice in LICENSES/t3code-MIT.txt).
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
  --diffs-modified-color-override: var(--project-slate);
  --diffs-fg-number-override: var(--text-muted);
  --diffs-fg-number-addition-override: color-mix(in srgb, var(--diff-added) 75%, var(--text));
  --diffs-fg-number-deletion-override: color-mix(in srgb, var(--diff-removed) 75%, var(--text));
  --diffs-bg-context-override: var(--bg);
  --diffs-bg-hover-override: color-mix(in srgb, var(--bg) 94%, var(--text));
  --diffs-bg-separator-override: color-mix(in srgb, var(--bg) 96%, var(--text));
  --diffs-bg-buffer-override: color-mix(in srgb, var(--bg) 90%, var(--text));
  background-color: var(--bg) !important;
  color: var(--text) !important;
}
/* A line number: 12px, at the right of a 44px column, 10px from the code. */
[data-column-number] {
  font-size: 12px;
  box-sizing: border-box;
  min-width: 44px;
  padding-inline: 0 10px !important;
}
[data-column-number] [data-line-number-content] { min-width: 0; }
[data-line] { padding-inline-start: 10px !important; }
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
  height: 32px !important;
  min-height: 32px !important;
  padding: 0 !important;
  color: var(--text) !important;
}
[data-diffs-header]:hover { box-shadow: inset 3px 0 color-mix(in srgb, var(--text) 24%, transparent); }
[data-diffs-header] slot { display: contents; }

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
// path and folded or not. Each file's diff, and the hash of what it holds,
// are made once for each file the server sent; the viewer draws an item
// again only when its version, made from them, changes.
const drawn = new WeakMap<FileDiff, { diff: FileDiffMetadata; hash: number } | null>()
export function viewerItems(files: readonly FileDiff[], folded: ReadonlySet<string>): CodeViewDiffItem<undefined>[] {
  return files.flatMap((file) => {
    if (!drawn.has(file)) {
      const diff = diffOf(file)
      drawn.set(file, diff === undefined ? null : { diff, hash: hashOf(`${file.from ?? ""}\0${file.patch ?? ""}`) })
    }
    const made = drawn.get(file)
    if (made === null || made === undefined) return []
    const collapsed = folded.has(file.path)
    return [{ id: file.path, type: "diff", fileDiff: made.diff, collapsed, version: made.hash * 2 + (collapsed ? 1 : 0) }]
  })
}

// A 32-bit FNV-1a hash of a text, as T3 Code keys its diffs.
function hashOf(text: string): number {
  let hash = 0x811c9dc5
  for (let at = 0; at < text.length; at += 1) hash = Math.imul(hash ^ text.charCodeAt(at), 0x01000193) >>> 0
  return hash
}

// How the viewer draws them, as T3 Code's diff panel: stacked or side by
// side, long lines wrapped or scrolled sideways, a bar on each line changed
// and no highlight within lines, each file's header kept in sight, the
// unchanged lines folded under a flat band across the file, which opens
// once `load` has read the file's sides, in evie's colours, with the sizes
// the look above sets. Past the library's limits, a long line or a long
// file is drawn as plain text.
export function viewerOptions(given: { layout: "stacked" | "split"; wrap: boolean; drawn(): void; load: FileDiffContentsLoader }): CodeViewReactOptions<undefined, undefined> {
  return {
    theme: THEME,
    themeType: "dark",
    diffStyle: given.layout === "split" ? "split" : "unified",
    diffIndicators: "bars",
    lineDiffType: "none",
    overflow: given.wrap ? "wrap" : "scroll",
    hunkSeparators: "line-info-basic",
    stickyHeaders: true,
    tokenizeMaxLineLength: TOKENIZE_LONGEST_LINE,
    unsafeCSS: LOOK,
    onPostRender: given.drawn,
    loadDiffFiles: given.load,
    itemMetrics: { diffHeaderHeight: 32, hunkSeparatorHeight: 24, spacing: 0, paddingTop: 0, paddingBottom: 8 },
    layout: { paddingTop: 0, paddingBottom: 0, gap: 0 },
  }
}
const TOKENIZE_LONGEST_LINE = 1000

// The workers that colour the code, as T3 Code's: a few, by how many cores
// the computer has, so that colouring a large file does not hold up the
// page. They start when the first diff is drawn, and stop half a minute
// after the last diff on the page has gone. Should they fail to start, or
// the page name no worker, the library colours on the page itself.
//
// Each runs the library's own small worker, `diff-worker.ts`, a module the
// server builds beside the page, with which it shares its files: it loads
// a few KB to start, and the rest as it first needs it. The page reads the
// language of each file and the theme, and hands them to the workers, so a
// worker never fetches Shiki's grammars itself. The bundler does not follow
// `new Worker(...)`, so the server names the worker's built file in the
// page's head, in the meta tag `DIFF_WORKER_META`, and the page hands its
// address here as `worker`. A page bundled as it is asked for, by
// `evie serve --dev`, names none.
export const DIFF_WORKER_META = "evie-diff-worker"
const POOL_IDLE = 30_000
let held = 0
let idle: ReturnType<typeof setTimeout> | undefined
// The pool while it is kept, and the workers started for it that it has
// not taken yet.
let kept: { pool: WorkerPoolManager; ahead: Worker[] } | undefined
export function holdPool(worker: string | undefined): { pool: WorkerPoolManager | undefined; release(): void } {
  clearTimeout(idle)
  held += 1
  if (kept === undefined && worker && typeof Worker !== "undefined") kept = poolOf(worker)
  let released = false
  return {
    pool: kept?.pool,
    release() {
      if (released) return
      released = true
      held -= 1
      if (held === 0) idle = setTimeout(letGo, POOL_IDLE)
    },
  }
}

// The pool, with its workers started as it is made. The library starts
// them only once the page has read the theme, which on a phone's network
// is two round trips later, and each then takes two more to fetch the
// files it starts with, one importing the other: started at once, they
// have them by the time the pool asks. A worker that has failed to start
// by then, as when its file is gone, is started again for the pool, which
// then learns of its failure and colours on the page.
function poolOf(address: string): { pool: WorkerPoolManager; ahead: Worker[] } {
  const cores = typeof navigator === "undefined" ? 4 : Math.max(1, navigator.hardwareConcurrency || 4)
  const size = Math.max(2, Math.min(6, Math.floor(cores / 2)))
  const start = () => new Worker(address, { type: "module" })
  const failed = new WeakSet<Worker>()
  const ahead = Array.from({ length: size }, () => {
    const each = start()
    each.addEventListener("error", () => failed.add(each))
    return each
  })
  const taken = () => {
    const each = ahead.shift()
    if (each !== undefined && !failed.has(each)) return each
    each?.terminate()
    return start()
  }
  const pool = getOrCreateWorkerPoolSingleton({
    poolOptions: { workerFactory: taken, poolSize: size, totalASTLRUCacheSize: 240 },
    highlighterOptions: { theme: THEME, preferredHighlighter: "shiki-js", tokenizeMaxLineLength: TOKENIZE_LONGEST_LINE, lineDiffType: "none" },
  })
  return { pool, ahead }
}

// Stops the pool's workers, and any it did not take.
function letGo() {
  terminateWorkerPoolSingleton()
  for (const each of kept?.ahead.splice(0) ?? []) each.terminate()
  kept = undefined
}
