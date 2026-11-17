import type { Host, Plugin, Registry } from "./types.ts"

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

// The plugin host: the lists plugins add to. It calls each plugin's setup, in
// the order given, and passes it the host.
export async function createHost(plugins: readonly Plugin[]): Promise<Host> {
  const host: Host = {
    tools: createRegistry("tool"),
    providers: createRegistry("provider"),
  }
  for (const plugin of plugins) await plugin.setup(host)
  return host
}
