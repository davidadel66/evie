import { accessSync, constants, statSync } from "node:fs"
import { mkdir, readFile, rm, writeFile } from "node:fs/promises"
import { homedir } from "node:os"
import { delimiter, dirname, isAbsolute, join, resolve } from "node:path"
import { dataFolder, loadSettings, type Settings } from "../index.ts"
import { fail, print, STOP_FOR_GOOD } from "./common.ts"

// `evie service`: the login service, which runs `evie serve` when the owner
// logs in and starts it again if it stops. It is a macOS launch agent: a
// small file in ~/Library/LaunchAgents that tells launchd, the Mac's own
// service runner, what to run and how.

const LABEL = "com.evie.serve"
// How many times loading the service is tried, and how long to wait between tries.
const LOAD_TRIES = 5
const LOAD_PAUSE_MS = 500
// How long after loading the service to look whether the server is still
// running: one that cannot start, such as with a broken settings file,
// has stopped by then.
const START_CHECK_MS = 2000

// The settings `evie serve` reads from its environment. The service starts
// it with none of the terminal's, so each has to be in the data folder's
// `.env`, or, for some, in its `evie.json`, for the service to have it.
const SERVER_SETTINGS: { name: string; inSettingsFile?: string }[] = [
  { name: "OPENROUTER_API_KEY" },
  { name: "BRAVE_API_KEY" },
  { name: "EVIE_MODEL", inSettingsFile: "model" },
  { name: "EVIE_MAX_TOKENS", inSettingsFile: "maxTokens" },
]

// What the login service needs from the computer it runs on. Tests stand
// in for it, so that they never touch the real launch agents or launchd.
export type Computer = {
  // The operating system, as Node names it: "darwin" for macOS.
  platform: string
  // The folder macOS reads a user's login services from.
  launchAgents: string
  // The user's id, which names their login session to launchctl.
  uid: number
  // Runs launchctl, the command that talks to launchd, and gives back its
  // exit code and what it printed.
  launchctl(args: string[]): Promise<{ code: number; output: string }>
  // Waits this many milliseconds, between one try and the next.
  wait(ms: number): Promise<void>
}

export function thisComputer(): Computer {
  return {
    platform: process.platform,
    launchAgents: join(homedir(), "Library", "LaunchAgents"),
    uid: process.getuid?.() ?? 0,
    async launchctl(args) {
      const run = Bun.spawn(["launchctl", ...args], { stdout: "pipe", stderr: "pipe" })
      const [out, err, code] = await Promise.all([new Response(run.stdout).text(), new Response(run.stderr).text(), run.exited])
      return { code, output: `${out}${err}`.trim() }
    },
    wait: (ms) => Bun.sleep(ms),
  }
}

// The program: what it says is printed, and a failure exits with an error.
export async function service(env: Settings, args: string[]) {
  const done = await runService(env, args, thisComputer())
  if (!done.ok) fail(done.text)
  print(done.text)
}

