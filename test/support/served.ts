import { connect, startServer, type ConnectOptions, type EvieOptions, type Plugin } from "../../src/index.ts"
import { openTokens } from "../../src/server/tokens.ts"
import { startEvie } from "./harness.ts"

export type Served = Awaited<ReturnType<typeof serveEvie>>

// Starts Evie behind its server on a free port, with one device token made,
// as `evie serve` and `evie token add` would. Tokens are made through a
// database handle of their own, the way the token commands open it.
export async function serveEvie(
  plugins: Plugin[],
  options: { origins?: string[]; recheckEvery?: number; questions?: boolean; projectSettings?: NonNullable<EvieOptions["projectSettings"]> } = {},
) {
  const run = await startEvie(plugins, { ...(options.questions && { questions: true }), ...(options.projectSettings && { projectSettings: options.projectSettings }) })
  const tokens = await openTokens(run.home)
  const made = tokens.add("test device")
  if (!("secret" in made)) throw new Error(made.refused)
  const server = startServer({
    evie: run.evie,
    tokens,
    host: "127.0.0.1",
    port: 0,
    ...(options.origins && { origins: options.origins }),
    ...(options.recheckEvery !== undefined && { recheckEvery: options.recheckEvery }),
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
