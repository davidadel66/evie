import { expect, test } from "bun:test"
import { createElement, Fragment } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { markdownBlocks, renderMarkdown } from "../web/src/markdown.ts"

// evie's replies are drawn from Markdown, as elements made one by one and
// never from HTML the model wrote, which is shown as the text it is.

// The page a reply makes, without its classes, which only style it.
const page = (text: string) => renderToStaticMarkup(createElement(Fragment, null, renderMarkdown(text))).replace(/ class="[^"]*"/g, "")

test("headings, paragraphs, emphasis, inline code and links are drawn as such", () => {
  expect(page("# Plan\n\nRead **the README** and _then_ run `bun test`, ~~not npm~~.\n\n### Then\n\nSee [the docs](https://bun.sh/docs) or <https://bun.sh>.")).toBe(
    "<h1>Plan</h1>" +
      "<p>Read <strong>the README</strong> and <em>then</em> run <code>bun test</code>, <del>not npm</del>.</p>" +
      "<h3>Then</h3>" +
      '<p>See <a href="https://bun.sh/docs" target="_blank" rel="noreferrer">the docs</a> or <a href="https://bun.sh" target="_blank" rel="noreferrer">https://bun.sh</a>.</p>',
  )
})

test("lists, nested and numbered from where they start, code blocks, quotes, rules and tables are drawn as such", () => {
  expect(page("- one\n  - inside\n- two\n\n3. three\n4. four")).toBe("<ul><li>one<ul><li>inside</li></ul></li><li>two</li></ul>" + '<ol start="3"><li>three</li><li>four</li></ol>')
  expect(page("```ts\nconst less = 1 < 2 && `ok`\n```")).toBe("<pre><code>const less = 1 &lt; 2 &amp;&amp; `ok`</code></pre>")
  expect(page("> Leave it\n\n---")).toBe("<blockquote><p>Leave it</p></blockquote><hr/>")
  expect(page("| File | Lines |\n|---|--:|\n| `a.ts` | 12 |")).toBe(
    "<div><table><thead><tr><th>File</th><th>Lines</th></tr></thead><tbody><tr><td><code>a.ts</code></td><td>12</td></tr></tbody></table></div>",
  )
})

test("a code block still being written, as a reply streams, is drawn as code until it is closed", () => {
  expect(page("Run this:\n\n```sh\nbun test --watch")).toBe("<p>Run this:</p><pre><code>bun test --watch</code></pre>")
})

test("HTML in a reply is shown as the text it is, in a block or among words, and never made into elements", () => {
  expect(page('<div onclick="alert(1)">Hi</div>\n\nA <b>bold</b> <img src=x onerror=alert(1)> word')).toBe(
    "<p>&lt;div onclick=&quot;alert(1)&quot;&gt;Hi&lt;/div&gt;</p><p>A &lt;b&gt;bold&lt;/b&gt; &lt;img src=x onerror=alert(1)&gt; word</p>",
  )
})

test("only a web or mail address is a link; an image is never loaded, only linked to; and named characters are the characters they name", () => {
  expect(page("[run](javascript:alert(1)) [file](src/a.ts) [mail](mailto:me@example.com)")).toBe('<p>run file <a href="mailto:me@example.com" target="_blank" rel="noreferrer">mail</a></p>')
  expect(page("![a chart](https://example.com/c.png) ![bad](javascript:x)")).toBe('<p><a href="https://example.com/c.png" target="_blank" rel="noreferrer">a chart</a> bad</p>')
  expect(page("Tom &amp; Jerry &lt;3")).toBe("<p>Tom &amp; Jerry &lt;3</p>")
})

test("every named character is the character it names, outside code, and one that names nothing stays as written", () => {
  expect(page("&copy; 2026 &mdash; &hellip; &eacute;t&eacute; &rarr; &nbsp;&amp;&lt;b&gt; &notathing; `&copy;`")).toBe(
    "<p>© 2026 — … été →  &amp;&lt;b&gt; &amp;notathing; <code>&amp;copy;</code></p>",
  )
})

test("a task list's boxes are drawn as boxes, in a tight list and in a loose one, whose items are paragraphs", () => {
  expect(page("- [x] water\n- [ ] weed")).toBe('<ul><li><input type="checkbox" disabled="" checked=""/>water</li><li><input type="checkbox" disabled=""/>weed</li></ul>')
  expect(page("- [x] water\n\n- [ ] weed")).toBe('<ul><li><p><input type="checkbox" disabled="" checked=""/>water</p></li><li><p><input type="checkbox" disabled=""/>weed</p></li></ul>')
})

// A reply still streaming in is drawn block by block: each finished block
// once, and only the last, still open, read again as each piece comes.
// Drawn so, it must look as one reading of the whole would draw it.
const inBlocks = (blocks: string[]) => renderToStaticMarkup(createElement(Fragment, null, blocks.flatMap(renderMarkdown))).replace(/ class="[^"]*"/g, "")