// Does what `evie service` was asked, and says how it went.
export async function runService(env: Settings, [action]: string[], computer: Computer): Promise<{ ok: boolean; text: string }> {
  const file = join(computer.launchAgents, `${LABEL}.plist`)
  const home = resolve(dataFolder(env))
  const log = join(home, "serve.log")
  const domain = `gui/${computer.uid}`
  // What launchctl knows of the service: given only while it is loaded.
  const loaded = async () => {
    const shown = await computer.launchctl(["print", `${domain}/${LABEL}`])
    return shown.code === 0 ? shown.output : undefined
  }

  if (["install", "uninstall", "status"].includes(action ?? "") && computer.platform !== "darwin") {
    return {
      ok: false,
      text: `The login service is a macOS launch agent, and is not supported on ${computer.platform} yet. Run \`evie serve\` by hand instead.`,
    }
  }

  // There is one login service, whichever copy of Evie installed it. A copy
  // with another data folder, such as a development copy, may look at it
  // but not replace or remove it.
  const current = await installed(file)
  if ((action === "install" || action === "uninstall") && current && current.home !== home) {
    return {
      ok: false,
      text:
        `The login service installed now is for the data folder ${current.home}, not this copy's, ${home}. ` +
        "There is one login service, so uninstall it from the copy of Evie that uses that folder first.",
    }
  }

  if (action === "install") {
    // The log is written in the data folder, which launchd does not make.
    await mkdir(home, { recursive: true })
    // One that is loaded already is running an older version: it is stopped
    // and unloaded, so that the new file is the one that runs. Its file is
    // replaced only once it is, so one that cannot be stopped is left whole.
    const older = (await loaded()) !== undefined
    if (older) {
      const stopped = await computer.launchctl(["bootout", `${domain}/${LABEL}`])
      if (stopped.code !== 0) return { ok: false, text: `The running login service could not be stopped. launchctl said: ${stopped.output}` }
    }
    await mkdir(computer.launchAgents, { recursive: true })
    await writeFile(file, launchAgent(env, log))
    // A name disabled earlier, as by an older program that used it, is never
    // loaded until it is enabled again; enabling one that is not disabled
    // changes nothing.
    const enabled = await computer.launchctl(["enable", `${domain}/${LABEL}`])
    if (enabled.code !== 0) return { ok: false, text: `The login service could not be enabled. launchctl said: ${enabled.output}` }
    // An older server that is slow to stop can leave the new one unable to
    // load for a moment, so loading is tried a few times before giving up.
    let started = await computer.launchctl(["bootstrap", domain, file])
    for (let tries = 1; started.code !== 0 && tries < LOAD_TRIES; tries++) {
      await computer.wait(LOAD_PAUSE_MS)
      started = await computer.launchctl(["bootstrap", domain, file])
    }
    if (started.code !== 0) {
      return { ok: false, text: `${file} was written, but launchctl could not load it, after ${LOAD_TRIES} tries. It said: ${started.output}` }
    }
    const missed = await notSeen(env, home)
    const settingsNote =
      missed.length === 0
        ? ""
        : `These settings are set in this terminal, but not in ${join(home, ".env")}, so the service will not see them: ` +
          `${missed.join(", ")}. Put them in that file, then run \`evie service install\` again.\n`

    await computer.wait(START_CHECK_MS)
    const shown = await loaded()
    const pid = shown === undefined ? undefined : runningAs(shown)
    if (pid === undefined) {
      return {
        ok: false,
        text:
          "The login service is installed, but the server is not running: it stopped soon after it started, and launchd " +
          `will start it again in a while. Its log says why: ${log}\n${settingsNote}`,
      }
    }
    return {
      ok: true,
      text:
        `${older ? "Reinstalled the login service, and started the server again on this version" : "Installed the login service, and started the server"}, ` +
        `which is running${pid ? ` as process ${pid}` : ""}: \`evie serve\` now runs when you log in, and starts again if it stops.\n` +
        `  Service file: ${file}\n  Log:          ${log}\n` +
        "If Bun moves, as when it is installed again somewhere else, run `evie service install` again.\n" +
        settingsNote,
    }
  }

  if (action === "uninstall") {
    const wasLoaded = (await loaded()) !== undefined
    if (wasLoaded) {
      // Removing the service stops the server for good, unlike the stop
      // before a restart, so the server is told first. A data folder that
      // is gone cannot be told, and the server is stopped all the same.
      await writeFile(join(current?.home ?? home, STOP_FOR_GOOD), "").catch(() => {})
      const stopped = await computer.launchctl(["bootout", `${domain}/${LABEL}`])
      if (stopped.code !== 0) return { ok: false, text: `The login service could not be stopped. launchctl said: ${stopped.output}` }
    }
    const hadFile = await Bun.file(file).exists()
    await rm(file, { force: true })
    if (!wasLoaded && !hadFile) return { ok: true, text: "The login service is not installed, so there was nothing to remove.\n" }
    return { ok: true, text: `Removed the login service, and stopped the server: \`evie serve\` no longer runs when you log in.\n` }
  }

  if (action === "status") {
    const shown = await loaded()
    const pid = shown === undefined ? undefined : runningAs(shown)
    const running = pid !== undefined
    const lines = current
      ? [`Installed:   yes, at ${file}`, `Data folder: ${current.home}`]
      : ["Installed:   no. `evie service install` installs it."]
    lines.push(
      running
        ? `Running:     yes${pid ? `, as process ${pid}` : ""}`
        : shown !== undefined
          ? "Running:     no. It is loaded, but not running at the moment; its log may say why."
          : "Running:     no",
    )
    if (current) lines.push(`Log:         ${current.log}`)
    if (current && current.home !== home) lines.push(`This service belongs to another copy of Evie: this one's data folder is ${home}.`)
    return { ok: true, text: `${lines.join("\n")}\n` }
  }
  return {
    ok: false,
    text: [
      "Usage:",
      "  evie service install     run `evie serve` at login, and again if it stops; run it again after an update",
      "  evie service uninstall   stop it, and no longer run it at login",
      "  evie service status      whether it is installed and running, and where its log is",
    ].join("\n"),
  }
}

