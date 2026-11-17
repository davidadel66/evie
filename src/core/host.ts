import type { Host, Plugin, Project, Registry } from "./types.ts"

function createRegistry<Entry extends { name: string }>(kind: string): Registry<Entry> {
  const entries: Entry[] = []
  return {
    add(entry) {
      if (entries.some((existing) => existing.name === entry.name)) {
        throw new Error(`A ${kind} named "${entry.name}" is already registered.`)
      }
      entries.push(entry)
    },
    list() {
      return entries
    },
  }
}

// The plugin host for one project, or for the general session: the lists
// plugins add to. It calls each plugin's setup, in the order given, and
// passes it the host.
export async function createHost(plugins: readonly Plugin[], project: Project | null): Promise<Host> {
  const host: Host = {
    project,
    tools: createRegistry("tool"),
    providers: createRegistry("provider"),
  }
  for (const plugin of plugins) {
    try {
      await plugin.setup(host)
    } catch (cause) {
      const reason = cause instanceof Error ? cause.message : String(cause)
      throw new Error(`The plugin "${plugin.name}" could not be set up: ${reason}`)
    }
  }
  return host
}
