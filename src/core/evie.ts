import { mkdir } from "node:fs/promises"
import { homedir } from "node:os"
import { join } from "node:path"
import { createHost, type HookRunner } from "./host.ts"
import { createInbox } from "./inbox.ts"
import { createEventLog } from "./log.ts"
import { createLoop, currentModel, type Loop } from "./loop.ts"
import { createProjects } from "./projects.ts"
import { openStore } from "./store.ts"
import type { Commands, Evie, EvieEvent, Host, Plugin, Project, ProjectPlugins, Provider } from "./types.ts"

export type EvieOptions = {
  // Settings from the environment. `EVIE_HOME` names the data folder.
  env: Record<string, string | undefined>
  // The name of the model to ask.
  model: string
  // The global plugins, in order. They are part of every project and of the
  // general session.
  plugins: readonly Plugin[]
  // What a project has on top of the global plugins: its own, added after
  // them for that project only, and any global ones it switches off.
  projectPlugins?: (project: Project) => ProjectPlugins | readonly Plugin[] | Promise<ProjectPlugins | readonly Plugin[]>
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
  type Scope = { host: Host; hooks: HookRunner; provider: Provider; notices: string[] }
  const scopes = new Map<string | null, Promise<Scope>>()

  async function buildScope(project: Project | null): Promise<Scope> {
    const given = project && options.projectPlugins ? await options.projectPlugins(project) : []
    const own: ProjectPlugins = "plugins" in given ? given : { plugins: given }
    const kept = options.plugins.filter((plugin) => !own.disable?.includes(plugin.name))
    const { host, hooks } = await createHost([...kept, ...own.plugins], project)
    const provider = host.providers.list()[0]
    if (!provider) throw new Error("No provider is registered. Load a plugin that registers one.")
    return { host, hooks, provider, notices: [...(own.notices ?? [])] }
  }

  function scopeFor(project: Project | null): Promise<Scope> {
    const key = project?.id ?? null
    let scope = scopes.get(key)
    if (!scope) {
      scope = buildScope(project)
      scopes.set(key, scope)
      // A set that failed to build is forgotten, so the next session tries again.
      scope.catch(() => scopes.delete(key))
    }
    return scope
  }

  await scopeFor(null)

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

  // Each session open in this process: its turn loop, its project and that
  // project's plugin host.
  type Open = { loop: Loop; project: Project | null; host: Host }
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

    const { host, hooks, provider, notices } = await scopeFor(project)

    // Continuing: the session used last in this project, when there is one.
    const last = input.continue ? projects.lastSession(project) : undefined
    if (last !== undefined && sessions.has(last)) return { session: last, project, resumed: true, notices }
    if (last !== undefined) settleUnfinishedTurn(last)

    const session = last ?? projects.createSession(project)
    const loop = createLoop({
      session,
      project,
      outputFolder: join(home, "tool-output"),
      model: options.model,
      log,
      inbox,
      host,
      hooks,
      provider,
      atomically: (work) => db.transaction(work)(),
      emit,
    })
    sessions.set(session, { loop, project, host })
    // A message that was waiting when Evie last stopped is answered now.
    if (last !== undefined) loop.wake()
    return { session, project, resumed: last !== undefined, notices }
  }

  // If Evie was stopped in the middle of a turn, the log ends with a turn
  // that never finished, and perhaps tool calls with no result. This closes
  // it off, so the conversation is one a model will accept again.
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
    const { project, host } = open(input.session)
    const command = host.commands.list().find((candidate) => candidate.name === input.name)
    if (!command) {
      const names = host.commands.list().map((candidate) => `/${candidate.name}`).join(", ") || "none"
      throw new Error(`There is no command "/${input.name}". The commands are: ${names}.`)
    }
    const text = await command.run(input.args, {
      session: input.session,
      project,
      model: currentModel(log.events(input.session), options.model),
      setModel: (model) => setModel({ session: input.session, model }),
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
      await Promise.all([...sessions.values()].map(({ loop }) => loop.stop()))
      db.close()
    },
  }
}
