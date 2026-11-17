import { mkdir } from "node:fs/promises"
import { homedir } from "node:os"
import { join } from "node:path"
import { createHost } from "./host.ts"
import { createInbox } from "./inbox.ts"
import { createEventLog } from "./log.ts"
import { createLoop, type Loop } from "./loop.ts"
import { createProjects } from "./projects.ts"
import { openStore } from "./store.ts"
import type { Commands, Evie, EvieEvent, Plugin, Provider } from "./types.ts"

export type EvieOptions = {
  // Settings from the environment. `EVIE_HOME` names the data folder.
  env: Record<string, string | undefined>
  // The name of the model to ask.
  model: string
  // The plugins to load, in order.
  plugins: readonly Plugin[]
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
  const host = await createHost(options.plugins)
  const registered = host.providers.list()[0]
  if (!registered) throw new Error("No provider is registered. Load a plugin that registers one.")
  const provider: Provider = registered

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

  function openSession(input: Commands["open_session"]["input"]): Commands["open_session"]["output"] {
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

    const session = projects.createSession(project)
    loops.set(
      session,
      createLoop({
        session,
        model: options.model,
        log,
        inbox,
        host,
        provider,
        atomically: (work) => db.transaction(work)(),
        emit,
      }),
    )
    return { session, project }
  }

  function sendMessage(input: Commands["send_message"]["input"]): void {
    const loop = loops.get(input.session)
    if (!loop) throw new Error(`The session "${input.session}" is not open.`)
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
          return openSession(command) as Output
        case "send_message":
          return sendMessage(command) as Output
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
