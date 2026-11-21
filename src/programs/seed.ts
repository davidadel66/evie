import { mkdir, rm, writeFile } from "node:fs/promises"
import { dirname, join, resolve } from "node:path"
import { openStore } from "../core/store.ts"
import { ask, createEvie, dataFolder, files, jobs, shell, subagents, type Plugin, type Settings } from "../index.ts"
import { scriptedProvider, type ScriptedReply } from "../plugins/scripted.ts"
import { fail, print, reason } from "./common.ts"

// `evie seed`: fills an empty data folder with made-up projects and
// sessions, for trying out a development copy of Evie or a new client
// without the owner's real ones. Each session is made by running real turns
// through the core, with a scripted stand-in for the model, so it needs no
// key and costs nothing. A folder that already has sessions is refused, so
// made-up sessions can never be mixed in with real ones.

// The made-up projects, each a few files made into a git repository. They
// are kept beside the data folder, in a folder of the same name with "-seed"
// added, such as ~/.evie-dev-seed for ~/.evie-dev: the tools are kept out of
// the data folder, so they could not work in a project inside it.
const projectsFolder = (home: string) => `${resolve(home)}-seed`
const PROJECTS: Record<string, Record<string, string>> = {
  "recipe-box": {
    "README.md": "# Recipe box\n\nThe family's recipes, one Markdown file each, and a small site to browse them.\n\nRun the tests with `npm test`.\n",
    "package.json": '{ "name": "recipe-box", "private": true, "scripts": { "test": "bun test" } }\n',
    "recipes/apple-cake.md": "# Apple cake\n\nTime: 1 hour\n\n- 3 apples\n- 200 g flour\n- 150 g sugar\n- 2 eggs\n",
    "recipes/flatbread.md": "# Flatbread\n\n- 250 g flour\n- 150 ml warm water\n- a pinch of salt\n",
    "recipes/lentil-soup.md": "# Lentil soup\n\n- 200 g red lentils\n- 1 onion\n- 1 litre of stock\n",
  },
  "garden-planner": {
    "README.md": "# Garden planner\n\nKeeps track of what is planted in each bed.\n",
    "src/plants.ts":
      "export type Plant = { name: string; bed: string }\n\n" +
      'export const plants: Plant[] = [\n  { name: "Tomato", bed: "south" },\n  { name: "Basil", bed: "south" },\n  { name: "Runner bean", bed: "fence" },\n]\n',
  },
  "weather-station": {
    "README.md": "# Weather station\n\nReads the sensors in the garden every ten minutes and keeps the readings.\n",
    "src/readings.ts":
      "export type Reading = { time: number; celsius: number; humidity: number }\n\n" +
      "export const average = (readings: Reading[]) => readings.reduce((sum, reading) => sum + reading.celsius, 0) / readings.length\n",
    "scripts/slow-tests.sh": '# The tests that talk to the sensors, which take a while.\nsleep 1\necho "12 tests passed"\n',
  },
}
type ProjectName = keyof typeof PROJECTS

// The name the made-up sessions give their model. A development server that
// carries one on uses its own model instead.
const MODEL = "scripted"

// What a made-up session's script does, as a client would.
type Session = {
  // Sends a message and waits for the turn it starts to end. `turns` is
  // more than one when that turn leaves work behind that wakes the session
  // again when it finishes.
  say(text: string, turns?: number): Promise<void>
  // Waits for the session to ask the owner something, and answers it.
  answer(choice: string): Promise<void>
  // Replaces the older part of the conversation with a summary, as /compact does.
  compact(): Promise<void>
  // Sends a message and waits only until the session asks the owner
  // something, which is left unanswered.
  sayUntilAsked(text: string): Promise<void>
}

// One made-up session: what it shows, its project (none for the general
// session), the model's replies in order, one for each request it is sent,
// and what the owner does in it. A subagent it starts has replies of its
// own, since the two work at the same time; its requests are the ones that
// open with its task.
type Seeded = {
  kind: string
  project?: ProjectName
  replies: ScriptedReply[]
  subagent?: { task: string; replies: ScriptedReply[] }
  // The size, in tokens, at which a conversation counts as long, when it is
  // set small so that a few turns are enough to compact.
  compactAt?: number
  // Evie is not closed when the script is done, since closing it would
  // refuse the question the session is waiting on. The session is left as a
  // server that is stopped all at once leaves it, with the question in its
  // log and no answer.
  leftWaiting?: true
  run(session: Session): Promise<void>
}