// Whether launchctl's description of the loaded service says its server is
// running, and as which process: an empty string when it does not say.
// Undefined when it is not running. The description is a list of
// `name = value` lines.
function runningAs(shown: string): string | undefined {
  if (!/^\s*state = running\s*$/m.test(shown)) return undefined
  return /^\s*pid = (\d+)\s*$/m.exec(shown)?.[1] ?? ""
}

// The names of the server's settings that are set in this terminal but in
// neither of the data folder's files, so that the service, which is not
// started from this terminal, will not have them. Only names: never values.
async function notSeen(env: Settings, home: string): Promise<string[]> {
  const inEnvFile = await loadSettings({ EVIE_HOME: home })
  const inSettingsFile: Record<string, unknown> = await readFile(join(home, "evie.json"), "utf8")
    .then((text) => JSON.parse(text) ?? {})
    .catch(() => ({}))
  return SERVER_SETTINGS.filter(
    ({ name, inSettingsFile: key }) => env[name] !== undefined && inEnvFile[name] === undefined && !(key !== undefined && inSettingsFile[key] !== undefined),
  ).map(({ name }) => name)
}

// What the installed service's file says: the data folder it serves, and
// where its log is. Undefined when none is installed. The file is read as
// `install` writes it; one without a data folder serves the usual one.
async function installed(file: string): Promise<{ home: string; log: string } | undefined> {
  const text = await readFile(file, "utf8").catch(() => undefined)
  if (text === undefined) return undefined
  const value = (key: string) => {
    const found = new RegExp(`<key>${key}</key>\\s*<string>([^<]*)</string>`).exec(text)?.[1]
    return found?.replaceAll("&lt;", "<").replaceAll("&gt;", ">").replaceAll("&amp;", "&")
  }
  const home = value("EVIE_HOME") ?? dataFolder({})
  return { home, log: value("StandardOutPath") ?? join(home, "serve.log") }
}

// The Bun to run the server with: the first the path finds, named as the
// path names it. That is often a link a package manager keeps in place when
// it updates Bun, where the Bun running now can be in a folder for one
// version, which an update removes. The Bun running now when the path has none.
function bunToRun(path: string[]): string {
  for (const folder of path.filter((each) => isAbsolute(each))) {
    const candidate = join(folder, "bun")
    try {
      accessSync(candidate, constants.X_OK)
      if (statSync(candidate).isFile()) return candidate
    } catch {}
  }
  return process.execPath
}

// The launch agent's file: run this checkout's `evie serve` with Bun, at
// login and again whenever it stops, with what it prints going to the log.
// launchd gives it a bare environment, so the data folder and a path that
// finds Bun and the commands Evie runs are set here.
function launchAgent(env: Settings, log: string): string {
  const path = (env.PATH ?? "").split(delimiter).filter((folder) => folder !== "")
  const bun = bunToRun(path)
  const cli = join(import.meta.dir, "..", "cli.ts")
  const environment: Record<string, string> = {
    ...(env.EVIE_HOME && { EVIE_HOME: resolve(env.EVIE_HOME) }),
    PATH: (path.includes(dirname(bun)) ? path : [dirname(bun), ...path]).join(delimiter),
  }
  const string = (text: string) => `<string>${text.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;")}</string>`
  return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  ${string(LABEL)}
  <key>ProgramArguments</key>
  <array>
${[bun, cli, "serve"].map((arg) => `    ${string(arg)}`).join("\n")}
  </array>
  <key>EnvironmentVariables</key>
  <dict>
${Object.entries(environment)
  .map(([name, value]) => `    <key>${name}</key>\n    ${string(value)}`)
  .join("\n")}
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  ${string(log)}
  <key>StandardErrorPath</key>
  ${string(log)}
</dict>
</plist>
`
}
