import { connect, startServer, type ConnectOptions, type Evie, type EvieOptions, type Plugin } from "../../src/index.ts"
import { openTokens } from "../../src/server/tokens.ts"
import type { WebApp } from "../../src/server/web-app.ts"
import { startEvie } from "./harness.ts"

export type Served = Awaited<ReturnType<typeof serveEvie>>

// Starts Evie behind its server on a free port, with one device token made,
// as `evie serve` and `evie token add` would. Tokens are made through a
// database handle of their own, the way the token commands open it.
export async function serveEvie(
  plugins: Plugin[],
  options: {
    host?: string
    origins?: string[]
    recheckEvery?: number
    keepaliveEvery?: number
    questions?: boolean
    // A project folder to use in place of a new temporary one. The caller removes it.
    projectFolder?: string
    projectSettings?: NonNullable<EvieOptions["projectSettings"]>
    dashboardsSetting?: NonNullable<EvieOptions["dashboardsSetting"]>
    passLasts?: number
    app?: WebApp
    signInCodeLasts?: number
    // What opens a project's folder, in place of the computer's own.
    openFolder?: NonNullable<EvieOptions["openFolder"]>
    compaction?: NonNullable<EvieOptions["compaction"]>
    // What the server is given in place of the core, made from it, such as
    // one that answers a command wrongly, as a server with a fault would.
    core?: (evie: Evie) => Evie
    // A data folder to use in place of a new temporary one, for a plugin
    // made with it. The caller removes it.
    home?: string
  } = {},
) {
  const run = await startEvie(plugins, {
    ...(options.home && { home: options.home }),
    ...(options.questions && { questions: true }),
    ...(options.compaction && { compaction: options.compaction }),
    ...(options.projectFolder && { projectFolder: options.projectFolder }),
    ...(options.projectSettings && { projectSettings: options.projectSettings }),
    ...(options.dashboardsSetting && { dashboardsSetting: options.dashboardsSetting }),
    ...(options.openFolder && { openFolder: options.openFolder }),
  })
  const tokens = await openTokens(run.home)
  const made = tokens.add("test device")
  if (!("secret" in made)) throw new Error(made.refused)
  const server = startServer({
    evie: options.core ? options.core(run.evie) : run.evie,
    tokens,
    host: options.host ?? "127.0.0.1",
    port: 0,
    ...(options.origins && { origins: options.origins }),
    ...(options.recheckEvery !== undefined && { recheckEvery: options.recheckEvery }),
    ...(options.keepaliveEvery !== undefined && { keepaliveEvery: options.keepaliveEvery }),
    ...(options.app && { app: options.app }),
    ...(options.signInCodeLasts !== undefined && { signInCodeLasts: options.signInCodeLasts }),
    ...(options.passLasts !== undefined && { passLasts: options.passLasts }),
    attachments: run.evie.attachments,
    storage: run.evie.storage,
    dashboards: run.evie.dashboards,
    dataFolder: run.home,
  })
  const clients: { close(): Promise<void> }[] = []
  return {
    run,
    tokens,
    server,
    token: made.secret,
    // Connects a client, with this device's token unless another is given.
    async connect(given: Partial<ConnectOptions> = {}) {
      const client = await connect({ url: server.url, token: made.secret, ...given })
      clients.push(client)
      return client
    },
    async stop() {
      for (const client of clients) await client.close()
      await server.stop()
      tokens.close()
      await run.stop()
    },
  }
}
