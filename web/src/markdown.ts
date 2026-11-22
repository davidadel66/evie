import { decodeHTMLStrict } from "entities/decode"
import { Lexer, type Token, type Tokens } from "marked"
import { Fragment, createElement as h, type ReactNode } from "react"

// evie's replies, drawn from Markdown. Only marked's lexer is used, which
// reads the text into pieces such as headings, lists and links; each piece
// is then made into an element here, so nothing the model wrote reaches the
// page as HTML. HTML in a reply is shown as the text it is.

export function renderMarkdown(text: string): ReactNode[] {
  return blocks(new Lexer({ gfm: true }).lex(text))
}

// A reply still streaming in, cut into its blocks, so that each can be
// drawn on its own: the blocks before the last are finished and drawn once,
// and only the last, still open, is read again as each piece comes. Read
// whole on every piece, a reply costs more with each piece, by the square
// of its length. Each cut is at a blank line that ends every open block, so
// that each block is drawn as it would be in the whole reply: not inside a
// fenced code block, and not before an indented line or a list's next item,
// which can still belong to the block above. A reply in which one part
// changes how another is read, such as a link defined elsewhere, is left in
// one piece. Adapted from T3 Code's splitMarkdownBlocks (MIT licence, notice
// in LICENSES/t3code-MIT.txt).
//
// `before` is what the reply was cut into at its last piece. While the
// reply still starts with all of it, the cuts before its last two blocks
// stand, and only those two are looked at again, line by line. The last cut
// is looked at again because the line after it may still be coming in: "1"
// starts a block of its own, but "1." may be the next item of a list above.
// What is left that grows with the reply is the check that it still starts
// with `before`: one comparison of the two texts, which the browser makes
// in one go, at a small part of the cost of reading them.
export function markdownBlocks(text: string, before: readonly string[] = []): string[] {
  // One comparison with the text before, in one piece, costs far less than
  // one for each block.
  const found = before.length > 2 && text.startsWith(before.join("")) ? before.slice(0, -2) : []
  let start = found.reduce((length, block) => length + block.length, 0) // where the open block starts
  // marked reads "\r" as the end of a line, as this does not.
  if (text.includes("\r", start)) return [text]
  let fence = "" // the marks of the fenced code block the line is in, if any
  let indent = 0 // how far in those marks are
  let blank = false // whether a blank line came just before
  for (let at = start; at <= text.length; ) {
    let end = text.indexOf("\n", at)
    if (end === -1) end = text.length
    const line = text.slice(at, end)
    if (fence !== "") {
      // A line left of the marks may end the list item the code is in,
      // which is not followed here.
      if (/[^ \t]/.test(line.slice(0, indent))) return [text]
      if (FENCE_END.exec(line)?.[1]!.startsWith(fence)) fence = ""
    } else if (BLANK.test(line)) {
      blank = true
    } else {
      if (READ_WHOLE.test(line)) return [text]
      if (blank && at > start && !GOES_ON.test(line)) {
        found.push(text.slice(start, at))
        start = at
      }
      blank = false
      const opens = FENCE.exec(line)?.[1]
      if (opens !== undefined) [fence, indent] = [opens, line.indexOf(opens)]
    }
    at = end + 1
  }
  found.push(text.slice(start))
  return found
}

// A fenced code block's first line: three backticks or more, with none after
// them on the line, or three tildes or more.
const FENCE = /^ {0,3}(`{3,}(?![^`\n]*`)|~{3,})/
// Its last line, as marked reads it: as many of the same marks or more, and
// only spaces after them.
const FENCE_END = /^ {0,3}(`{3,}|~{3,}) *$/
// A blank line, as marked reads one between blocks: only spaces and tabs.
const BLANK = /^[ \t]*$/
// A line after a blank line that can still belong to the block above: one
// indented, or a list's next item.
const GOES_ON = /^(?:[ \t]|(?:[-*+]|\d{1,9}[.)])(?:[ \t]|$))/
// A line that changes how other blocks are read: a link's definition, which
// a link anywhere can use, or the last line of one whose name runs over more
// than one line; HTML at the start of a line, which can run on past blank
// lines; a link's tag, after which marked links no more addresses; or a line
// of other spaces only, such as a no-break space, which marked reads as
// blank inside a list and as words outside one.
const READ_WHOLE = /^[ \t>*+\-\d.)]*\[[^\]\n]+\]:|^[^[\n]*\]:|^ {0,3}<[A-Za-z!?/]|<a |^\s+$/i

const HEADINGS = ["h1", "h2", "h3", "h4", "h5", "h6"] as const
// Each heading's text style, at T3 Code's 20, 18, 16 and 14px.
const HEADING_SIZES = ["text-page-title", "text-heading", "text-title"] as const
const ALIGN = { left: "text-left", center: "text-center", right: "text-right" }

