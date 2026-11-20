import { mkdir } from "node:fs/promises"
import { homedir } from "node:os"
import { join } from "node:path"
import { createHost, type HookRunner } from "./host.ts"
import { createInbox } from "./inbox.ts"
import { createEventLog } from "./log.ts"
import { createLoop, currentModel, type Loop } from "./loop.ts"
import { createProjects } from "./projects.ts"
import { createQuestions } from "./questions.ts"
import { openStore } from "./store.ts"
import type { Ask, Commands, Evie, EvieEvent, Host, Plugin, Project, ProjectSettings, Provider } from "./types.ts"

export type EvieOptions = {
  // Settings from the environment. `EVIE_HOME` names the data folder.
  env: Record<string, string | undefined>
  // The model sessions start with, unless their project sets its own.
  model?: string
  // The longest a reply may be, in tokens, unless a project sets its own.
  maxTokens?: number
  // Where a model can be set, named when none is. It is only used in that message.
  settingsFile?: string
  // When a long conversation is compacted: a ceiling in tokens, and limits
  // for models whose own limit the provider does not know.
  compaction?: { ceiling?: number; limits?: Record<string, number> }
  // The global plugins, in order. They are part of every project and of the
  // general session.
  plugins: readonly Plugin[]
  // What a project has on top of the global plugins: its own, added after
  // them for that project only, and any global ones it switches off. It is
  // called the first time a session opens in the project, and can ask the
  // owner a question, such as whether to trust the project.
  projectSettings?: (project: Project, context: { ask: Ask }) => ProjectSettings | readonly Plugin[] | Promise<ProjectSettings | readonly Plugin[]>
}

// Where all of Evie's state lives. Moving Evie is copying this folder.
export function dataFolder(env: Record<string, string | undefined>): string {
  return env.EVIE_HOME || join(homedir(), ".evie")
}

