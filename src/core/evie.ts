import { mkdir } from "node:fs/promises"
import { homedir } from "node:os"
import { join } from "node:path"
import { createHost, type HookRunner } from "./host.ts"
import { createInbox } from "./inbox.ts"
import { createJobs, EndNotKnown } from "./jobs.ts"
import { createEventLog } from "./log.ts"
import { createLoop, currentModel, type Loop } from "./loop.ts"
import { createProjects } from "./projects.ts"
import { createQuestions } from "./questions.ts"
import { openStore } from "./store.ts"
import { createSummaries } from "./summaries.ts"
import type { Ask, Children, Commands, Core, EvieEvent, Host, JobRecord, JobWork, Plugin, Project, ProjectSettings, Provider } from "./types.ts"

type Cut = "none" | "asking" | "working"

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
  // How often, in milliseconds, sessions are looked at for any that a week
  // of quiet has settled, to send their summaries: nothing happens in a
  // session to tell of it. Once an hour when left out, which is for tests to change.
  quietCheckEvery?: number
}

// How many events a page of a session's history holds, unless it asks for
// another number, and the most it can ask for.
const PAGE = 50
const LONGEST_PAGE = 500

// What the model is told when a turn a restart cut off is carried on, and
// when it is not. A turn is carried on only when it was cut off within the
// last hour, as by a restart, an update or a crash, which come back within
// seconds: older work is not started again by itself. It is carried on
// after at most three restarts, in five minutes for the owner's session
// and on one task for a subagent, so that a turn that itself stops the
// server cannot keep it restarting.
const RESTARTED =
  "Evie's server restarted while you were working, and your turn was cut short. Carry on from where you left off. A " +
  "command or other action you started may have finished, or may still be running: before you repeat one that " +
  "changes something, check that it did not take effect and is not still running, and if you cannot tell, say so " +
  "rather than repeat it."
const STOPPED_ASKING =
  "Evie's server restarted while this turn was waiting for an answer to a question, so the turn was stopped and the " +
  "question went unanswered. It is left for the owner to carry on."
const STOPPED_LONG_AGO =
  "Evie's server restarted while you were working, and this turn was cut off more than an hour ago, so it was left " +
  "stopped rather than carried on now. It is left for the owner to carry on."
const CUT_OFF_WITHIN = 60 * 60_000
const STOPPED_AGAIN =
  "Evie's server restarted while you were working, and this turn had already been carried on after three restarts, " +
  "so it was left stopped, in case it is what stops the server."
const MOST_RESTARTS = 3
const RESTARTS_WINDOW = 5 * 60_000

// Where all of Evie's state lives. Moving Evie is copying this folder.
export function dataFolder(env: Record<string, string | undefined>): string {
  return env.EVIE_HOME || join(homedir(), ".evie")
}

