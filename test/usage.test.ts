import { Database } from "bun:sqlite"
import { afterEach, expect, test } from "bun:test"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createOpenRouterProvider, type Chunk } from "../src/index.ts"
import { readUsage } from "../src/core/usage.ts"
import { DONE, data, fakeOpenRouter, sse } from "./support/fake-openrouter.ts"
import { say, startEvie } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

// What a workspace has used, summed from its sessions' logs: the cost the
// OpenRouter plugin reads off each reply, and the command that adds them up.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

const DAY = 24 * 60 * 60 * 1000

test("the OpenRouter plugin keeps the cost a reply was charged beside its tokens, and leaves it out when the reply has none", async () => {
  const reply = (extra: object) =>
    data({ choices: [{ index: 0, delta: { content: "ok" }, finish_reason: "stop" }], usage: { prompt_tokens: 12, completion_tokens: 5, ...extra } })
  const finished = async (usage: object) => {
    const fake = fakeOpenRouter([{ status: 200, body: sse(reply(usage), DONE) }])
    const provider = createOpenRouterProvider({ env: { OPENROUTER_API_KEY: "key" }, fetch: fake.fetch })
    const chunks: Chunk[] = []
    for await (const chunk of provider.stream({ model: "some/model", messages: [{ role: "user", text: "hi" }], tools: [] }, new AbortController().signal)) chunks.push(chunk)
    return chunks.at(-1)
  }

  expect(await finished({ cost: 0.0042 })).toMatchObject({ usage: { inputTokens: 12, outputTokens: 5, cost: 0.0042 } })
  expect(await finished({})).toEqual({ type: "finish", reason: "stop", usage: { inputTokens: 12, outputTokens: 5 } })
  // A cost that makes no sense is not kept.
  expect(await finished({ cost: -1 })).toEqual({ type: "finish", reason: "stop", usage: { inputTokens: 12, outputTokens: 5 } })
})

async function start() {
  const model = scriptedProvider([
    { text: ["A"], usage: { inputTokens: 100, outputTokens: 10, cost: 0.5 } },
    { text: ["B"], usage: { inputTokens: 200, outputTokens: 20 } },
  ])
  const run = await startEvie([model.plugin])
  cleanups.push(run.stop)
  return run
}

test("read_usage adds up a workspace's replies, and says how many have no cost", async () => {
  const run = await start()
  await say(run, "one")
  await say(run, "two")

  const usage = await run.evie.send({ type: "read_usage", project: run.project!.id })

  expect(usage.all).toEqual({ sessions: 1, replies: 2, inputTokens: 300, outputTokens: 30, cachedTokens: 0, cost: 0.5, unpriced: 1 })
  expect(usage.today).toEqual(usage.all)
})

test("a workspace with no replies has nothing used, and another workspace's replies are not counted", async () => {
  const run = await start()
  await say(run, "one")
  const folder = await mkdtemp(join(tmpdir(), "evie-other-"))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  const other = await run.evie.send({ type: "open_session", folder })
  expect(other.project!.id).not.toBe(run.project!.id)

  expect((await run.evie.send({ type: "read_usage", project: run.project!.id })).all.replies).toBe(1)
  expect((await run.evie.send({ type: "read_usage", project: other.project!.id })).all).toEqual({
    sessions: 0, replies: 0, inputTokens: 0, outputTokens: 0, cachedTokens: 0, cost: 0, unpriced: 0,
  })
  await expect(run.evie.send({ type: "read_usage", project: "no-such" })).rejects.toThrow("There is no project")
})

test("replies are put in today, the last 7 days, the last 30 days and all time by when they were made", () => {
  const db = new Database(":memory:")
  db.run("CREATE TABLE session (id TEXT PRIMARY KEY, project TEXT, created INTEGER)")
  db.run("CREATE TABLE event (session TEXT, seq INTEGER, type TEXT, body TEXT, time INTEGER)")
  db.run("INSERT INTO session VALUES ('s1', 'p1', 0), ('s2', 'p1', 0), ('s3', 'p2', 0)")
  const now = new Date(2026, 10, 28, 15, 0).getTime()
  const reply = (session: string, seq: number, ago: number, cost: number | undefined) =>
    db.run("INSERT INTO event VALUES (?, ?, 'assistant_message', ?, ?)", [session, seq, JSON.stringify({ text: "x", usage: { inputTokens: 10, outputTokens: 1, ...(cost !== undefined && { cost }) } }), now - ago])
  reply("s1", 1, 60 * 1000, 1)
  reply("s1", 2, 2 * DAY, 2)
  reply("s2", 1, 20 * DAY, 4)
  reply("s2", 2, 90 * DAY, undefined)
  reply("s3", 1, 60 * 1000, 100)
  // A reply with no usage at all, and an event of another kind, are not counted.
  db.run("INSERT INTO event VALUES ('s1', 3, 'assistant_message', '{\"text\":\"x\"}', ?)", [now])
  db.run("INSERT INTO event VALUES ('s1', 4, 'user_message', '{\"text\":\"x\"}', ?)", [now])

  const usage = readUsage(db, "p1", now)

  expect([usage.today.replies, usage.today.cost]).toEqual([1, 1])
  expect([usage.week.replies, usage.week.cost, usage.week.sessions]).toEqual([2, 3, 1])
  expect([usage.month.replies, usage.month.cost, usage.month.sessions]).toEqual([3, 7, 2])
  expect([usage.all.replies, usage.all.cost, usage.all.unpriced]).toEqual([4, 7, 1])
})
