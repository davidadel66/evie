// Small web apps for the dashboard tests: a start command that runs one, a
// port for it, and whether it answers.

// A port on this computer that nothing is listening on, as an app's own.
export function freePort(): number {
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: () => new Response("ok") })
  const port = server.port!
  server.stop(true)
  return port
}

// A command that runs an app answering "ok" on `port`, once `after`
// milliseconds have passed.
export function appCommand(port: number, after = 0): string {
  const listen = `Bun.serve({ hostname: "127.0.0.1", port: ${port}, fetch: () => new Response("ok") });`
  const code = after > 0 ? `setTimeout(() => { ${listen} }, ${after});` : listen
  return `${JSON.stringify(process.execPath)} -e ${JSON.stringify(code)}`
}

// Whether anything answers at an address.
export async function answers(url: string): Promise<boolean> {
  try {
    const answer = await fetch(url, { redirect: "manual", signal: AbortSignal.timeout(500) })
    await answer.body?.cancel()
    return true
  } catch {
    return false
  }
}

// Whether an address has stopped answering within `within` milliseconds.
export async function untilDown(url: string, within = 3000): Promise<boolean> {
  const until = Date.now() + within
  while (Date.now() < until) {
    if (!(await answers(url))) return true
    await Bun.sleep(25)
  }
  return !(await answers(url))
}
