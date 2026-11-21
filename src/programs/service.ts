import { mkdir, rm, writeFile } from "node:fs/promises"
import { homedir } from "node:os"
import { delimiter, dirname, join, resolve } from "node:path"
import { dataFolder, type Settings } from "../index.ts"
import { fail, print } from "./common.ts"

// `evie service`: the login service, which runs `evie serve` when the owner
// logs in and starts it again if it stops. It is a macOS launch agent: a
// small file in ~/Library/LaunchAgents that tells launchd, the Mac's own
// service runner, what to run and how.

const LABEL = "com.evie.serve"

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

  if (action === "install") {
    // The log is written in the data folder, which launchd does not make.
    await mkdir(home, { recursive: true })
    await mkdir(computer.launchAgents, { recursive: true })
    await writeFile(file, launchAgent(env, log))
    // One that is loaded already is running an older version: it is stopped
    // and unloaded, so that the new file is the one that runs.
    const older = (await loaded()) !== undefined
    if (older) {
      const stopped = await computer.launchctl(["bootout", `${domain}/${LABEL}`])
      if (stopped.code !== 0) return { ok: false, text: `The running login service could not be stopped. launchctl said: ${stopped.output}` }
    }
    const started = await computer.launchctl(["bootstrap", domain, file])
    if (started.code !== 0) {
      return { ok: false, text: `${file} was written, but launchctl could not load it. It said: ${started.output}` }
    }
    return {
      ok: true,
      text:
        `${older ? "Reinstalled the login service, and started the server again on this version" : "Installed the login service, and started the server"}: ` +
        `\`evie serve\` now runs when you log in, and starts again if it stops.\n` +
        `  Service file: ${file}\n  Log:          ${log}\n`,
    }
  }

  if (action === "uninstall") {
    const wasLoaded = (await loaded()) !== undefined
    if (wasLoaded) {
      const stopped = await computer.launchctl(["bootout", `${domain}/${LABEL}`])
      if (stopped.code !== 0) return { ok: false, text: `The login service could not be stopped. launchctl said: ${stopped.output}` }
    }
    const hadFile = await Bun.file(file).exists()
    await rm(file, { force: true })
    if (!wasLoaded && !hadFile) return { ok: true, text: "The login service is not installed, so there was nothing to remove.\n" }
    return { ok: true, text: `Removed the login service, and stopped the server: \`evie serve\` no longer runs when you log in.\n` }
  }

  if (action === "status") {
    const installed = await Bun.file(file).exists()
    const shown = await loaded()
    // launchctl describes a loaded service as a list of `name = value` lines.
    const running = shown !== undefined && /^\s*state = running\s*$/m.test(shown)
    const pid = shown === undefined ? undefined : /^\s*pid = (\d+)\s*$/m.exec(shown)?.[1]
    return {
      ok: true,
      text:
        (installed ? `Installed: yes, at ${file}.\n` : "Installed: no. `evie service install` installs it.\n") +
        (running
          ? `Running:   yes${pid ? `, as process ${pid}` : ""}.\n`
          : shown !== undefined
            ? "Running:   no. It is loaded, but not running at the moment; its log may say why.\n"
            : "Running:   no.\n") +
        `Log:       ${log}\n`,
    }
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

// The launch agent's file: run this checkout's `evie serve` with the Bun
// running now, at login and again whenever it stops, with what it prints
// going to the log. launchd gives it a bare environment, so the data folder
// and a path that finds Bun and the commands Evie runs are set here.
function launchAgent(env: Settings, log: string): string {
  const bun = process.execPath
  const cli = join(import.meta.dir, "..", "cli.ts")
  const path = (env.PATH ?? "").split(delimiter).filter((folder) => folder !== "")
  const environment: Record<string, string> = {
    ...(env.EVIE_HOME && { EVIE_HOME: resolve(env.EVIE_HOME) }),
    PATH: [...new Set([dirname(bun), ...path])].join(delimiter),
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