// Starts the core and returns the client interface: send a command, and
// subscribe to events. A client never reaches the core any other way.
//
// Evie is not started in a project. A client opens a session, which belongs
// to a project or to none, and one Evie serves every open session.
export async function createEvie(options: EvieOptions): Promise<Evie> {
  // Each project has a plugin host of its own, built the first time a session
  // opens in it: the global plugins, then the project's own. The general
  // session's host has the global plugins alone, and is built now, so that a
  // problem with them stops Evie at start.
  type Scope = {
    host: Host
    hooks: HookRunner
    provider: Provider
    notices: string[]
    // The project's own model and reply limit, when it sets them.
    model?: string
    maxTokens?: number
  }
  const scopes = new Map<string | null, Promise<Scope>>()

  async function buildScope(project: Project | null, ask: Ask): Promise<Scope> {
    const given = project && options.projectSettings ? await options.projectSettings(project, { ask }) : []
    const own: ProjectSettings = "plugins" in given ? given : { plugins: given }
    const kept = options.plugins.filter((plugin) => !own.disable?.includes(plugin.name))
    const { host, hooks } = await createHost([...kept, ...own.plugins], project)
    // A command of the core's own, beside the ones plugins register.
    host.commands.add({
      name: "compact",
      description: "Replace the older part of the conversation with a summary, to save room.",
      run: (_args, context) => context.compact(),
    })
    const provider = host.providers.list()[0]
    if (!provider) throw new Error("No provider is registered. Load a plugin that registers one.")
    return {
      host,
      hooks,
      provider,
      notices: [...(own.notices ?? [])],
      ...(own.model !== undefined && { model: own.model }),
      ...(own.maxTokens !== undefined && { maxTokens: own.maxTokens }),
    }
  }

  // `ask` is used only when the scope is built now, by the session that is
  // opening the project for the first time.
  function scopeFor(project: Project | null, ask: Ask): Promise<Scope> {
    const key = project?.id ?? null
    let scope = scopes.get(key)
    if (!scope) {
      scope = buildScope(project, ask)
      scopes.set(key, scope)
      // A set that failed to build is forgotten, so the next session tries again.
      scope.catch(() => scopes.delete(key))
    }
    return scope
  }

  await scopeFor(null, async () => null)

  const home = dataFolder(options.env)
  await mkdir(home, { recursive: true })
  const db = openStore(join(home, "evie.db"))
  const log = createEventLog(db)
  const inbox = createInbox(db)
  const projects = createProjects(db)

  const listeners = new Set<(event: EvieEvent) => void>()
  const emit = (event: EvieEvent) => {
    for (const listener of [...listeners]) {
      // A client that throws while handling an event must not break the turn.
      try {
        listener(event)
      } catch {}
    }
  }

  const questions = createQuestions({ log, emit })
  let closed = false

  // Each session open in this process: its turn loop, its project and that
  // project's plugin host. `canAnswer` says whether its client shows questions.
  type Open = { loop: Loop; project: Project | null; host: Host; model: string; canAnswer: boolean }
  const sessions = new Map<string, Open>()

  async function openSession(input: Commands["open_session"]["input"]): Promise<Commands["open_session"]["output"]> {
    if (input.project !== undefined && input.folder !== undefined) {
      throw new Error("Open a session with a project or a folder, not both.")
    }
    let project = null
    if (input.project !== undefined) {
      project = projects.get(input.project) ?? null
      if (!project) throw new Error(`There is no project with the id "${input.project}".`)
    } else if (input.folder !== undefined) {
      project = projects.forFolder(input.folder)
    }

    // Continuing: the session used last in this project, when there is one.
    const last = input.continue ? projects.lastSession(project) : undefined
    // A new session is created only once it is needed: when the project's
    // settings ask a question, or when everything else has gone well.
    let session = last
    const sessionId = () => (session ??= projects.createSession(project))
    const canAnswer = input.questions === true

    const scope = await scopeFor(project, (question) => questions.ask(sessionId(), question, { canAnswer }))
    if (closed) throw new Error("Evie was closed while the session was opening.")
    const { host, hooks, provider, notices } = scope
    // A project's own model wins over the global one.
    const model = scope.model ?? options.model
    if (!model) {
      const where = options.settingsFile ? `Add "model" to ${options.settingsFile}` : "Set a model"
      throw new Error(`No model is set. ${where}, with a name from https://openrouter.ai/models.`)
    }
    const maxTokens = scope.maxTokens ?? options.maxTokens

    // From here to the session being listed as open nothing waits, so two
    // clients continuing the same session at once cannot both start it.
    const alreadyOpen = last === undefined ? undefined : sessions.get(last)
    if (last !== undefined && alreadyOpen) {
      alreadyOpen.canAnswer ||= canAnswer
      return { session: last, project, resumed: true, notices }
    }
    if (last !== undefined) settleUnfinished(last)

    const opened: Open = { project, host, model, canAnswer } as Open
    opened.loop = createLoop({
      session: sessionId(),
      project,
      outputFolder: join(home, "tool-output"),
      model,
      ...(maxTokens !== undefined && { maxTokens }),
      ...(options.compaction && { compaction: options.compaction }),
      log,
      inbox,
      host,
      hooks,
      provider,
      atomically: (work) => db.transaction(work)(),
      emit,
      ask: (question, signal) => questions.ask(sessionId(), question, { canAnswer: opened.canAnswer, signal }),
    })
    sessions.set(sessionId(), opened)
    // A message that was waiting when Evie last stopped is answered now.
    if (last !== undefined) opened.loop.wake()
    return { session: sessionId(), project, resumed: last !== undefined, notices }
  }

  // If Evie was stopped in the middle of a turn, the log ends with a turn
  // that never finished, and perhaps tool calls with no result or a question
  // with no answer. This closes them off, so the conversation is one a model
  // will accept again and no question is left looking open.
  function settleUnfinished(session: string): void {
    const answered = new Set(log.events(session).flatMap((event) => (event.type === "answer" ? [event.body.id] : [])))
    for (const event of log.events(session)) {
      if (event.type === "question" && !answered.has(event.body.id)) emit(log.append(session, "answer", { id: event.body.id, choice: null }))
    }
    settleUnfinishedTurn(session)
  }

  function settleUnfinishedTurn(session: string): void {
    const events = log.events(session)
    const started = events.findLastIndex((event) => event.type === "turn_start")
    if (started === -1) return
    const turn = events.slice(started)
    if (turn.some((event) => event.type === "turn_end")) return

    const answered = new Set(turn.flatMap((event) => (event.type === "tool_result" ? [event.body.callId] : [])))
    const asked = turn.flatMap((event) => (event.type === "assistant_message" ? (event.body.toolCalls ?? []) : []))
    const closing = db.transaction(() => [
      ...asked
        .filter((call) => !answered.has(call.id))
        .map((call) =>
          log.append(session, "tool_result", {
            callId: call.id,
            name: call.name,
            text: "Evie stopped before this tool finished, so it has no result.",
          }),
        ),
      log.append(session, "turn_end", { reason: "error", error: "Evie stopped while this turn was running." }),
    ])()
    for (const event of closing) emit(event)
  }

  function open(session: string): Open {
    const found = sessions.get(session)
    if (!found) throw new Error(`The session "${session}" is not open.`)
    return found
  }

  function sendMessage(input: Commands["send_message"]["input"]): void {
    const { loop } = open(input.session)
    inbox.add(input.session, { text: input.text })
    loop.wake()
  }

  // The switch is written to the log, and the next request reads it from there.
  function setModel(input: Commands["set_model"]["input"]): void {
    open(input.session)
    const model = input.model.trim()
    if (model === "") throw new Error("Give the name of a model, as listed at https://openrouter.ai/models.")
    emit(log.append(input.session, "model_changed", { model }))
  }

  async function runCommand(input: Commands["run_command"]["input"]): Promise<Commands["run_command"]["output"]> {
    const { project, host, model: startedWith, loop } = open(input.session)
    const command = host.commands.list().find((candidate) => candidate.name === input.name)
    if (!command) {
      const names = host.commands.list().map((candidate) => `/${candidate.name}`).join(", ") || "none"
      throw new Error(`There is no command "/${input.name}". The commands are: ${names}.`)
    }
    const text = await command.run(input.args, {
      session: input.session,
      project,
      model: currentModel(log.events(input.session), startedWith),
      setModel: (model) => setModel({ session: input.session, model }),
      compact: () => loop.compactNow(),
      busy: loop.busy(),
      tell: (note) => loop.tell(note),
    })
    return { text }
  }

  return {
    async send(command) {
      // Each branch returns what its command promises; the cast tells the
      // compiler so, since it cannot follow the command's type through.
      type Output = Commands[(typeof command)["type"]]["output"]
      switch (command.type) {
        case "open_session":
          return (await openSession(command)) as Output
        case "send_message":
          return sendMessage(command) as Output
        case "cancel":
          return open(command.session).loop.cancel() as Output
        case "set_model":
          return setModel(command) as Output
        case "answer_question":
          // The session may still be opening: a project can ask before it is open.
          return questions.answer(command.session, command.id, command.choice) as Output
        case "run_command":
          return (await runCommand(command)) as Output
        default:
          // Reached only by a client that is not type-checked.
          throw new Error(`Unknown command "${(command as { type: unknown }).type}".`)
      }
    },
    subscribe(listener) {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
    async close() {
      closed = true
      // A question still open is refused, which lets whatever asked it finish.
      questions.refuseAll()
      await Promise.all([...sessions.values()].map(({ loop }) => loop.stop()))
      db.close()
    },
  }
}