// Starts the core and returns the client interface: send a command, and
// subscribe to events. A client never reaches the core any other way.
//
// Evie is not started in a project. A client opens a session, which belongs
// to a project or to none, and one Evie serves every open session.
export async function createEvie(options: EvieOptions): Promise<Core> {
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
    // Built from settings that hold for now only, so not kept.
    forNow?: true
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
      ...(own.forNow && { forNow: true as const }),
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
      // A set that failed to build is forgotten, so the next session tries
      // again. So is one built from settings that hold for now only, such as
      // while the owner has not said whether to trust the project, so the
      // next session asks again. A session already open keeps the set it has.
      scope.then(
        (built) => built.forNow && scopes.delete(key),
        () => scopes.delete(key),
      )
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
  const deliver = (event: EvieEvent) => {
    for (const listener of [...listeners]) {
      // A client that throws while handling an event must not break the turn.
      try {
        listener(event)
      } catch {}
    }
  }
  // Every event goes out this way. Then the summary of its session, if the
  // event changed it. A piece of a reply cannot, and is sent too often to look.
  const emit = (event: EvieEvent) => {
    deliver(event)
    if (closed || event.type === "assistant_delta" || event.type === "summary") return
    // The event has been sent, and a stored one stored, so a summary that
    // cannot be worked out, as while the database is busy, must not break
    // the turn. It is skipped, and the next change sends one.
    try {
      summaries.changed(event.session)
    } catch {}
  }

  const questions = createQuestions({ log, emit })
  let closed = false
  // Job output goes beside the tool output that was too long to show.
  const jobs = createJobs({
    outputFolder: join(home, "tool-output"),
    emit,
    tell: (session, text, wake, kept) => sessions.get(session)?.loop.tell(text, wake, kept),
  })

  // Each session open in this process: its turn loop, its project and that
  // project's plugin host. `canAnswer` says whether its client shows questions.
  // `parent` is the session that started this one as a subagent, and
  // `children` are its own subagents, as its tools are given them; `child`
  // opens one of them, if it is not open yet.
  type Open = { loop: Loop; project: Project | null; host: Host; notices: string[]; model: string; canAnswer: boolean; parent?: string; children: Children; child(id: string): Open }
  const sessions = new Map<string, Open>()

  const summaries = createSummaries({
    db,
    asking: (session) => questions.waiting(session).length > 0,
    open: (session) => sessions.has(session),
    jobRunning: (session) => jobs.list(session).some((job) => job.status === "running"),
    emit: deliver,
  })
  // It is no reason on its own to keep the process running. A look that
  // fails, as while the database is busy, must not stop Evie: the next tries again.
  const quietCheck = setInterval(() => {
    try {
      summaries.look()
    } catch {}
  }, options.quietCheckEvery ?? 60 * 60_000).unref()

  // Starts the turn loop of a session and lists the session as open. Used
  // for a session a client opens and for a subagent alike: they differ only
  // in who is asked when the session has a question.
  function startLoop(given: { session: string; project: Project | null; scope: Scope; model: string; canAnswer: boolean; parent?: string }): Open {
    const { session, project, scope, model, parent } = given
    const as = parent === undefined ? { readOnly: false } : projects.startedAs(session)
    const maxTokens = scope.maxTokens ?? options.maxTokens
    const { children, child } = childrenOf(session, project, scope)
    const opened: Open = { project, host: scope.host, notices: scope.notices, model, canAnswer: given.canAnswer, children, child, ...(parent !== undefined && { parent }) } as Open
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
          : questions.ask(parent, { ...question, text: `A subagent asks: ${question.text}` }, { canAnswer: sessions.get(parent)?.canAnswer ?? false, signal, asker: session }),
      jobs: {
        start: (work) => jobs.start(session, work),
        reserve: (work) => jobs.reserve(session, work),
        list: () => jobs.list(session),
        stop: (id) => jobs.stop(session, id, "model"),
        whileWaiting: (asked) => jobs.whileWaiting(session, asked),
      },
      children,
      ...(parent !== undefined && { parent }),
      ...(as.agent !== undefined && { agent: as.agent }),
      ...(as.readOnly && { readOnly: true }),
    })
    sessions.set(session, opened)
    return opened
  }

  // A session's subagents, as its tools are given them, and what opens one.
  function childrenOf(session: string, project: Project | null, scope: Scope): { children: Children; child(id: string): Open } {
    // One of this session's subagents, opened again if Evie has been
    // stopped since it was started.
    const child = (id: string): Open => {
      if (projects.parentOf(id) !== session) throw new Error(`"${id}" is not a subagent of this session.`)
      const open = sessions.get(id)
      if (open) return open
      const left = settleLeft(id)
      // It carries on with the model it was given, which its log records.
      const parentModel = currentModel(log.events(session), sessions.get(session)!.model)
      const opened = startLoop({ session: id, project, scope, model: currentModel(log.events(id), parentModel), canAnswer: false, parent: session })
      carryOn(id, opened, left)
      return opened
    }
    const send = (id: string, text: string) => {
      const { loop } = child(id)
      inbox.add(id, { text })
      loop.wake()
    }
    const children: Children = {
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
    return { children, child }
  }

  // `resuming` is for a session opened to pick up work left in it, with no
  // client yet: its questions wait for the owner, though one asked while it
  // opens, which no client could be shown, is refused.
  async function openSession(input: Commands["open_session"]["input"], resuming = false): Promise<Commands["open_session"]["output"]> {
    if (input.project !== undefined && input.folder !== undefined) {
      throw new Error("Open a session with a project or a folder, not both.")
    }
    if (input.folder !== undefined && typeof input.folder !== "string") throw new Error("`folder` is the path of a folder, and must be text.")
    if (input.session !== undefined && (input.project !== undefined || input.folder !== undefined || input.continue === true)) {
      throw new Error("Open a session by its id alone, without a project, a folder or `continue`.")
    }
    if (input.after !== undefined && input.session === undefined) {
      throw new Error("`after` is the number of the last event seen in a session, so it is given only with `session`.")
    }
    let project = null
    if (input.session !== undefined) {
      const found = projects.projectOf(input.session)
      if (found === undefined) throw new Error(`There is no session with the id "${input.session}".`)
      // A subagent runs as part of the session that started it, which asks
      // its questions and opens it again when needed.
      const parent = projects.parentOf(input.session)
      if (parent !== undefined) throw new Error(`The session "${input.session}" is a subagent's. Open the session that started it, "${parent}", instead.`)
      project = found
    } else if (input.project !== undefined) {
      project = projects.get(input.project) ?? null
      if (!project) throw new Error(`There is no project with the id "${input.project}".`)
    } else if (input.folder !== undefined) {
      project = projects.forFolder(input.folder)
    }

    // Continuing: the session used last in this project, when there is one,
    // or the one named, which is opened in its own project the same way.
    const last = input.session ?? (input.continue ? projects.lastSession(project) : undefined)
    // A new session is created only once it is needed: when the project's
    // settings ask a question, or when everything else has gone well.
    let session = last
    const sessionId = () => (session ??= projects.createSession(project))
    const canAnswer = input.questions === true

    // A session still open in this process, such as one another client left
    // running, is joined as it is. Its project's plugins were set up when it
    // opened, so nothing is set up or asked about again.
    const alreadyOpen = () => (last === undefined ? undefined : sessions.get(last))
    const join = (id: string, open: Open) => {
      open.canAnswer ||= canAnswer
      return { ...whereItIs(id, input.after), project, resumed: true, notices: open.notices }
    }
    const openBefore = alreadyOpen()
    if (last !== undefined && openBefore && !closed) return join(last, openBefore)

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
    const openAfter = alreadyOpen()
    if (last !== undefined && openAfter) return join(last, openAfter)
    const left = last !== undefined ? settleLeft(last) : undefined

    const opened = startLoop({ session: sessionId(), project, scope, model, canAnswer: canAnswer || resuming })
    if (left) carryOn(sessionId(), opened, left)
    return { ...whereItIs(sessionId(), input.after), project, resumed: last !== undefined, notices }
  }

  // Where a session that is being opened stands: whether a turn is running
  // in it, as in one another client left running, the questions it is still
  // waiting on, its jobs, and the reply being written. A turn is running
  // when the log's last turn has not ended. With `after`, the events after
  // that number too.
  // All of it is read in one step, with nothing waited for in between, so it
  // describes a single moment: every event emitted later is sent as it happens.
  function whereItIs(session: string, after?: number) {
    const events = log.events(session)
    const turnRunning = events.findLastIndex((event) => event.type === "turn_start") > events.findLastIndex((event) => event.type === "turn_end")
    const replySoFar = sessions.get(session)?.loop.replySoFar() ?? ""
    return {
      session,
      turnRunning,
      openQuestions: questions.waiting(session),
      lastSeq: events.at(-1)?.seq ?? 0,
      jobs: jobs.list(session),
      ...(after !== undefined && { events: events.filter((event) => event.seq > after) }),
      ...(replySoFar !== "" && { replySoFar }),
    }
  }

  // What an earlier run of Evie left in a session that is opening in this
  // process for the first time: the records of its jobs not yet dealt with,
  // and the calls of a turn it cut off that were waiting on one of them.
  // `cut` says whether it ended with a turn cut off, and if so whether the
  // turn was waiting for an answer to a question.
  // `cutAt` is when the turn's last event was written.
  type Left = { records: JobRecord[]; moved: Set<string>; cut: Cut; cutAt: number }

  // Closes off a turn the session's last run cut off, before the session
  // opens, so the conversation is one a model will accept again. A call that
  // was waiting on work that may become a job, such as a command, is told
  // the work carries on as a job.
  function settleLeft(session: string): Left {
    const records = jobs.left(session)
    const waitedOn = new Map(records.flatMap((record) => (record.inCall && record.call !== undefined ? [[record.call, record]] : [])))
    const { given: moved, cut, cutAt } = settleUnfinished(session, (call) => {
      const record = waitedOn.get(call)
      return (
        record &&
        `Evie restarted while this was running, so it was moved to the background, where it carries on as job ${record.id}, ` +
          `and its output is being written to ${record.outputFile}. You will be told when it ends, with its output. Do not start another copy while it is running.`
      )
    })
    return { records, moved, cut, cutAt }
  }

  // Once the session is open: a turn cut off is carried on, with a note,
  // unless it was waiting for an answer or has been carried on too often;
  // its jobs left are taken back, each picked up by the plugin of its kind;
  // and a message that was waiting when Evie last stopped is answered.
  function carryOn(session: string, opened: Open, { records, moved, cut, cutAt }: Left): void {
    if (cut === "asking") opened.loop.tell(STOPPED_ASKING)
    if (cut === "working") {
      const recent = Date.now() - cutAt <= CUT_OFF_WITHIN
      const again = recent && restartsOf(session) < MOST_RESTARTS
      opened.loop.tell(!recent ? STOPPED_LONG_AGO : again ? RESTARTED : STOPPED_AGAIN, again)
    }
    for (const record of records) {
      // Work a call was waiting on that ended within its call never became a job.
      if (record.inCall && !moved.has(record.call ?? "")) {
        jobs.release(record)
        continue
      }
      const kind = opened.host.jobKinds.list().find((each) => each.name === record.kind)
      let work: JobWork
      try {
        work = kind ? kind.resume(record, { children: opened.children }) : lost(record, `no plugin loaded now picks up work of the kind "${record.kind}".`)
      } catch (cause) {
        work = lost(record, cause instanceof Error ? cause.message : String(cause))
      }
      jobs.adopt(record, work)
    }
    // A turn carried on takes the message in itself.
    if (!opened.loop.busy()) opened.loop.wake()
  }

  // How many times a session's turns have been carried on after a restart:
  // in the last five minutes, or for a subagent, since its task was given.
  function restartsOf(session: string): number {
    const events = log.events(session)
    const counted =
      projects.parentOf(session) === undefined
        ? events.filter((event) => event.time >= Date.now() - RESTARTS_WINDOW)
        : events.slice(events.findLastIndex((event) => event.type === "user_message") + 1)
    return counted.filter((event) => event.type === "note" && event.body.text === RESTARTED).length
  }

  // Work left that cannot be picked up again.
  const lost = (record: JobRecord, why: string): JobWork => ({
    kind: record.kind,
    title: record.title,
    done: Promise.reject(
      new EndNotKnown(`Evie restarted while it was running, and could not pick it up again: ${why} Check whether it took effect, and run it again only if it did not.`),
    ),
    stop: () => {},
  })

  // If Evie was stopped in the middle of a turn, the log ends with a turn
  // that never finished, and perhaps tool calls with no result or a question
  // with no answer. This closes them off, so the conversation is one a model
  // will accept again and no question is left looking open. `resultOf` gives
  // a call's result when there is more to say than that it has none.
  // Returns the calls given such a result, and how the turn was cut off.
  function settleUnfinished(session: string, resultOf: (call: string) => string | undefined): { given: Set<string>; cut: Cut; cutAt: number } {
    const before = log.events(session)
    // The questions this session asked: in its own log, and for a
    // subagent, in the log of the session that started it. A subagent's
    // questions in this one's log are its own to settle.
    const parent = projects.parentOf(session)
    const unanswered = [
      ...unansweredIn(session, (asker) => asker === undefined),
      ...(parent === undefined ? [] : unansweredIn(parent, (asker) => asker === session)),
    ]
    for (const { where, id } of unanswered) emit(log.append(where, "answer", { id, choice: null }))
    const given = new Set<string>()
    const events = log.events(session)
    const started = events.findLastIndex((event) => event.type === "turn_start")
    if (started === -1) return { given, cut: "none", cutAt: 0 }
    const turn = events.slice(started)
    if (turn.some((event) => event.type === "turn_end")) return { given, cut: "none", cutAt: 0 }
    // Its last event before any written here.
    const cutAt = before.at(-1)!.time
    const asking = unanswered.some(({ time }) => time >= events[started]!.time)

    const done = new Set(turn.flatMap((event) => (event.type === "tool_result" ? [event.body.callId] : [])))
    const asked = turn.flatMap((event) => (event.type === "assistant_message" ? (event.body.toolCalls ?? []) : []))
    const closing = db.transaction(() => [
      ...asked
        .filter((call) => !done.has(call.id))
        .map((call) => {
          const text = resultOf(call.id)
          if (text !== undefined) given.add(call.id)
          return log.append(session, "tool_result", { callId: call.id, name: call.name, text: text ?? "Evie stopped before this tool finished, so it has no result." })
        }),
      log.append(session, "turn_end", { reason: "error", error: "Evie stopped while this turn was running." }),
    ])()
    for (const event of closing) emit(event)
    return { given, cut: asking ? "asking" : "working", cutAt }
  }

  // The questions in a session's log with no answer, of those whose asker matches.
  function unansweredIn(where: string, askedBy: (asker: string | undefined) => boolean) {
    const events = log.events(where)
    const answered = new Set(events.flatMap((event) => (event.type === "answer" ? [event.body.id] : [])))
    return events.flatMap((event) => (event.type === "question" && askedBy(event.body.asker) && !answered.has(event.body.id) ? [{ where, id: event.body.id, time: event.time }] : []))
  }

  function open(session: string): Open {
    const found = sessions.get(session)
    if (!found) throw new Error(`The session "${session}" is not open.`)
    return found
  }

  // Here and in the commands that follow, a field read as text is checked to
  // be text, since a client that is not type-checked can send anything, and
  // what is stored is read back as text.
  function sendMessage(input: Commands["send_message"]["input"]): void {
    if (typeof input.text !== "string") throw new Error("`text` is the message, and must be text.")
    const { loop } = open(input.session)
    inbox.add(input.session, { text: input.text })
    loop.wake()
  }

  // The switch is written to the log, and the next request reads it from there.
  function setModel(input: Commands["set_model"]["input"]): void {
    if (typeof input.model !== "string") throw new Error("`model` is the name of a model, and must be text.")
    open(input.session)
    const model = input.model.trim()
    if (model === "") throw new Error("Give the name of a model, as listed at https://openrouter.ai/models.")
    emit(log.append(input.session, "model_changed", { model }))
  }

  // Written to the log, where the session's state is read from. Any session
  // can be settled, open or not, but not a subagent's, which is not listed,
  // and not while it is working or waiting for an answer.
  function settleSession(input: Commands["settle_session"]["input"]): void {
    if (projects.projectOf(input.session) === undefined) throw new Error(`There is no session with the id "${input.session}".`)
    const parent = projects.parentOf(input.session)
    if (parent !== undefined) throw new Error(`The session "${input.session}" is a subagent's, which is not listed. Settle the session that started it, "${parent}", instead.`)
    if (typeof input.settled !== "boolean") throw new Error("`settled` is true to settle the session, or false to bring it back.")
    // What goes on in it next is activity, which would make it active again.
    const state = summaries.state(input.session)
    if (input.settled && (state === "working" || state === "needs_you")) {
      throw new Error("This session is working, or waiting for an answer to a question. Wait until it has finished, then settle it.")
    }
    emit(log.append(input.session, "settled", { settled: input.settled }))
  }

  function readHistory(input: Commands["read_history"]["input"]): Commands["read_history"]["output"] {
    if (projects.projectOf(input.session) === undefined) throw new Error(`There is no session with the id "${input.session}".`)
    const limit = input.limit ?? PAGE
    if (!Number.isInteger(limit) || limit < 1 || limit > LONGEST_PAGE) throw new Error(`\`limit\` is how many events to read, a whole number from 1 to ${LONGEST_PAGE}.`)
    if (input.before !== undefined && !Number.isInteger(input.before)) throw new Error("`before` is the number of an event, and the page holds the events before it.")
    return log.page(input.session, input.before ?? Number.MAX_SAFE_INTEGER, limit)
  }

  async function runCommand(input: Commands["run_command"]["input"]): Promise<Commands["run_command"]["output"]> {
    if (typeof input.name !== "string" || typeof input.args !== "string") {
      throw new Error("`name` is the command's name and `args` what was typed after it, and both must be text.")
    }
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

  // Opens a session again in this process, to pick up what was left in it.
  // A subagent's is opened by the session that started it, which is opened first.
  async function reopen(session: string): Promise<void> {
    const parent = projects.parentOf(session)
    if (parent === undefined) {
      await openSession({ session }, true)
      return
    }
    await reopen(parent)
    open(parent).child(session)
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
        case "settle_session":
          return settleSession(command) as Output
        case "read_history":
          return readHistory(command) as Output
        case "list_projects":
          return projects.list() as Output
        case "list_sessions":
          return summaries.list() as Output
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
    async resumeWork() {
      const left = new Set([...log.cutOff(), ...jobs.left().map((record) => record.session)])
      // Each on its own, so that one that cannot be opened again, such as
      // one whose project has gone, does not hold up the others.
      await Promise.all([...left].map((session) => reopen(session).catch(() => {})))
    },
    async close(how = {}) {
      closed = true
      clearInterval(quietCheck)
      if (how.restart) {
        // Nothing more is written from here: the log is left as a crash
        // would leave it, for the next start to pick up.
        jobs.leave()
        db.close()
        return
      }
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
