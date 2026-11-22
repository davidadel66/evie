import { expect, test } from "bun:test"
import { compareApart } from "../src/core/diff.ts"

// Comparing a file's lines apart from the server's own thread: each
// comparison has a time limit, and so has a whole answer, past which the
// files left are said to be too slow to compare, so that one answer cannot
// hold the thread for minutes.

const lines = (count: number, change: (at: number) => string) => new TextEncoder().encode(Array.from({ length: count }, (_, at) => change(at)).join("\n"))
const before = lines(20_000, (at) => `line ${at}`)
const after = lines(20_000, (at) => (at % 7 === 0 ? `changed ${at}` : `line ${at}`))

test("a file left once a whole answer's time is up is too slow to compare, and is not compared", async () => {
  const file = await compareApart("a.txt", before, after, { patch: true, until: Date.now() - 1 })
  expect(file).toEqual({ path: "a.txt", status: "changed", added: 0, removed: 0, tooSlow: true })
})

test("a comparison that takes longer than its limit is too slow, and the next is made all the same", async () => {
  expect(await compareApart("a.txt", before, after, { patch: false, within: 1 })).toEqual({ path: "a.txt", status: "changed", added: 0, removed: 0, tooSlow: true })
  const small = await compareApart("b.txt", new TextEncoder().encode("one\ntwo\n"), new TextEncoder().encode("one\n2\n"), { patch: true })
  expect(small).toEqual({ path: "b.txt", status: "changed", added: 1, removed: 1, patch: "@@ -1,2 +1,2 @@\n one\n-two\n+2\n" })
})
