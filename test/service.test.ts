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
// `refuses` names a launchctl command that fails, with what it prints.
async function mac(options: { loaded?: boolean; refuses?: Record<string, string> } = {}) {
  const folder = await tempFolder()
  let loaded = options.loaded ?? false
  const calls: string[] = []
  const computer: Computer = {
    platform: "darwin",
    launchAgents: join(folder, "LaunchAgents"),
    uid: 501,
    async launchctl(args) {
      calls.push(args.join(" "))
      const refused = options.refuses?.[args[0]!]
      if (refused !== undefined) return { code: 5, output: refused }
      if (args[0] === "print") {
        return loaded
          ? { code: 0, output: "gui/501/com.evie.serve = {\n\tstate = running\n\tpid = 4242\n}" }
          : { code: 113, output: 'Could not find service "com.evie.serve" in domain for user gui: 501' }
      }
      if (args[0] === "bootstrap") loaded = true
      if (args[0] === "bootout") loaded = false
      return { code: 0, output: "" }
    },
  }
  return { computer, calls, folder, plist: join(folder, "LaunchAgents", "com.evie.serve.plist") }
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
  expect(calls).toEqual(["print gui/501/com.evie.serve", `bootstrap gui/501 ${plist}`])
})

test("installing again unloads the older service first, so the server restarts on this version", async () => {
  const { computer, calls, folder, plist } = await mac({ loaded: true })
  const home = join(folder, "data")
  await mkdir(dirname(plist), { recursive: true })
  await writeFile(plist, "an older version")

  const done = await runService({ EVIE_HOME: home, PATH: "/usr/bin" }, ["install"], computer)

  expect(done.ok).toBe(true)
  expect(calls).toEqual(["print gui/501/com.evie.serve", "bootout gui/501/com.evie.serve", `bootstrap gui/501 ${plist}`])
  expect(await readFile(plist, "utf8")).toContain(`<string>${CLI}</string>`)
})

test("a service launchctl will not load is reported with what launchctl said", async () => {
  const { computer, folder, plist } = await mac({ refuses: { bootstrap: "Bootstrap failed: 5: Input/output error" } })

  const done = await runService({ EVIE_HOME: join(folder, "data"), PATH: "/usr/bin" }, ["install"], computer)

  expect(done.ok).toBe(false)
  expect(done.text).toContain("Bootstrap failed: 5: Input/output error")
  expect(done.text).toContain(plist)
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

test("status says whether the service is installed and running, and where its log is", async () => {
  const { computer, calls, folder, plist } = await mac()
  const env = { EVIE_HOME: join(folder, "data"), PATH: "/usr/bin" }
  const log = join(folder, "data", "serve.log")

  const before = await runService(env, ["status"], computer)
  expect(before.ok).toBe(true)
  expect(before.text).toBe(
    "Installed: no. `evie service install` installs it.\n" + "Running:   no.\n" + `Log:       ${log}\n`,
  )

  await runService(env, ["install"], computer)
  calls.length = 0
  const after = await runService(env, ["status"], computer)
  expect(after.text).toBe(`Installed: yes, at ${plist}.\n` + "Running:   yes, as process 4242.\n" + `Log:       ${log}\n`)
  expect(calls).toEqual(["print gui/501/com.evie.serve"])
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
