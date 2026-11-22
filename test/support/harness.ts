import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createEvie, type Evie, type EvieEvent, type EvieOptions, type Plugin, type StoredEvent } from "../../src/index.ts"

export type Run = Awaited<ReturnType<typeof startEvie>>

// Starts Evie the way a client would and collects every event the client is
// sent. Each run gets its own temporary data folder, unless one is passed in
// to stand for Evie being started again, and a temporary project folder with
// one session already open in it.
export async function startEvie(
  plugins: Plugin[],
  options: {
    model?: string
    home?: string
    // A project folder to use, to stand for Evie being started again in the same project.
    projectFolder?: string
    projectSettings?: NonNullable<EvieOptions["projectSettings"]>
    trust?: NonNullable<EvieOptions["trust"]>
    compaction?: NonNullable<EvieOptions["compaction"]>
    quietCheckEvery?: number
    // Open the session as a client that shows questions and answers them.
    questions?: boolean
  } = {},
) {
  const home = options.home ?? (await mkdtemp(join(tmpdir(), "evie-home-")))
  const projectFolder = options.projectFolder ?? (await mkdtemp(join(tmpdir(), "evie-project-")))
  const remove = async () => {
    if (!options.home) await rm(home, { recursive: true, force: true })
    if (!options.projectFolder) await rm(projectFolder, { recursive: true, force: true })
  }
  const evie = await createEvie({
    env: { EVIE_HOME: home },
    model: options.model ?? "test/model",
    plugins,
    ...(options.projectSettings && { projectSettings: options.projectSettings }),
    ...(options.trust && { trust: options.trust }),
    ...(options.compaction && { compaction: options.compaction }),
    ...(options.quietCheckEvery !== undefined && { quietCheckEvery: options.quietCheckEvery }),
  }).catch(async (error) => {
    await remove()
    throw error
  })
  const events: EvieEvent[] = []
  evie.subscribe((event) => events.push(event))
  const { session, project } = await evie.send({ type: "open_session", folder: projectFolder, ...(options.questions && { questions: true }) })
  return {
    evie,
    events,
    home,
    projectFolder,
    session,
    project,
    async stop() {
      await evie.close()
      await remove()
    },
  }
}

// Sends a message to a session and waits until the turn it starts has ended.
export async function say(run: { evie: Evie; session: string }, text: string, session = run.session) {
  const ended = new Promise<void>((resolve) => {
    const stop = run.evie.subscribe((event) => {
      if (event.type === "turn_end" && event.session === session) {
        stop()
        resolve()
      }
    })
  })
  await run.evie.send({ type: "send_message", session, text })
  await ended
}

// The events that are kept in the log, leaving out the ones only sent to clients.
export const stored = (events: EvieEvent[]) => events.filter((event): event is StoredEvent => "seq" in event)

// Resolves with the next event the client is sent that matches.
export function nextEvent<Type extends EvieEvent["type"]>(run: { evie: Evie }, type: Type) {
  return new Promise<Extract<EvieEvent, { type: Type }>>((resolve) => {
    const stop = run.evie.subscribe((event) => {
      if (event.type !== type) return
      stop()
      resolve(event as Extract<EvieEvent, { type: Type }>)
    })
  })
}
