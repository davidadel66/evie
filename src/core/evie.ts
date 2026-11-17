import { mkdir } from "node:fs/promises"
import { homedir } from "node:os"
import { join } from "node:path"
import { createHost } from "./host.ts"
import { createInbox } from "./inbox.ts"
import { createEventLog } from "./log.ts"
import { createLoop } from "./loop.ts"
import { openStore } from "./store.ts"
import type { Evie, EvieEvent, Plugin } from "./types.ts"

export type EvieOptions = {
  // Settings from the environment. `EVIE_HOME` names the data folder.
  env: Record<string, string | undefined>
  // The project the session works in.
  projectFolder: string
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
export async function createEvie(options: EvieOptions): Promise<Evie> {
  const host = await createHost(options.plugins)
  const provider = host.providers.list()[0]
  if (!provider) throw new Error("No provider is registered. Load a plugin that registers one.")

  const home = dataFolder(options.env)
  await mkdir(home, { recursive: true })
  const db = openStore(join(home, "evie.db"))
  const log = createEventLog(db)
  const inbox = createInbox(db)

  const listeners = new Set<(event: EvieEvent) => void>()
  const emit = (event: EvieEvent) => {
    for (const listener of [...listeners]) {
      // A client that throws while handling an event must not break the turn.
      try {
        listener(event)
      } catch {}
    }
  }

  const session = log.createSession(options.projectFolder)
  const loop = createLoop({
    session,
    model: options.model,
    log,
    inbox,
    host,
    provider,
    atomically: (work) => db.transaction(work)(),
    emit,
  })

  return {
    async send(command) {
      inbox.add(session, { text: command.text })
      loop.wake()
    },
    subscribe(listener) {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
    async close() {
      await loop.stop()
      db.close()
    },
  }
}
