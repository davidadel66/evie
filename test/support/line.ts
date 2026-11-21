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
  // For each client: its own connection to the server, the commands it
  // sent, by id, so that the reply to one can be lost or held back, and
  // whether the connection has gone silent.
  type Ends = { upstream: WebSocket; sent: Map<string, string>; frozen: boolean }
  const clients = new Set<ServerWebSocket<Ends>>()
  let listening: ReturnType<typeof Bun.serve<Ends>> | undefined
  let port = 0
  // What to do with the next reply to a command of a given type.
  const lose = new Map<string, () => void>()
  const hold = new Map<string, { until: Promise<unknown>; caught(): void }>()
  // Waiting for a message with this text in it to go through, either way.
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

  const saw = (text: string) => {
    for (const watch of [...watching]) {
      if (!text.includes(watch.text)) continue
      watching.delete(watch)
      watch.seen()
    }
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
        if (bun.upgrade(request, { data: { upstream, sent: new Map(), frozen: false } })) return undefined
        upstream.close()
        return new Response("Connect here with a WebSocket.", { status: 400 })
      },
      websocket: {
        open(client) {
          clients.add(client)
          const { upstream, sent } = client.data
          // While a reply is held back, it and everything after it, in order.
          let queued: string[] | undefined
          const pass = (text: string) => {
            client.send(text)
            saw(text)
          }
          upstream.addEventListener("message", (message) => {
            if (client.data.frozen) return
            const text = String(message.data)
            if (queued) return void queued.push(text)
            const received = JSON.parse(text) as ServerMessage
            const type = "reply" in received ? sent.get(received.reply) : undefined
            const losing = type === undefined ? undefined : lose.get(type)
            if (losing && type !== undefined) {
              lose.delete(type)
              return void cut().then(losing)
            }
            const held = type === undefined ? undefined : hold.get(type)
            if (held && type !== undefined) {
              hold.delete(type)
              const waiting = (queued = [text])
              held.caught()
              return void held.until.then(() => {
                queued = undefined
                for (const text of waiting) pass(text)
              })
            }
            pass(text)
          })
          upstream.addEventListener("close", (event) => client.close(event.code === 1006 || event.code === 1005 ? 1000 : event.code, event.reason))
        },
        message(client, text) {
          if (client.data.frozen) return
          const message = JSON.parse(String(text)) as ClientMessage
          client.data.sent.set(message.id, message.command.type)
          client.data.upstream.send(String(text))
          saw(String(text))
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
    // Every connection through the line goes silent at once, both ways,
    // without closing, as when a laptop wakes on another network and its
    // old connections are dead. New connections go through as usual.
    freeze() {
      for (const client of clients) client.data.frozen = true
    },
    // The reply to the next command of this type is lost: the line is cut
    // as it comes back, after the server has carried the command out.
    // Settles once that has happened.
    loseReplyTo(type: string) {
      return new Promise<void>((lost) => lose.set(type, lost))
    },
    // The reply to the next command of this type is held back until `until`
    // settles, and so is everything the server sends after it on that
    // connection, as on a slow network: a WebSocket keeps its messages in
    // order. Settles once the reply has reached the line and is being held.
    holdReplyTo(type: string, until: Promise<unknown>) {
      return new Promise<void>((caught) => hold.set(type, { until, caught }))
    },
    // Settles once a message with this text in it has gone through the
    // line, from the server or from a client.
    passedOn(text: string) {
      return new Promise<void>((seen) => watching.add({ text, seen }))
    },
    stop: cut,
  }
}