const says = (...text: string[]): ScriptedReply => ({ text })

const RECIPE_TASK =
  "In this project, list the recipes in recipes/ that do not say how long they take to make. Change nothing. " +
  "Report the file names, one to a line."

const SESSIONS: Seeded[] = [
  {
    kind: "a plain conversation",
    replies: [
      says(
        "A plain text file is hard to beat: one line per thing to read, with the newest at the top. ",
        "When you finish one, move it to a second file, so you keep a record of what you have read.",
      ),
      says("Keep one file of things to read, newest first, and move each to a second file when it is done."),
    ],
    async run(session) {
      await session.say("What is a simple way to keep a list of things I want to read?")
      await session.say("Thanks. Could you say that in one sentence?")
    },
  },
  {
    kind: "tool calls",
    project: "recipe-box",
    replies: [
      { text: ["I'll read the README first."], calls: [{ name: "read", input: { path: "README.md" } }] },
      { text: [], calls: [{ name: "read", input: { path: "package.json" } }] },
      {
        text: ["The README says `npm test`, but package.json runs the tests with Bun."],
        calls: [{ name: "edit", input: { path: "README.md", oldString: "`npm test`", newString: "`bun test`" } }],
      },
      { text: [], calls: [{ name: "shell", input: { command: "git diff --stat" } }] },
      says("Fixed: the README now says to run the tests with `bun test`, as package.json does. Only README.md changed."),
    ],
    async run(session) {
      await session.say("The README gives the wrong command for running the tests. Can you fix it?")
    },
  },
  {
    kind: "a question answered",
    project: "garden-planner",
    replies: [
      {
        text: ["Before I add it, one thing to settle."],
        calls: [{ name: "ask", input: { question: "Should watering record the time as well as the day?", choices: ["Just the day", "The day and the time"] } }],
      },
      {
        text: [],
        calls: [
          {
            name: "edit",
            input: {
              path: "src/plants.ts",
              oldString: "export type Plant = { name: string; bed: string }",
              newString:
                "export type Plant = { name: string; bed: string; lastWatered?: string }\n\n" +
                "// Marks a plant as watered today. Only the day is kept.\n" +
                "export const water = (plant: Plant) => {\n  plant.lastWatered = new Date().toISOString().slice(0, 10)\n}",
            },
          },
        ],
      },
      says("Done: each plant can now have a `lastWatered` day, and `water(plant)` sets it to today. Only the day is kept, as you chose."),
    ],
    async run(session) {
      const said = session.say("Add a way to mark a plant as watered.")
      await session.answer("Just the day")
      await said
    },
  },
  {
    // The tests take a second, long after the turn that started them has
    // ended, so their finishing wakes the session for a second turn.
    kind: "a background job",
    project: "weather-station",
    replies: [
      { text: [], calls: [{ name: "shell", input: { command: "sh scripts/slow-tests.sh", background: true } }] },
      says("The slow tests are running in the background. I'll tell you how they went when they finish."),
      says("The slow tests have finished: all 12 passed."),
    ],
    async run(session) {
      await session.say("Run the slow tests in the background, and tell me when they're done.", 2)
    },
  },
  {
    // The subagent's report wakes the session for a second turn.
    kind: "a subagent",
    project: "recipe-box",
    replies: [
      { text: [], calls: [{ name: "subagent", input: { description: "Find recipes with no time", task: RECIPE_TASK } }] },
      says("A subagent is looking through the recipes. I'll pass on what it finds."),
      says("Two recipes say nothing about how long they take: flatbread.md and lentil-soup.md. Shall I add a time to each?"),
    ],
    subagent: {
      task: RECIPE_TASK,
      replies: [
        { text: [], calls: [{ name: "shell", input: { command: "grep -L 'Time:' recipes/*.md" } }] },
        says("Two recipes give no time:\nrecipes/flatbread.md\nrecipes/lentil-soup.md"),
      ],
    },
    async run(session) {
      await session.say("Which recipes don't say how long they take? Don't change anything.", 2)
    },
  },
  {
    kind: "a compacted conversation",
    project: "weather-station",
    compactAt: 1000,
    replies: [
      { text: ["Let me read it."], calls: [{ name: "read", input: { path: "src/readings.ts" } }] },
      says(
        "`average` adds up the temperature of every reading and divides by how many there are. It looks only at ",
        "`celsius`: humidity and time are kept but not used. Two things to know: an empty list gives `NaN`, since it ",
        "divides by zero, and every reading counts the same, however far apart in time the readings were taken.",
      ),
      says(
        "A time-weighted average: each reading counts for as long as it stood, until the next one came in. For a ",
        "station that reads every ten minutes but sometimes misses a few, that keeps one lonely reading from being ",
        "drowned out by a burst of others.",
      ),
      says(
        "Return `undefined` rather than `NaN`, and let the caller decide what to show, such as a dash. `NaN` spreads ",
        "quietly through any sum it touches, so a chart built on it goes blank without saying why.",
      ),
      says("Fine, nothing has been changed. The two changes we talked about are a time-weighted `average`, and `undefined` for an empty list."),
      // The summary that /compact asks for.
      says(
        "## Goal\n- Understand how src/readings.ts averages readings, and how it could be better.\n\n",
        "## What the owner asked for\n- Leave the code as it is for now.\n\n## Decisions\n- (none)\n\n",
        "## Done\n- Explained `average`: a plain mean of `celsius`, which gives NaN for an empty list.\n\n",
        "## In progress\n- (none)\n\n## State of the changes\n- Nothing changed.\n\n",
        "## Next step\n- When the owner is back: make `average` time-weighted, and return undefined for an empty list.\n\n",
        "## Files that matter\n- src/readings.ts: holds `average`.\n\n## What else is needed\n- (none)",
      ),
      says("Make `average` weight each reading by how long it stood until the next one, and have it return `undefined` for an empty list instead of `NaN`."),
    ],
    async run(session) {
      await session.say("How are the readings averaged? Walk me through src/readings.ts.")
      await session.say("What would be better for readings taken at uneven times?")
      await session.say("And for the empty list?")
      await session.say("Good. Let's leave the code as it is for now; I'll come back to it.")
      await session.compact()
      await session.say("I'm back. What were the two changes again?")
    },
  },
  // A session settled by hand belongs here too, once the core has a way to
  // settle one.
  {
    kind: "a question still waiting",
    project: "garden-planner",
    leftWaiting: true,
    replies: [
      {
        text: ["A few names come to mind."],
        calls: [{ name: "ask", input: { question: "Which name do you prefer for the project?", choices: ["Sprout", "Leafy", "Keep garden-planner"] } }],
      },
    ],
    async run(session) {
      await session.sayUntilAsked("Can you think of a friendlier name for this project?")
    },
  },
]

