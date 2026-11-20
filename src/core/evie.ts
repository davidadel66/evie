import { mkdir } from "node:fs/promises"
import { homedir } from "node:os"
import { join } from "node:path"
import { createHost, type HookRunner } from "./host.ts"
import { createInbox } from "./inbox.ts"
import { createJobs } from "./jobs.ts"
import { createEventLog } from "./log.ts"
import { createLoop, currentModel, type Loop } from "./loop.ts"
import { createProjects } from "./projects.ts"
import { createQuestions } from "./questions.ts"
import { openStore } from "./store.ts"
import type { Ask, Children, Commands, Evie, EvieEvent, Host, Plugin, Project, ProjectSettings, Provider } from "./types.ts"

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
  // Job output goes beside the tool output that was too long to show.
  const jobs = createJobs({
    outputFolder: join(home, "tool-output"),
    emit,
    tell: (session, text, wake) => sessions.get(session)?.loop.tell(text, wake),
  })

  // Each session open in this process: its turn loop, its project and that
  // project's plugin host. `canAnswer` says whether its client shows questions.
  // `parent` is the session that started this one as a subagent.
  type Open = { loop: Loop; project: Project | null; host: Host; model: string; canAnswer: boolean; parent?: string }
  const sessions = new Map<string, Open>()

  // Starts the turn loop of a session and lists the session as open. Used
  // for a session a client opens and for a subagent alike: they differ only
  // in who is asked when the session has a question.
  function startLoop(given: { session: string; project: Project | null; scope: Scope; model: string; canAnswer: boolean; parent?: string }): Open {
    const { session, project, scope, model, parent } = given
    const as = parent === undefined ? { readOnly: false } : projects.startedAs(session)
    const maxTokens = scope.maxTokens ?? options.maxTokens
    const opened: Open = { project, host: scope.host, model, canAnswer: given.canAnswer, ...(parent !== undefined && { parent }) } as Open
    opened.loop = createLoop({
      session,
      project,
      outputFolder: join(home, "tool-output"),
      model,
      ...(maxTokens !== undefined && { maxTokens }),
      ...(options.compaction && { compaction: options.compaction }),
      log,
      inbox,
      host: scope.host,
      hooks: scope.hooks,
      provider: scope.provider,
      atomically: (work) => db.transaction(work)(),
      emit,
      // A subagent has no client of its own. What it needs to ask is put to
      // the owner in the session that started it, marked as coming from it.
      ask: (question, signal) =>
        parent === undefined
          ? questions.ask(session, question, { canAnswer: opened.canAnswer, signal })
          : questions.ask(parent, { ...question, text: `A subagent asks: ${question.text}` }, { canAnswer: sessions.get(parent)?.canAnswer ?? false, signal }),
      jobs: {
        start: (work) => jobs.start(session, work),
        list: () => jobs.list(session),
        stop: (id) => jobs.stop(session, id, "model"),
        whileWaiting: (asked) => jobs.whileWaiting(session, asked),
      },
      children: childrenOf(session, project, scope),
      ...(parent !== undefined && { parent }),
      ...(as.agent !== undefined && { agent: as.agent }),
      ...(as.readOnly && { readOnly: true }),
    })
    sessions.set(session, opened)
    return opened
  }

  // A session's subagents, as its tools are given them.
  function childrenOf(session: string, project: Project | null, scope: Scope): Children {
    // One of this session's subagents, opened again if Evie has been
    // stopped since it was started.
    const child = (id: string): Open => {
      if (projects.parentOf(id) !== session) throw new Error(`"${id}" is not a subagent of this session.`)
      const open = sessions.get(id)
      if (open) return open
      settleUnfinished(id)
      // It carries on with the model it was given, which its log records.
      const parentModel = currentModel(log.events(session), sessions.get(session)!.model)
      return startLoop({ session: id, project, scope, model: currentModel(log.events(id), parentModel), canAnswer: false, parent: session })
    }
    const send = (id: string, text: string) => {
      const { loop } = child(id)
      inbox.add(id, { text })
      loop.wake()
    }
    return {
      async start(input) {
        if (closed) throw new Error("Evie is closing.")
        if (projects.parentOf(session) !== undefined) throw new Error("A subagent cannot start subagents of its own.")
        const model = input.model?.trim() || currentModel(log.events(session), sessions.get(session)!.model)
        const id = projects.createSession(project, session, {
          ...(input.agent !== undefined && { agent: input.agent }),
          ...(input.readOnly && { readOnly: true }),
        })
        // Recorded in its log, so that it keeps this model if it is opened again later.
        log.append(id, "model_changed", { model })
        startLoop({ session: id, project, scope, model, canAnswer: false, parent: session })
        send(id, input.text)
        return id
      },
      async send(id, text) {
        if (closed) throw new Error("Evie is closing.")
        send(id, text)
      },
      async whenIdle(id) {
        const { loop } = child(id)
        // A subagent has nothing left to do when its turns have ended and
        // so have any background jobs of its own, whose ending wakes it for
        // another turn.
        do {
          await loop.idle()
          await jobs.whenEnded(id)
        } while (loop.busy())
        // Its report is what it said in full in its last turn, not something from an earlier round.
        const events = log.events(id)
        const lastTurn = events.slice(events.findLastIndex((event) => event.type === "turn_start"))
        const ended = lastTurn.findLast((event) => event.type === "turn_end")
        const said = lastTurn.findLast((event) => event.type === "assistant_message" && !event.body.interrupted && event.body.text.trim() !== "")
        return {
          reply: said?.type === "assistant_message" ? said.body.text : "",
          ended: ended?.type === "turn_end" ? ended.body.reason : "error",
          ...(ended?.type === "turn_end" && ended.body.error !== undefined && { error: ended.body.error }),
        }
      },
      busy: (id) => sessions.get(id)?.loop.busy() ?? false,
      // Stopping is stopping: messages it had not yet read are dropped, so
      // that it does not carry on with them by itself.
      stop: (id) => {
        const { loop } = child(id)
        inbox.take(id)
        loop.cancel()
      },
    }
  }

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
    const { notices } = scope
    // A project's own model wins over the global one.
    const model = scope.model ?? options.model
    if (!model) {
      const where = options.settingsFile ? `Add "model" to ${options.settingsFile}` : "Set a model"
      throw new Error(`No model is set. ${where}, with a name from https://openrouter.ai/models.`)
    }

    // From here to the session being listed as open nothing waits, so two
    // clients continuing the same session at once cannot both start it.
    const alreadyOpen = last === undefined ? undefined : sessions.get(last)
    if (last !== undefined && alreadyOpen) {
      alreadyOpen.canAnswer ||= canAnswer
      return { session: last, project, resumed: true, notices }
    }
    if (last !== undefined) settleUnfinished(last)

    const opened = startLoop({ session: sessionId(), project, scope, model, canAnswer })
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
    // A command's own name first, then any command that covers the name.
    let command = host.commands.list().find((candidate) => candidate.name === input.name)
    if (!command) {
      for (const candidate of host.commands.list()) {
        if (await candidate.covers?.(input.name)) {
          command = candidate
          break
        }
      }
    }
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
      // Read each time it is asked, since a turn can start while a command runs.
      get busy() {
        return loop.busy()
      },
      tell: (note) => loop.tell(note),
      invokedAs: input.name,
      send: (text) => sendMessage({ session: input.session, text }),
      jobs: {
        list: () => jobs.list(input.session),
        stop: (id) => jobs.stop(input.session, id, "owner"),
        background: () => jobs.background(input.session),
      },
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
      // Jobs are marked and noted as stopped by the closing first, while
      // the sessions that started them can still record it. Then the turns
      // are stopped, and then the jobs are waited for.
      const jobsStopped = jobs.stopAll()
      await Promise.all([...sessions.values()].map(({ loop }) => loop.stop()))
      await jobsStopped
      db.close()
    },
  }
}
