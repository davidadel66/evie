import { expect, test } from "bun:test"
import { createElement, Fragment } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { renderMarkdown } from "../web/src/markdown.ts"

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
