import type { ServerWebSocket } from "bun"
import type { ClientMessage, ServerMessage } from "../../src/index.ts"

export type Line = Awaited<ReturnType<typeof lineTo>>

// Stands for the network between clients and Evie's server, so that a test
// can cut the line and mend it while the server runs on, as when a laptop
// sleeps. Clients connect to `url` in place of the server's address. Every
// message is passed on both ways, and every other request is passed on to
// the server as it is.
export async function lineTo(serverUrl: string) {
  const server = new URL(serverUrl)
  // For each client: its own connection to the server, and the commands it
  // sent, by id, so that the reply to one can be lost or held back.
  type Ends = { upstream: WebSocket; sent: Map<string, string> }
  const clients = new Set<ServerWebSocket<Ends>>()
  let listening: ReturnType<typeof Bun.serve<Ends>> | undefined
  let port = 0
  // What to do with the next reply to a command of a given type.
  const lose = new Set<string>()
  const hold = new Map<string, { until: Promise<unknown>; caught(): void }>()
  // Waiting for a message from the server with this text in it to be passed on.
  const watching = new Set<{ text: string; seen(): void }>()

  // Only what the server reads of a request is passed on.
  const headersOf = (request: Request) => {
    const kept: Record<string, string> = {}
    for (const name of ["authorization", "cookie", "origin"]) {
      const value = request.headers.get(name)
      if (value !== null) kept[name] = value
    }
    return kept
  }

  const listen = () => {
    listening = Bun.serve<Ends>({
      hostname: "127.0.0.1",
      port,
      async fetch(request, bun) {
        const path = new URL(request.url).pathname
        const to = new URL(path, server)
        if (path !== "/ws") return fetch(to, { method: request.method, headers: headersOf(request), body: await request.text() || null })
        // The server is reached first, so that a connection it refuses is
        // refused here too, saying why.
        to.protocol = "ws:"
        // The server sends nothing until it is sent a command, so nothing is
        // missed before the client's side opens.
        const upstream = new WebSocket(to, { headers: headersOf(request) })
        const opened = await new Promise<boolean>((resolve) => {
          upstream.addEventListener("open", () => resolve(true), { once: true })
          upstream.addEventListener("close", () => resolve(false), { once: true })
        })
        if (!opened) return fetch(new URL("/check", server), { headers: headersOf(request) })
        if (bun.upgrade(request, { data: { upstream, sent: new Map() } })) return undefined
        upstream.close()
        return new Response("Connect here with a WebSocket.", { status: 400 })
      },
      websocket: {
        open(client) {
          clients.add(client)
          const { upstream, sent } = client.data
          upstream.addEventListener("message", (message) => {
            const text = String(message.data)
            const received = JSON.parse(text) as ServerMessage
            const type = "reply" in received ? sent.get(received.reply) : undefined
            if (type !== undefined && lose.delete(type)) return void cut()
            const held = type === undefined ? undefined : hold.get(type)
            if (held && type !== undefined) {
              hold.delete(type)
              held.caught()
              return void held.until.then(() => client.send(text))
            }
            client.send(text)
            for (const watch of [...watching]) {
              if (!text.includes(watch.text)) continue
              watching.delete(watch)
              watch.seen()
            }
          })
          upstream.addEventListener("close", (event) => client.close(event.code === 1006 || event.code === 1005 ? 1000 : event.code, event.reason))
        },
        message(client, text) {
          const message = JSON.parse(String(text)) as ClientMessage
          client.data.sent.set(message.id, message.command.type)
          client.data.upstream.send(String(text))
        },
        close(client) {
          clients.delete(client)
          client.data.upstream.close()
        },
      },
    })
    port = listening.port!
  }

  // Every connection through the line ends at once, and nothing can connect
  // until it is mended.
  async function cut() {
    const was = listening
    listening = undefined
    for (const client of clients) client.data.upstream.close()
    await was?.stop(true)
  }

  listen()
  return {
    url: `http://127.0.0.1:${port}`,
    cut,
    // Listens again, on the same address.
    mend() {
      if (!listening) listen()
    },
    // The reply to the next command of this type is lost: the line is cut
    // as it comes back, after the server has carried the command out.
    loseReplyTo(type: string) {
      lose.add(type)
    },
    // The reply to the next command of this type is held back until `until`
    // settles, while everything else the server sends goes on through.
    // Settles once the reply has reached the line and is being held.
    holdReplyTo(type: string, until: Promise<unknown>) {
      return new Promise<void>((caught) => hold.set(type, { until, caught }))
    },
    // Settles once a message from the server with this text in it has been
    // passed on to a client.
    passedOn(text: string) {
      return new Promise<void>((seen) => watching.add({ text, seen }))
    },
    stop: cut,
  }
}
