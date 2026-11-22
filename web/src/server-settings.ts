import type { Evie, SettingsChange, SettingsView } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"

// The settings kept on evie's server, as the settings window shows them,
// apart from its screen, which is told them and redrawn as they change.
// They are read each time the page connects, and again after each change.

export type SettingsState = {
  // The settings as the server last gave them, or undefined until it has.
  view: SettingsView | undefined
  // Why they could not be read, in words for the owner, or undefined.
  problem: string | undefined
  // From when a restart is asked for until the server started in its place
  // has answered.
  restarting: boolean
}

export type ServerSettings = {
  // The settings now. It is the same object until they change.
  state(): SettingsState
  // Calls the listener each time they change, until the function returned
  // is called.
  subscribe(listener: () => void): () => void
  // Makes one change on the server. Gives back why the server refused it,
  // in its words, or nothing.
  change(change: SettingsChange): Promise<string | undefined>
  // Asks the server to start again. Gives back why it refused, or nothing.
  restart(): Promise<string | undefined>
  // Stops following the connection.
  close(): void
}

export function openServerSettings(connection: Connection): ServerSettings {
  let state: SettingsState = { view: undefined, problem: undefined, restarting: false }
  const listeners = new Set<() => void>()
  // When the server that was asked to restart started.
  let restartOf: number | undefined

  function set(next: Partial<SettingsState>) {
    state = { ...state, ...next }
    for (const listener of [...listeners]) listener()
  }

  // A view from the server asked to restart, which can answer once more
  // before it stops, leaves the restart under way.
  function show(view: SettingsView) {
    const restarting = restartOf !== undefined && view.started === restartOf
    if (!restarting) restartOf = undefined
    set({ view, problem: undefined, restarting })
  }

  function read(evie: Evie) {
    void evie.send({ type: "read_settings" }).then(show, (cause: unknown) => set({ problem: `The settings could not be read: ${wordsOf(cause)}` }))
  }

  // Read each time the connection is made, as when it comes back after the
  // server has started again. Changes are sent on the last client
  // connected, so that one asked for while the line is down waits for it,
  // as the client's commands do.
  let connected: Evie | undefined
  let client: Evie | undefined
  function follow() {
    const now = connection.state()
    const evie = now.status === "connected" ? now.evie : undefined
    if (evie !== undefined && evie !== connected) read(evie)
    connected = evie
    if (evie !== undefined) client = evie
  }
  const unfollow = connection.subscribe(follow)
  follow()

  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    async change(change) {
      const evie = client
      if (evie === undefined) return NOT_CONNECTED
      try {
        show(await evie.send({ type: "change_settings", ...change }))
        return undefined
      } catch (cause) {
        return wordsOf(cause)
      }
    },
    async restart() {
      const evie = client
      if (evie === undefined || state.view === undefined) return NOT_CONNECTED
      const started = state.view.started
      restartOf = started
      set({ restarting: true })
      try {
        await evie.send({ type: "restart_server", started })
        return undefined
      } catch (cause) {
        restartOf = undefined
        set({ restarting: false })
        return wordsOf(cause)
      }
    },
    close() {
      unfollow()
    },
  }
}

const NOT_CONNECTED = "Not connected to evie's server just now. Wait until it is connected."
const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))

// ---- How each setting stands, in words -------------------------------------------------------

// A setting's state in a word or two, and, when there is more to know, a
// note saying it: most often that a change waits for a restart.
export type Standing = { word: string; note?: string }

const ONCE_STARTED = "once evie's server starts again."

export function keyState(key: SettingsView["keys"][number]): Standing {
  const word = key.set ? "Set" : "Not set"
  if (key.environment) return { word, note: "The environment evie's server runs in sets it too, and wins over this." }
  return key.pending ? { word, note: `Applies ${ONCE_STARTED}` } : { word }
}

export function pluginState(plugin: SettingsView["plugins"][number]): Standing {
  if (!plugin.listed) return { word: "Removed", note: `Still loaded until evie's server starts again.` }
  return plugin.loaded ? { word: "Loaded" } : { word: "Not loaded yet", note: `Loads ${ONCE_STARTED}` }
}

export function projectState(project: SettingsView["projects"][number]): Standing {
  const word = project.trusted ? "Trusted" : "Not trusted"
  return project.pending ? { word, note: "Its open sessions keep their settings until evie's server starts again." } : { word }
}
