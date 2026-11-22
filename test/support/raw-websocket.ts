import { constants, inflateRawSync } from "node:zlib"

export type RawFrame = {
  // Whether the server compressed the message, as permessage-deflate marks
  // it: the frame's first reserved bit, RSV1.
  compressed: boolean
  // The message's bytes as they crossed the line, compressed or not.
  size: number
  // The message's text, decompressed where it was compressed.
  text: string
}

// A WebSocket client written by hand over TCP, so that a test chooses what
// its handshake offers, as a browser or a client without compression would,
// and sees each message as the server sent it. Only what the tests need:
// text messages, in single frames, of fewer than 126 bytes from the client.
export async function rawWebSocket(serverUrl: string, headers: Record<string, string>) {
  const address = new URL("/ws", serverUrl)
  const frames: RawFrame[] = []
  const waiting = new Set<() => void>()
  const wakeAll = () => {
    for (const wake of [...waiting]) wake()
    waiting.clear()
  }
  // Why no more messages will come, once none will.
  let ended: Error | undefined
  let head: string | undefined
  let buffer = Buffer.alloc(0)
  // permessage-deflate keeps what it has sent in its window, unless the
  // server says it does not, so each message may point back into earlier
  // ones. Every compressed message so far is then decompressed again
  // together, and the new one is what comes after what the earlier ones made.
  let keepsContext = true
  let compressedSoFar = Buffer.alloc(0)
  let decompressedSoFar = 0

  const read = (chunk: Buffer) => {
    buffer = Buffer.concat([buffer, chunk])
    if (head === undefined) {
      const end = buffer.indexOf("\r\n\r\n")
      if (end < 0) return
      head = buffer.subarray(0, end).toString()
      buffer = buffer.subarray(end + 4)
      keepsContext = !/^sec-websocket-extensions:.*server_no_context_takeover/im.test(head)
    }
    for (;;) {
      if (buffer.length < 2) break
      let length = buffer[1]! & 0x7f
      let at = 2
      if (length === 126) [length, at] = buffer.length < 4 ? [Infinity, 4] : [buffer.readUInt16BE(2), 4]
      else if (length === 127) [length, at] = buffer.length < 10 ? [Infinity, 10] : [Number(buffer.readBigUInt64BE(2)), 10]
      if (buffer.length < at + length) break
      const payload = buffer.subarray(at, at + length)
      const first = buffer[0]!
      buffer = buffer.subarray(at + length)
      const opcode = first & 0x0f
      // A ping or a close is passed over. A message in several frames, which
      // the server does not send, is not read: the tests waiting are told.
      if (opcode >= 0x8) continue
      if (opcode !== 0x1 || (first & 0x80) === 0) {
        ended ??= new Error("The server sent a message in several frames, which this client does not read.")
        break
      }
      const compressed = (first & 0x40) !== 0
      let text = payload.toString()
      if (compressed) {
        if (!keepsContext) [compressedSoFar, decompressedSoFar] = [Buffer.alloc(0), 0]
        compressedSoFar = Buffer.concat([compressedSoFar, payload, Buffer.from([0x00, 0x00, 0xff, 0xff])])
        const decompressed = inflateRawSync(compressedSoFar, { finishFlush: constants.Z_SYNC_FLUSH })
        text = decompressed.subarray(decompressedSoFar).toString()
        decompressedSoFar = decompressed.length
      }
      frames.push({ compressed, size: payload.length, text })
    }
    wakeAll()
  }

  const opened = Promise.withResolvers<void>()
  const socket = await Bun.connect({
    hostname: address.hostname,
    port: Number(address.port),
    socket: {
      data(_socket, chunk) {
        if (ended) return
        read(chunk)
        if (head !== undefined) opened.resolve()
      },
      close() {
        ended ??= new Error("The server closed the connection.")
        opened.reject(ended)
        wakeAll()
      },
    },
  })
  const lines = [
    `GET ${address.pathname} HTTP/1.1`,
    `Host: ${address.host}`,
    "Upgrade: websocket",
    "Connection: Upgrade",
    "Sec-WebSocket-Version: 13",
    `Sec-WebSocket-Key: ${Buffer.from(crypto.getRandomValues(new Uint8Array(16))).toString("base64")}`,
    ...Object.entries(headers).map(([name, value]) => `${name}: ${value}`),
  ]
  socket.write(`${lines.join("\r\n")}\r\n\r\n`)
  await opened.promise
  if (!head!.startsWith("HTTP/1.1 101")) throw new Error(`The server did not open a WebSocket: ${head}`)

  return {
    // What the server agreed to, from its `Sec-WebSocket-Extensions`, or
    // nothing.
    extensions: /^sec-websocket-extensions: *(.*)$/im.exec(head!)?.[1]?.trim() ?? "",
    // Every message the server has sent, in order.
    frames,
    // Settles with the first message so far, or still to come, that matches,
    // or fails, saying why, once no more messages will come.
    async next(matches: (frame: RawFrame) => boolean): Promise<RawFrame> {
      for (;;) {
        const found = frames.find(matches)
        if (found) return found
        if (ended) throw ended
        await new Promise<void>((wake) => waiting.add(wake))
      }
    },
    // Sends a text message, masked, as a client must.
    send(text: string) {
      const payload = Buffer.from(text)
      if (payload.length >= 126) throw new Error("This client sends only messages of fewer than 126 bytes.")
      const mask = crypto.getRandomValues(new Uint8Array(4))
      socket.write(Buffer.concat([Buffer.from([0x81, 0x80 | payload.length]), mask, payload.map((byte, at) => byte ^ mask[at % 4]!)]))
    },
    close() {
      socket.end()
    },
  }
}