export async function seed(env: Settings) {
  const home = dataFolder(env)
  await mkdir(home, { recursive: true }).catch(fail)
  let count = 0
  try {
    count = sessionsIn(home)
  } catch (cause) {
    fail(cause)
  }
  if (count > 0) {
    fail(
      `${home} already has sessions, so nothing was seeded. The seed only fills a data folder with none, so that ` +
        "made-up sessions are never mixed in with real ones. Set EVIE_HOME to an empty folder to seed that.",
    )
  }

  for (const [name, contents] of Object.entries(PROJECTS)) await makeProject(join(projectsFolder(home), name), contents).catch(fail)
  for (const seeded of SESSIONS) await seedSession(env, home, seeded).catch((cause) => fail(`The made-up session "${seeded.kind}" did not go as scripted: ${reason(cause)}`))

  const width = Math.max(...SESSIONS.map((seeded) => seeded.kind.length))
  print(
    `Seeded ${home} with made-up data, made by a scripted model at no cost:\n` +
      `  projects, in ${projectsFolder(home)}: ${Object.keys(PROJECTS).join(", ")}\n` +
      `  sessions:\n` +
      SESSIONS.map((seeded) => `    ${seeded.kind.padEnd(width)}   ${seeded.project ?? "no project"}\n`).join(""),
  )
}

// How many sessions the data folder's database holds. Opening it makes it
// when there is none, and refuses one written by a newer Evie.
function sessionsIn(home: string): number {
  const db = openStore(join(home, "evie.db"))
  try {
    return db.query<{ count: number }, []>("SELECT COUNT(*) AS count FROM session").get()!.count
  } finally {
    db.close()
  }
}

