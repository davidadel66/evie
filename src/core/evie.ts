import { mkdir } from "node:fs/promises"
import { homedir } from "node:os"
import { join } from "node:path"
import { createHost, type HookRunner } from "./host.ts"
import { createInbox } from "./inbox.ts"
import { createEventLog } from "./log.ts"
import { createLoop, type Loop } from "./loop.ts"
import { createProjects } from "./projects.ts"
import { openStore } from "./store.ts"
import type { Commands, Evie, EvieEvent, Host, Plugin, Project, Provider } from "./types.ts"

export type EvieOptions = {
  // Settings from the environment. `EVIE_HOME` names the data folder.
  env: Record<string, string | undefined>
  // The name of the model to ask.
  model: string
  // The global plugins, in order. They are part of every project and of the
  // general session.
  plugins: readonly Plugin[]
  // A project's own plugins, added after the global ones for that project only.
  projectPlugins?: (project: Project) => readonly Plugin[] | Promise<readonly Plugin[]>
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
  type Scope = { host: Host; hooks: HookRunner; provider: Provider }
  const scopes = new Map<string | null, Promise<Scope>>()

  async function buildScope(project: Project | null): Promise<Scope> {
    const own = project && options.projectPlugins ? await options.projectPlugins(project) : []
    const { host, hooks } = await createHost([...options.plugins, ...own], project)
    const provider = host.providers.list()[0]
    if (!provider) throw new Error("No provider is registered. Load a plugin that registers one.")
    return { host, hooks, provider }
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

  // One turn loop for each session open in this process.
  const loops = new Map<string, Loop>()

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

    const { host, hooks, provider } = await scopeFor(project)
    const session = projects.createSession(project)
    loops.set(
      session,
      createLoop({
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
      }),
    )
    return { session, project }
  }

  function openLoop(session: string): Loop {
    const loop = loops.get(session)
    if (!loop) throw new Error(`The session "${session}" is not open.`)
    return loop
  }

  function sendMessage(input: Commands["send_message"]["input"]): void {
    const loop = openLoop(input.session)
    inbox.add(input.session, { text: input.text })
    loop.wake()
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
          return openLoop(command.session).cancel() as Output
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
      await Promise.all([...loops.values()].map((loop) => loop.stop()))
      db.close()
    },
  }
}
