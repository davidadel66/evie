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
