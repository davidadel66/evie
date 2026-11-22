import { expect, test } from "bun:test"
import { RESTART, startedAgainBy, watchOver, type Watched } from "../src/programs/restart.ts"
import type { StopSignal } from "../src/programs/common.ts"

// What starts `evie serve` again after a restart, and the process that
// watches over it when it was run by hand, apart from real processes.

test("the server knows it is started again by the login service only when launchd started it as that service, and by a watcher only when that is its parent", () => {
  const label = "com.evie.serve"
  expect(startedAgainBy({ ppid: 1, env: { XPC_SERVICE_NAME: label } })).toBe("login service")
  // A process whose parent went is given to process 1 too.
  expect(startedAgainBy({ ppid: 1, env: { XPC_SERVICE_NAME: "0" } })).toBeUndefined()
  // A command the service's server ran keeps the name, under another parent.
  expect(startedAgainBy({ ppid: 4242, env: { XPC_SERVICE_NAME: label } })).toBeUndefined()
  expect(startedAgainBy({ ppid: 4242, env: { EVIE_WATCHED_BY: "4242" } })).toBe("watcher")
  expect(startedAgainBy({ ppid: 5151, env: { EVIE_WATCHED_BY: "4242" } })).toBeUndefined()
})

// A stand-in for the servers a watcher starts: each records the signals it
// is sent, and exits when the test says, with the code it says.
function servers() {
  const started: { signals: StopSignal[]; exit(code: number): void }[] = []
  let signal: (sent: StopSignal) => void = () => {}
  const start = (): Watched => {
    let exit = (_code: number) => {}
    const exited = new Promise<number>((resolve) => (exit = resolve))
    const server = { signals: [] as StopSignal[], exit }
    started.push(server)
    return { kill: (sent) => server.signals.push(sent), exited }
  }
  return { started, start, signals: { on: (listener: (sent: StopSignal) => void) => (signal = listener) }, send: (sent: StopSignal) => signal(sent) }
}
const tick = () => Bun.sleep(5)

test("a watcher starts the server again each time it stops for a restart, passes a stop signal on, and ends with its exit code", async () => {
  const run = servers()
  const watched = watchOver(run.start, run.signals)
  await tick()
  run.started[0]!.exit(RESTART)
  await tick()
  expect(run.started).toHaveLength(2)

  run.send("SIGINT")
  expect(run.started[1]!.signals).toEqual(["SIGINT"])
  run.started[1]!.exit(130)
  expect(await watched).toBe(130)
})

test("a stop signal that comes while no server is running is kept: the watcher ends with its code, and starts none", async () => {
  const run = servers()
  const watched = watchOver(run.start, run.signals)
  await tick()
  // The server stops for a restart just as Ctrl-C comes, before it has gone.
  run.send("SIGINT")
  run.started[0]!.exit(RESTART)

  expect(await watched).toBe(130)
  expect(run.started).toHaveLength(1)
})