function blocks(tokens: Token[]): ReactNode[] {
  return tokens.flatMap((token, key): ReactNode[] => {
    const piece = token as Tokens.Generic
    switch (token.type) {
      case "space":
        return []
      case "heading": {
        const heading = token as Tokens.Heading
        return [h(HEADINGS[heading.depth - 1] ?? "h6", { key, className: HEADING_SIZES[heading.depth - 1] ?? "text-label" }, inline(heading.tokens))]
      }
      case "paragraph":
        return [h("p", { key }, inline((token as Tokens.Paragraph).tokens))]
      // The words of a list item that holds nothing else.
      case "text": {
        const text = token as Tokens.Text
        return [h(Fragment, { key }, inline(text.tokens ?? [text]))]
      }
      case "code":
        return [h("pre", { key, className: "overflow-x-auto border-l border-line pl-3 font-mono text-code" }, h("code", null, (token as Tokens.Code).text))]
      case "blockquote":
        return [h("blockquote", { key, className: "border-l border-line pl-3 text-text-muted" }, blocks((token as Tokens.Blockquote).tokens))]
      case "hr":
        return [h("hr", { key, className: "border-t border-line" })]
      case "list": {
        const list = token as Tokens.List
        const items = list.items.map((item, at) => h("li", { key: at, className: "pl-1 [&+li]:mt-1 [&>*+*]:mt-2" }, blocks(item.tokens)))
        if (!list.ordered) return [h("ul", { key, className: "list-disc pl-5" }, items)]
        return [h("ol", { key, className: "list-decimal pl-5", ...(list.start !== "" && list.start !== 1 && { start: list.start }) }, items)]
      }
      case "checkbox":
        return [checkbox(token as Tokens.Checkbox, key)]
      case "table": {
        const table = token as Tokens.Table
        const cell = (tag: "th" | "td", cell: Tokens.TableCell, at: number) =>
          h(tag, { key: at, className: `border-b border-line px-3 py-2 ${tag === "th" ? "font-semibold" : ""} ${ALIGN[cell.align ?? "left"]}` }, inline(cell.tokens))
        return [
          h(
            "div",
            { key, className: "overflow-x-auto" },
            h(
              "table",
              { className: "border-collapse" },
              h("thead", null, h("tr", null, table.header.map((each, at) => cell("th", each, at)))),
              h("tbody", null, table.rows.map((row, at) => h("tr", { key: at }, row.map((each, column) => cell("td", each, column))))),
            ),
          ),
        ]
      }
      // HTML the model wrote, shown as it wrote it.
      case "html":
        return [h("p", { key, className: "whitespace-pre-wrap" }, (token as Tokens.HTML).text.trimEnd())]
      // Anything else, such as a link's definition, is shown as written.
      default:
        return [h("p", { key }, piece.raw.trimEnd())]
    }
  })
}

function inline(tokens: Token[] | undefined): ReactNode[] {
  return (tokens ?? []).map((token, key): ReactNode => {
    switch (token.type) {
      case "text": {
        const text = token as Tokens.Text
        return text.tokens ? h(Fragment, { key }, inline(text.tokens)) : words(text.text)
      }
      case "escape":
        return (token as Tokens.Escape).text
      case "strong":
        return h("strong", { key, className: "font-semibold" }, inline((token as Tokens.Strong).tokens))
      case "em":
        return h("em", { key, className: "italic" }, inline((token as Tokens.Em).tokens))
      case "del":
        return h("del", { key, className: "line-through" }, inline((token as Tokens.Del).tokens))
      case "codespan":
        return h("code", { key, className: "font-mono text-code" }, (token as Tokens.Codespan).text)
      case "link": {
        const link = token as Tokens.Link
        return linkTo(link.href, inline(link.tokens), key)
      }
      // An image is linked to by its description, so that a reply never
      // has the page load anything from another site.
      case "image": {
        const image = token as Tokens.Image
        return linkTo(image.href, words(image.text), key)
      }
      case "br":
        return h("br", { key })
      // A task list's box, which in a loose list starts the item's paragraph.
      case "checkbox":
        return checkbox(token as Tokens.Checkbox, key)
      case "html":
        return (token as Tokens.Tag).text
      default:
        return (token as Tokens.Generic).raw
    }
  })
}

// A task list's box, ticked or not, which cannot be changed here.
const checkbox = (token: Tokens.Checkbox, key: number) => h("input", { key, type: "checkbox", checked: token.checked, disabled: true, className: "mr-2 align-middle" })

// A link that opens in a new tab, only to a web or mail address. Any other
// address, such as a script's or a path in the project, which would open
// nothing useful here, leaves the words as they are.
const SAFE = ["http:", "https:", "mailto:"]
function linkTo(href: string, words: ReactNode, key: number): ReactNode {
  let safe = false
  try {
    safe = SAFE.includes(new URL(href).protocol)
  } catch {}
  if (!safe) return h(Fragment, { key }, words)
  return h("a", { key, href, target: "_blank", rel: "noreferrer", className: "underline decoration-text-muted underline-offset-2 hover:decoration-text" }, words)
}

// Text as the lexer gives it, with each named character, such as &amp; or
// &mdash;, made back into the one it names, as a browser would, but as
// text: what it makes is never read as HTML. The lexer makes numbered ones,
// such as &#65;, back itself, and leaves code as it was written.
const words = (text: string) => decodeHTMLStrict(text)
