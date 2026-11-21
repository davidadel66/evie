import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, readFile, realpath, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { dirname, join } from "node:path"
import { runService, type Computer } from "../src/programs/service.ts"

// `evie service`, with a stand-in for the Mac: a temporary folder in place
// of ~/Library/LaunchAgents, and a launchctl that records what it is asked
// and answers the way the real one does. Nothing here touches the real ones.

const CLI = join(import.meta.dir, "..", "src", "cli.ts")
const BUN = process.execPath

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function tempFolder() {
  const folder = await realpath(await mkdtemp(join(tmpdir(), "evie-service-")))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

// A Mac whose launchctl has the service loaded, or not, to begin with.
// `refuses` names a launchctl command that fails, with what it prints, for
// the first `times` it is run, or every time. With `stops`, the server the
// service starts stops at once. Pauses are recorded, not waited for.
async function mac(options: { loaded?: boolean; refuses?: Record<string, { output: string; times?: number }>; stops?: true } = {}) {
  const folder = await tempFolder()
  let loaded = options.loaded ?? false
  const calls: string[] = []
  const waits: number[] = []
  const refused: Record<string, number> = {}
  const computer: Computer = {
    platform: "darwin",
    launchAgents: join(folder, "LaunchAgents"),
    uid: 501,
    async wait(ms) {
      waits.push(ms)
    },
    async launchctl(args) {
      calls.push(args.join(" "))
      const refusal = options.refuses?.[args[0]!]
      if (refusal && (refusal.times === undefined || (refused[args[0]!] ?? 0) < refusal.times)) {
        refused[args[0]!] = (refused[args[0]!] ?? 0) + 1
        return { code: 5, output: refusal.output }
      }
      if (args[0] === "print") {
        if (loaded && options.stops) return { code: 0, output: "gui/501/com.evie.serve = {\n\tstate = not running\n\tlast exit code = 1\n}" }
        return loaded
          ? { code: 0, output: "gui/501/com.evie.serve = {\n\tstate = running\n\tpid = 4242\n}" }
          : { code: 113, output: 'Could not find service "com.evie.serve" in domain for user gui: 501' }
      }
      if (args[0] === "bootstrap") loaded = true
      if (args[0] === "bootout") loaded = false
      return { code: 0, output: "" }
    },
  }
  return { computer, calls, waits, folder, plist: join(folder, "LaunchAgents", "com.evie.serve.plist") }
}

test("install writes a login service that runs this evie's server, and loads it", async () => {
  const { computer, calls, folder, plist } = await mac()
  const home = join(folder, "data & more")

  const done = await runService({ EVIE_HOME: home, PATH: "/usr/bin:/bin" }, ["install"], computer)

  expect(done.ok).toBe(true)
  expect(done.text).toContain(plist)
  expect(done.text).toContain(join(home, "serve.log"))
  expect(await readFile(plist, "utf8")).toBe(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.evie.serve</string>
  <key>ProgramArguments</key>
  <array>
    <string>${BUN}</string>
    <string>${CLI}</string>
    <string>serve</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>EVIE_HOME</key>
    <string>${folder}/data &amp; more</string>
    <key>PATH</key>
    <string>${dirname(BUN)}:/usr/bin:/bin</string>
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>${folder}/data &amp; more/serve.log</string>
  <key>StandardErrorPath</key>
  <string>${folder}/data &amp; more/serve.log</string>
</dict>
</plist>
`)
  expect(calls).toEqual(["print gui/501/com.evie.serve", `bootstrap gui/501 ${plist}`, "print gui/501/com.evie.serve"])
})

test("installing again unloads the older service first, so the server restarts on this version", async () => {
  const { computer, calls, folder, plist } = await mac()
  const env = { EVIE_HOME: join(folder, "data"), PATH: "/usr/bin" }
  await runService(env, ["install"], computer)
  // As an older checkout of Evie would have written it.
  await writeFile(plist, (await readFile(plist, "utf8")).replace(CLI, "/an/older/checkout/src/cli.ts"))
  calls.length = 0

  const done = await runService(env, ["install"], computer)

  expect(done.ok).toBe(true)
  expect(calls).toEqual(["print gui/501/com.evie.serve", "bootout gui/501/com.evie.serve", `bootstrap gui/501 ${plist}`, "print gui/501/com.evie.serve"])
  expect(await readFile(plist, "utf8")).toContain(`<string>${CLI}</string>`)
})

test("install looks again a moment after starting the server, and says it is running only when it is", async () => {
  const up = await mac()
  const started = await runService({ EVIE_HOME: join(up.folder, "data"), PATH: "/usr/bin" }, ["install"], up.computer)
  expect(started.ok).toBe(true)
  expect(started.text).toContain("started the server, which is running as process 4242")
  expect(up.waits).toEqual([2000])

  const down = await mac({ stops: true })
  const stopped = await runService({ EVIE_HOME: join(down.folder, "data"), PATH: "/usr/bin" }, ["install"], down.computer)
  expect(stopped.ok).toBe(false)
  expect(stopped.text).toContain("The login service is installed, but the server is not running")
  expect(stopped.text).toContain(join(down.folder, "data", "serve.log"))
  expect(stopped.text).not.toContain("started the server")
})

test("install names the settings set in this terminal that the service will not see, and never their values", async () => {
  const { computer, folder } = await mac()
  const home = join(folder, "data")
  await mkdir(home, { recursive: true })
  await writeFile(join(home, ".env"), "BRAVE_API_KEY=brave-secret\n")
  await writeFile(join(home, "evie.json"), JSON.stringify({ maxTokens: 8000 }))
  // As `evie` passes them on: the terminal's settings, with the .env file's added.
  const env = {
    EVIE_HOME: home,
    PATH: "/usr/bin",
    OPENROUTER_API_KEY: "sk-or-secret",
    EVIE_MODEL: "some/model",
    EVIE_MAX_TOKENS: "4000",
    BRAVE_API_KEY: "brave-secret",
    EVIE_TOKEN: "a-device-token",
  }

  const done = await runService(env, ["install"], computer)

  expect(done.ok).toBe(true)
  expect(done.text).toContain(
    `These settings are set in this terminal, but not in ${join(home, ".env")}, so the service will not see them: OPENROUTER_API_KEY, EVIE_MODEL.`,
  )
  for (const value of ["sk-or-secret", "some/model", "brave-secret", "a-device-token"]) expect(done.text).not.toContain(value)
  for (const covered of ["BRAVE_API_KEY", "EVIE_MAX_TOKENS", "EVIE_TOKEN"]) expect(done.text).not.toContain(covered)
})

test("a service launchctl will not load at first, as while the older server is still stopping, is loaded when tried again", async () => {
  const { computer, calls, waits, folder, plist } = await mac({
    loaded: true,
    refuses: { bootstrap: { output: "Bootstrap failed: 5: Input/output error", times: 2 } },
  })

  const done = await runService({ EVIE_HOME: join(folder, "data"), PATH: "/usr/bin" }, ["install"], computer)

  expect(done.ok).toBe(true)
  const load = `bootstrap gui/501 ${plist}`
  expect(calls).toEqual(["print gui/501/com.evie.serve", "bootout gui/501/com.evie.serve", load, load, load, "print gui/501/com.evie.serve"])
  expect(waits).toEqual([500, 500, 2000])
})

test("a service launchctl will not load after five tries is reported with what launchctl said", async () => {
  const { computer, calls, waits, folder, plist } = await mac({ refuses: { bootstrap: { output: "Bootstrap failed: 5: Input/output error" } } })

  const done = await runService({ EVIE_HOME: join(folder, "data"), PATH: "/usr/bin" }, ["install"], computer)

  expect(done.ok).toBe(false)
  expect(done.text).toContain("Bootstrap failed: 5: Input/output error")
  expect(done.text).toContain(plist)
  expect(calls.filter((call) => call.startsWith("bootstrap"))).toHaveLength(5)
  expect(waits).toEqual([500, 500, 500, 500])
})

test("uninstall unloads the service and removes its file", async () => {
  const { computer, calls, folder, plist } = await mac()
  const env = { EVIE_HOME: join(folder, "data"), PATH: "/usr/bin" }
  await runService(env, ["install"], computer)
  calls.length = 0

  const done = await runService(env, ["uninstall"], computer)

  expect(done.ok).toBe(true)
  expect(calls).toEqual(["print gui/501/com.evie.serve", "bootout gui/501/com.evie.serve"])
  expect(await Bun.file(plist).exists()).toBe(false)
})

test("status says whether the service is installed and running, and which data folder and log it has", async () => {
  const { computer, calls, folder, plist } = await mac()
  const env = { EVIE_HOME: join(folder, "data"), PATH: "/usr/bin" }

  const before = await runService(env, ["status"], computer)
  expect(before.ok).toBe(true)
  expect(before.text).toBe("Installed:   no. `evie service install` installs it.\n" + "Running:     no\n")

  await runService(env, ["install"], computer)
  calls.length = 0
  const after = await runService(env, ["status"], computer)
  expect(after.text).toBe(
    `Installed:   yes, at ${plist}\n` +
      `Data folder: ${join(folder, "data")}\n` +
      "Running:     yes, as process 4242\n" +
      `Log:         ${join(folder, "data", "serve.log")}\n`,
  )
  expect(calls).toEqual(["print gui/501/com.evie.serve"])
})

test("there is one login service for every copy of Evie: another copy sees whose it is, and cannot replace or remove it", async () => {
  const { computer, calls, folder, plist } = await mac()
  const everyday = join(folder, "everyday")
  await runService({ EVIE_HOME: everyday, PATH: "/usr/bin" }, ["install"], computer)
  const installed = await readFile(plist, "utf8")
  calls.length = 0
  const otherCopy = { EVIE_HOME: join(folder, "dev"), PATH: "/usr/bin" }

  const status = await runService(otherCopy, ["status"], computer)
  expect(status.text).toContain(`Data folder: ${everyday}\n`)
  expect(status.text).toContain(`Log:         ${join(everyday, "serve.log")}\n`)
  expect(status.text).toContain(`belongs to another copy of Evie: this one's data folder is ${join(folder, "dev")}`)

  for (const action of ["install", "uninstall"]) {
    const done = await runService(otherCopy, [action], computer)
    expect(done.ok).toBe(false)
    expect(done.text).toContain(`The login service installed now is for the data folder ${everyday}`)
    expect(done.text).toContain("uninstall it from the copy of Evie that uses that folder first")
  }
  expect(await readFile(plist, "utf8")).toBe(installed)
  expect(calls.filter((call) => !call.startsWith("print"))).toEqual([])
})

test("on a system other than macOS, each action says the login service is not supported there yet", async () => {
  const { computer, calls, folder, plist } = await mac()
  const linux = { ...computer, platform: "linux" }

  for (const action of ["install", "uninstall", "status"]) {
    const done = await runService({ EVIE_HOME: join(folder, "data"), PATH: "/usr/bin" }, [action], linux)
    expect(done.ok).toBe(false)
    expect(done.text).toContain("not supported on linux yet")
  }
  expect(calls).toEqual([])
  expect(await Bun.file(plist).exists()).toBe(false)
})
