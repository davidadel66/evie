import { afterEach, expect, test } from "bun:test"
import { existsSync } from "node:fs"
import { mkdir, mkdtemp, readFile, realpath, rm, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { dirname, join } from "node:path"
import { STOP_FOR_GOOD } from "../src/programs/common.ts"
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
// service starts stops at once. With `disabled`, the service's name was
// disabled earlier, as by an older program of the same name, and launchctl
// will not load it until it is enabled. Pauses are recorded, not waited for.
async function mac(options: { loaded?: boolean; refuses?: Record<string, { output: string; times?: number }>; stops?: true; disabled?: true } = {}) {
  const folder = await tempFolder()
  let loaded = options.loaded ?? false
  let disabled = options.disabled ?? false
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
      if (args[0] === "enable") disabled = false
      if (args[0] === "bootstrap" && disabled) return { code: 5, output: "Bootstrap failed: 5: Input/output error" }
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

  // A path with no Bun on it, so the Bun running now is used, and its folder is put first.
  const done = await runService({ EVIE_HOME: home, PATH: `${folder}/tools:/usr/bin` }, ["install"], computer)

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
    <string>${dirname(BUN)}:${folder}/tools:/usr/bin</string>
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
  expect(calls).toEqual(["print gui/501/com.evie.serve", "enable gui/501/com.evie.serve", `bootstrap gui/501 ${plist}`, "print gui/501/com.evie.serve"])
})

test("install enables the service's name before loading it, so a name disabled earlier does not stop it loading", async () => {
  const { computer, calls, folder, plist } = await mac({ disabled: true })

  const done = await runService({ EVIE_HOME: join(folder, "data"), PATH: "/usr/bin" }, ["install"], computer)

  expect(done.ok).toBe(true)
  expect(calls.slice(1, 3)).toEqual(["enable gui/501/com.evie.serve", `bootstrap gui/501 ${plist}`])
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
  expect(calls).toEqual(["print gui/501/com.evie.serve", "bootout gui/501/com.evie.serve", "enable gui/501/com.evie.serve", `bootstrap gui/501 ${plist}`, "print gui/501/com.evie.serve"])
  expect(await readFile(plist, "utf8")).toContain(`<string>${CLI}</string>`)
})

test("installing again when the older service cannot be stopped leaves its file as it was", async () => {
  const { computer, folder, plist } = await mac({ refuses: { bootout: { output: "Boot-out failed: 5: Input/output error" } } })
  const env = { EVIE_HOME: join(folder, "data"), PATH: "/usr/bin" }
  await runService(env, ["install"], computer)
  const older = (await readFile(plist, "utf8")).replace(CLI, "/an/older/checkout/src/cli.ts")
  await writeFile(plist, older)

  const done = await runService(env, ["install"], computer)

  expect(done).toEqual({ ok: false, text: "The running login service could not be stopped. launchctl said: Boot-out failed: 5: Input/output error" })
  expect(await readFile(plist, "utf8")).toBe(older)
})

test("the service runs the Bun this terminal's path finds, as written there, so that it outlives an update to Bun", async () => {
  const { computer, folder, plist } = await mac()
  const tools = join(folder, "tools")
  await mkdir(tools)
  // As a package manager links a versioned Bun into a folder on the path.
  await symlink(BUN, join(tools, "bun"))
  const path = `/no/such/folder:${tools}:/usr/bin`

  const done = await runService({ EVIE_HOME: join(folder, "data"), PATH: path }, ["install"], computer)

  const written = await readFile(plist, "utf8")
  expect(written).toContain(`  <array>\n    <string>${join(tools, "bun")}</string>\n`)
  expect(written).toContain(`<key>PATH</key>\n    <string>${path}</string>`)
  expect(done.text).toContain("If Bun moves, as when it is installed again somewhere else, run `evie service install` again.")
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
  expect(calls).toEqual(["print gui/501/com.evie.serve", "bootout gui/501/com.evie.serve", "enable gui/501/com.evie.serve", load, load, load, "print gui/501/com.evie.serve"])
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

test("uninstall that cannot stop the service takes back the file telling the server it stops for good", async () => {
  const { computer, folder } = await mac({ refuses: { bootout: { output: "Boot-out failed: 5: Input/output error" } } })
  const home = join(folder, "data")
  const env = { EVIE_HOME: home, PATH: "/usr/bin" }
  await runService(env, ["install"], computer)

  const done = await runService(env, ["uninstall"], computer)

  expect(done.ok).toBe(false)
  // Left there, it would make the server's next restart count as a stop for good.
  expect(existsSync(join(home, STOP_FOR_GOOD))).toBe(false)
})

test("uninstall tells the server it is stopping for good, by a file in its data folder, before unloading it; installing again does not", async () => {
  const { computer, folder } = await mac()
  const home = join(folder, "data")
  const env = { EVIE_HOME: home, PATH: "/usr/bin" }
  const told = join(home, STOP_FOR_GOOD)
  // Whether the file was there each time launchctl was asked to unload the service.
  const atBootout: boolean[] = []
  const watched: Computer = {
    ...computer,
    launchctl: (args) => {
      if (args[0] === "bootout") atBootout.push(existsSync(told))
      return computer.launchctl(args)
    },
  }

  await runService(env, ["install"], watched)
  await runService(env, ["install"], watched)
  expect(atBootout).toEqual([false])
  expect(existsSync(told)).toBe(false)

  await runService(env, ["uninstall"], watched)
  expect(atBootout).toEqual([false, true])
  expect(await readFile(told, "utf8")).toBe("")
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