// Writes a made-up project's files, from scratch, and makes it a git
// repository with one commit. Git is run apart from the owner's own git
// settings, so that nothing in them, such as signing every commit, applies.
async function makeProject(folder: string, contents: Record<string, string>) {
  await rm(folder, { recursive: true, force: true })
  for (const [file, text] of Object.entries(contents)) {
    await mkdir(dirname(join(folder, file)), { recursive: true })
    await writeFile(join(folder, file), text)
  }
  const env = {
    PATH: process.env.PATH ?? "",
    GIT_CONFIG_GLOBAL: "/dev/null",
    GIT_CONFIG_NOSYSTEM: "1",
    GIT_AUTHOR_NAME: "Evie seed",
    GIT_AUTHOR_EMAIL: "seed@example.invalid",
    GIT_COMMITTER_NAME: "Evie seed",
    GIT_COMMITTER_EMAIL: "seed@example.invalid",
  }
  for (const args of [["-c", "init.defaultBranch=main", "init", "-q"], ["add", "-A"], ["commit", "-q", "-m", "First version"]]) {
    const run = Bun.spawn(["git", ...args], { cwd: folder, env, stdout: "pipe", stderr: "pipe" })
    const [said, code] = await Promise.all([new Response(run.stderr).text(), run.exited])
    if (code !== 0) throw new Error(`git ${args.join(" ")} failed in ${folder}: ${said.trim()}`)
  }
}

// Runs one made-up session on an Evie of its own, with its own script, and
// closes it. Every turn must end as the script says, and use the whole of it.
async function seedSession(env: Settings, home: string, seeded: Seeded) {
  const own = scriptedProvider(seeded.replies)
  const subagent = seeded.subagent && { task: seeded.subagent.task, ...scriptedProvider(seeded.subagent.replies) }
  // One model for the session and its subagent, which answers each from its own script.
  const model: Plugin = {
    name: "scripted-provider",
    setup(host) {
      host.providers.add({
        name: "scripted",
        stream: (request, signal) => {
          const first = request.messages[0]
          const isSubagents = subagent && first?.role === "user" && first.text === subagent.task
          return (isSubagents ? subagent : own).provider.stream(request, signal)
        },
      })
    },
  }
  const plugins: Plugin[] = [model, files(), shell(), jobs(), ask(), subagents({ dataFolder: home })]
  const evie = await createEvie({ env, model: MODEL, plugins, ...(seeded.compactAt && { compaction: { ceiling: seeded.compactAt } }) })

  const problems: string[] = []
  let ended = 0
  // The question the session is waiting on, when there is one.
  let question: string | undefined
  const waiting = new Set<() => void>()
  let session = ""
  evie.subscribe((event) => {
    if (event.type === "turn_end" && event.body.reason !== "completed") problems.push(event.body.error ?? `a turn was ${event.body.reason}`)
    if (event.type === "turn_end" && event.session === session) ended += 1
    if (event.type === "question" && event.session === session) question = event.body.id
    if (event.type === "answer" && event.session === session) question = undefined
    for (const check of [...waiting]) check()
  })
  // Settles once `done` holds.
  const until = (done: () => boolean) =>
    new Promise<void>((resolve) => {
      const check = () => {
        if (!done()) return
        waiting.delete(check)
        resolve()
      }
      waiting.add(check)
      check()
    })

  try {
    const folder = seeded.project && join(projectsFolder(home), seeded.project)
    session = (await evie.send({ type: "open_session", questions: true, ...(folder && { folder }) })).session
    await seeded.run({
      async say(text, turns = 1) {
        const target = ended + turns
        await evie.send({ type: "send_message", session, text })
        await until(() => ended >= target || problems.length > 0)
      },
      async answer(choice) {
        await until(() => question !== undefined || problems.length > 0)
        if (question !== undefined) await evie.send({ type: "answer_question", session, id: question, choice })
      },
      async sayUntilAsked(text) {
        await evie.send({ type: "send_message", session, text })
        await until(() => question !== undefined || problems.length > 0)
      },
      async compact() {
        const { text } = await evie.send({ type: "run_command", session, name: "compact", args: "" })
        if (!text.startsWith("Compacted")) problems.push(text)
      },
    })
  } finally {
    if (!seeded.leftWaiting) await evie.close()
  }
  if (problems.length > 0) throw new Error(problems.join(" "))
  const allUsed = (who: string, asked: number, replies: number) => {
    if (asked !== replies) throw new Error(`the ${who} model was asked ${asked} times, and its script has ${replies} replies.`)
  }
  allUsed("session's", own.requests.length, seeded.replies.length)
  if (subagent) allUsed("subagent's", subagent.requests.length, seeded.subagent!.replies.length)
}
