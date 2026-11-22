import type { CheckedToolCall, HookContext, Hooks, Host, Plugin, Project, Registry, Request } from "./types.ts"

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

// How the core runs the hooks plugins attached. Each runs every handler for
// its hook in the order the plugins were listed.
export type HookRunner = {
  // The request as the hooks left it.
  beforeRequest(request: Request, context: HookContext): Promise<Request>
  // The reason a hook denied the call, or undefined when none did.
  beforeTool(call: CheckedToolCall, context: HookContext): Promise<string | undefined>
  // The result text as the hooks left it.
  afterTool(call: CheckedToolCall, text: string, context: HookContext): Promise<string>
  // Tells every plugin that keeps files for sessions that one was deleted.
  // A plugin that breaks there does not keep the others from being told:
  // the first failure is thrown once they all have been.
  sessionDeleted(session: string): Promise<void>
}

type Attached = { [Name in keyof Hooks]: { plugin: string; handler: Hooks[Name] }[] }

// The plugin host for one project, or for the general session: the lists
// plugins add to and the hooks they attach to. It calls each plugin's setup,
// in the order given, and passes it the host.
export async function createHost(
  plugins: readonly Plugin[],
  project: Project | null,
): Promise<{ host: Host; hooks: HookRunner }> {
  const attached: Attached = { before_request: [], before_tool: [], after_tool: [], session_deleted: [] }
  const shared = {
    project,
    tools: createRegistry("tool"),
    providers: createRegistry("provider"),
    profiles: createRegistry("model profile"),
    commands: createRegistry("command"),
    jobKinds: createRegistry("job kind"),
  } satisfies Omit<Host, "on">

  // Every plugin shares the same lists, but each is handed its own copy of
  // the host, so a hook is known by the plugin that attached it.
  const hostFor = (plugin: string): Host => ({
    ...shared,
    on(name, handler) {
      ;(attached[name] as { plugin: string; handler: unknown }[]).push({ plugin, handler })
    },
  })

  for (const plugin of plugins) {
    try {
      await plugin.setup(hostFor(plugin.name))
    } catch (cause) {
      throw new Error(`The plugin "${plugin.name}" could not be set up: ${reason(cause)}`)
    }
  }

  // Runs one plugin's handler. An error inside it is a bug in that plugin.
  async function call<Result>(hook: keyof Hooks, plugin: string, run: () => Result | Promise<Result>): Promise<Result> {
    try {
      return await run()
    } catch (cause) {
      throw new Error(`The plugin "${plugin}" broke in its ${hook} hook: ${reason(cause)}`)
    }
  }

  const hooks: HookRunner = {
    async beforeRequest(request, context) {
      for (const { plugin, handler } of attached.before_request) {
        request = (await call("before_request", plugin, () => handler(request, context))) ?? request
      }
      return request
    },
    async beforeTool(toolCall, context) {
      for (const { plugin, handler } of attached.before_tool) {
        const answer = await call("before_tool", plugin, () => handler(toolCall, context))
        // Only a reason in words is a denial. Anything else a plugin returns
        // must not cut the later hooks short.
        if (answer && typeof answer.deny === "string") return answer.deny
      }
      return undefined
    },
    async afterTool(toolCall, text, context) {
      for (const { plugin, handler } of attached.after_tool) {
        text = (await call("after_tool", plugin, () => handler(toolCall, text, context))) ?? text
      }
      return text
    },
    async sessionDeleted(session) {
      let failure: unknown
      for (const { plugin, handler } of attached.session_deleted) {
        await call("session_deleted", plugin, () => handler(session)).catch((cause) => (failure ??= cause))
      }
      if (failure !== undefined) throw failure
    },
  }

  return { host: hostFor("the core"), hooks }
}

const reason = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))