const REPLIES = {
  "headings and paragraphs": "# Plan\n\nRead **the README** first.\n\n## Then\n\nRun `bun test`.\n\nA heading\n===\n\nAnother\n---\n\n---\n\nAfter the rule.",
  lists: "- one\n- two\n\nBetween.\n\n3. three\n4. four\n\n* star\n\n+ plus\n\nEnd.",
  "loose lists, whose items are paragraphs": "- one\n\n- two\n\n  still two\n\n- three\n\n1. first\n\n2. second\n\n10. tenth\n\nAfter.",
  "nested lists": "- one\n  - inside\n\n    more inside\n\n  - inside again\n- two\n\n  1. numbered\n\n  2. inside\n\nAfter the list.",
  "task lists": "- [x] water\n\n- [ ] weed\n\nDone.",
  tables: "| File | Lines |\n|---|--:|\n| `a.ts` | 12 |\n| b.ts | 3 |\n\nBetween the tables.\n\n| Step | Before | After |\n| --- | --- | --- |\n| Parse | 12 ms | 3 ms |",
  "code blocks with blank lines in them": "Run this:\n\n```ts\nconst a = 1\n\n\nconst b = 2\n```\n\nThen:\n\n~~~\nx\n\n- not a list\n\n# nor a heading\n~~~\n\n````md\n```\ninside\n\n```\n````\n\n```\nclosed by more\n\n`````\n\nAfter.",
  "a code block in a list item": "- item\n\n  ```sh\n  a\n\n  b\n  ```\n\n- next\n\nAfter.",
  "indented code": "A paragraph.\n\n    code one\n\n    code two\n\nAfter.",
  quotes: "> a\n>\n> b\n\n> c\n\nAfter.",
  "a reply cut short in a code block": "Run:\n\n```sh\nbun test\n\nbun run check",
  "a reply cut short after a blank line": "One.\n\n- a\n\n",
  "lines of other spaces, which marked reads as blank only in a list": "a\n \nb\n\n　\nc\n\n- d\n\n \n\n- e",
  "a link defined elsewhere": "See [the docs][d] and [bun].\n\n[d]: https://bun.sh/docs\n\n[bun]: https://bun.sh",
  "a link defined elsewhere, its name over two lines": "See [a b].\n\n[a\nb]: https://bun.sh",
  "HTML, which runs past blank lines": '<details>\n\n<div onclick="alert(1)">hidden</div>\n\n</details>\n\nA <b>bold</b> word.',
  "a link's tag, after which no address is linked": "A <a href=x> b\n\nhttps://bun.sh",
  "a code block whose closing marks end in a tab, which marked does not close": "```\ncode\n```\t\n\nAfter.",
}

// Each reply streams in a character at a time, each cut made from the one
// before, as the conversation makes it. Drawn by its blocks, it must be as
// a reading of the whole draws it at its end, and at every point before.
test("a reply drawn block by block as it streams is drawn as one reading of the whole draws it, at its end and at every point before", () => {
  for (const [name, text] of Object.entries(REPLIES)) {
    let cut: string[] = []
    for (let at = 1; at <= text.length; at++) {
      const sofar = text.slice(0, at)
      cut = markdownBlocks(sofar, cut)
      if (inBlocks(cut) !== page(sofar)) expect([name, sofar, inBlocks(cut)]).toEqual([name, sofar, page(sofar)])
    }
    expect([name, cut.join(""), inBlocks(cut)]).toEqual([name, text, page(text)])
  }
})

test("a cut made from the one before is the cut made afresh, and a new reply is cut afresh", () => {
  for (const text of Object.values(REPLIES)) {
    let cut: string[] = []
    for (let at = 1; at <= text.length; at++) {
      cut = markdownBlocks(text.slice(0, at), cut)
      if (cut.join("|") !== markdownBlocks(text.slice(0, at)).join("|")) expect(cut).toEqual(markdownBlocks(text.slice(0, at)))
    }
  }
  // "2" after a blank line starts a block of its own, until "2." makes it a list's next item.
  const before = ["A\n\n", "B\n\n1. a\n\n", "2"]
  expect(markdownBlocks("A\n\nB\n\n1. a\n\n2", before.slice(0, -1))).toEqual(before)
  expect(markdownBlocks("A\n\nB\n\n1. a\n\n2.", before)).toEqual(["A\n\n", "B\n\n1. a\n\n2."])
  expect(markdownBlocks("Another\n\nreply", ["One\n\n", "two\n\n", "three"])).toEqual(["Another\n\n", "reply"])
})

test("a streaming reply is cut at blank lines between blocks, never inside a code block or a list", () => {
  expect(markdownBlocks("# A\n\nText\n\n```\na\n\nb\n```\n\nList:\n\n- x\n\n- y\n\n  y again\n\nEnd")).toEqual(["# A\n\n", "Text\n\n", "```\na\n\nb\n```\n\n", "List:\n\n- x\n\n- y\n\n  y again\n\n", "End"])
  // Only the last block is still open, however long the reply.
  const long = Array.from({ length: 200 }, (_, at) => `Paragraph ${at}, of some length.`).join("\n\n")
  expect(markdownBlocks(long).at(-1)).toBe("Paragraph 199, of some length.")
})

test("a reply that one part reads differently elsewhere, such as a link's definition or HTML, is read whole", () => {
  for (const text of [REPLIES["a link defined elsewhere"], REPLIES["a link defined elsewhere, its name over two lines"], REPLIES["HTML, which runs past blank lines"], REPLIES["a link's tag, after which no address is linked"], "One.\r\n\r\nTwo."]) expect(markdownBlocks(text)).toEqual([text])
  // In a code block, such lines are only code.
  expect(markdownBlocks("```html\n<div>\n[a]: b\n```\n\nAfter.")).toEqual(["```html\n<div>\n[a]: b\n```\n\n", "After."])
})

test("HTML among a streaming reply's words is shown as the text it is", () => {
  const text = "First.\n\nA <b>bold</b> <img src=x onerror=alert(1)> word.\n\nLast."
  expect(markdownBlocks(text)).toHaveLength(3)
  expect(inBlocks(markdownBlocks(text))).toBe("<p>First.</p><p>A &lt;b&gt;bold&lt;/b&gt; &lt;img src=x onerror=alert(1)&gt; word.</p><p>Last.</p>")
})
