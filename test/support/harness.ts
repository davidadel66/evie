import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createEvie, type Evie, type EvieEvent, type Plugin } from "../../src/index.ts"

// Starts Evie the way a client would, with its own temporary data folder and
// project folder, and collects every event the client is sent.
export async function startEvie(plugins: Plugin[], model = "test/model") {
  const home = await mkdtemp(join(tmpdir(), "evie-home-"))
  const projectFolder = await mkdtemp(join(tmpdir(), "evie-project-"))
  const remove = async () => {
    await rm(home, { recursive: true, force: true })
    await rm(projectFolder, { recursive: true, force: true })
  }
  const evie = await createEvie({ env: { EVIE_HOME: home }, projectFolder, model, plugins }).catch(async (error) => {
    await remove()
    throw error
  })
  const events: EvieEvent[] = []
  evie.subscribe((event) => events.push(event))
  return {
    evie,
    events,
    home,
    projectFolder,
    async stop() {
      await evie.close()
      await remove()
    },
  }
}

// Sends a message and waits until the turn it starts has ended.
export async function say(evie: Evie, text: string) {
  const ended = new Promise<void>((resolve) => {
    const stop = evie.subscribe((event) => {
      if (event.type === "turn_end") {
        stop()
        resolve()
      }
    })
  })
  await evie.send({ type: "send_message", text })
  await ended
}

export const stored = (events: EvieEvent[]) => events.filter((event) => event.type !== "assistant_delta")
