import { spawn, type ChildProcess } from "node:child_process"
import { existsSync, realpathSync, rmSync } from "node:fs"
import { constants } from "node:os"
import { join } from "node:path"
import { keepOutputTidy } from "../core/tools.ts"
import { setUpPlugins } from "../core/evie.ts"
import { createEvie, dataFolder, loadSettingsFiles, startServer, type EvieServer, type Settings } from "../index.ts"
import { ensureOwnToken, openTokens } from "../server/tokens.ts"
import type { WebApp } from "../server/web-app.ts"
import { settingsEditor } from "../settings-editor.ts"
import { loadServerSettings } from "../settings-files.ts"
import { EXIT_CODES, fail, print, reason, STOP_FOR_GOOD, type StopSignal } from "./common.ts"
import { LABEL } from "./service.ts"

// Removes the file that says the server is to stop for good, if it is there.
function forgetStopForGood(home: string) {
  try {
    rmSync(join(home, STOP_FOR_GOOD), { force: true })
  } catch {
    // Left as it is: the next stop is for good too.
  }
}

// `evie serve`: starts the core with the plugins the settings list, and
// serves it until told to stop, with the web app. The first time, it makes
// this computer's token. Once it is listening, it picks up the work an
// earlier run left, and tidies away tool output older than a week, and
// again each day while it runs.
//
// `--dev` is for working on the web app: it is bundled again on each
// request, and the open page is updated as its files are saved. It needs
// the server started in the repository's folder, for its bunfig.toml.
export async function serve(env: Settings, args: string[] = []) {
  if (args[0] === "--check") return check(env, args.slice(1))
  if (args.some((arg) => arg !== "--dev")) {
    fail(
      "Usage:\n  evie serve           run Evie's server\n  evie serve --dev     run it with the web app rebuilt as it changes\n" +
        "  evie serve --check   check that it would start, without starting it",
    )
  }
  // Working on the web app, its module is needed at once, and a failure to
  // load it is for the developer to see.
  const development = args.includes("--dev") ? await import("../server/web-app.ts") : undefined
  if (development && realpathSync(process.cwd()) !== realpathSync(development.REPOSITORY)) {
    fail(`Start \`evie serve --dev\` in the repository's folder, ${development.REPOSITORY}: Bun takes the web app's Tailwind plugin from the bunfig.toml there.`)
  }
  const settings = await loadSettingsFiles({ env }).catch(fail)
  const serverSettings = await loadServerSettings({ env }).catch(fail)
  // What a restart asked for from the settings window does, once the server listens.
  let restart = () => {}
  const evie = await createEvie({
    env,
    plugins: settings.plugins,
    projectSettings: settings.projectSettings,
    trust: settings.trust,
    settingsFile: settings.file,
    settings: settingsEditor({ env, environment: process.env, files: settings, restart: () => restart() }),
    ...(settings.model !== undefined && { model: settings.model }),
    ...(settings.maxTokens !== undefined && { maxTokens: settings.maxTokens }),
    ...(settings.compaction !== undefined && { compaction: settings.compaction }),
  }).catch(fail)

  // From here on, a problem that stops Evie closes the core first.
  const closeAndFail = async (cause: unknown): Promise<never> => {
    await evie.close().catch(() => {})
    return fail(cause)
  }
  const home = dataFolder(env)
  const tokens = await openTokens(home).catch(closeAndFail)
  await ensureOwnToken(tokens, home).catch((cause) => {
    tokens.close()
    return closeAndFail(`This computer's token could not be made: ${reason(cause)}`)
  })

  // The web app is built before the server listens, which takes a moment.
  const built = development ? { app: development.webAppForDevelopment() } : await builtWebApp()
  if ("problem" in built) console.error(built.problem)

  let server: EvieServer
  try {
    server = startServer({
      evie,
      tokens,
      host: serverSettings.host,
      port: serverSettings.port,
      origins: serverSettings.origins,
      ...("app" in built && { app: built.app }),
    })
  } catch (cause) {
    tokens.close()
    return closeAndFail(
      `Evie could not listen on ${serverSettings.host}, port ${serverSettings.port}: ${reason(cause)} ` +
        `Is another \`evie serve\` running? The port can be changed under "server" in ${settings.file}.`,
    )
  }
  // A file saying to stop for good, left from before this server started,
  // as by an uninstall while none was running, is not for this one: left,
  // it would make its next restart a stop for good.
  forgetStopForGood(home)
  print(`evie serve · listening on ${server.url}\n`)
  if (development) print("The web app is bundled from its source on each request, and updated in the browser as it changes.\n")
  // Only now, so that a second `evie serve`, which cannot have the port,
  // never touches the folder or the work of the one already running.
  const stopTidying = keepOutputTidy(join(home, "tool-output"))
  void evie.resumeWork().catch((cause) => console.error(`The work left by the last run could not be picked up: ${reason(cause)}`))

  // Stopped for good, with Ctrl-C, by closing the terminal it runs in, or
  // with the file that says so, Evie is closed properly: turns are stopped,
  // and so is any command left running in the background. Told to stop
  // otherwise, as launchd does when the login service is restarted, or
  // asked to restart, its work is left as it is for the next start to pick up.
  // Each step is taken even when the one before it fails, so the core is
  // closed whatever happens to the listener. Told again while that is under
  // way, it stops at once.
  let stopping = false
  function stop(forGood: boolean, then: () => void) {
    stopping = true
    stopTidying()
    forgetStopForGood(home)
    void server
      .stop()
      .finally(() => evie.close(forGood ? {} : { restart: true }))
      .finally(() => tokens.close())
      .catch((cause) => console.error(`Evie did not stop cleanly: ${reason(cause)}`))
      .finally(then)
  }
  for (const [signal, code] of Object.entries(EXIT_CODES) as [StopSignal, number][]) {
    process.on(signal, () => {
      if (stopping) process.exit(code)
      stop(signal !== "SIGTERM" || existsSync(join(home, STOP_FOR_GOOD)), () => process.exit(code))
    })
  }
  restart = () => {
    if (stopping) return
    print("evie serve · starting again, as the settings window asked\n")
    stop(false, startAgain)
  }
}

// The exit code of a server that stopped to be started again.
const RESTART = 75
// Set in the environment of an `evie serve` that another watches over, to
// that one's process id.
const WATCHED_BY = "EVIE_WATCHED_BY"

// Once a restart has stopped the server, whatever runs it starts it again:
// launchd, for the login service, which starts it again whenever it stops;
// or the `evie serve` that started this one and watches over it. Run by
// hand, there is nothing to, so this process does: see `watchOver`.
// launchd is process 1, and names the service it starts in its
// environment, which is checked too, since a process whose parent has gone
// is given to process 1 as well.
function startAgain(): void {
  const byLoginService = process.ppid === 1 && process.env.XPC_SERVICE_NAME === LABEL
  if (byLoginService || process.env[WATCHED_BY] === String(process.ppid)) process.exit(RESTART)
  void watchOver()
}

// From now on this process only watches over a new `evie serve`, started
// with the same arguments, in the same folder and printing to the same
// place, and starts it again each time it stops for a restart. It passes
// on each signal that stops a server, so Ctrl-C and closing the terminal
// stop it as before, and it ends as that one ends, with its exit code. The
// new one is in a process group of its own, so that the terminal's Ctrl-C
// reaches it once, through this process.
async function watchOver(): Promise<never> {
  let current: ChildProcess | undefined
  for (const signal of Object.keys(EXIT_CODES) as StopSignal[]) {
    process.removeAllListeners(signal)
    process.on(signal, () => current?.kill(signal))
  }
  for (;;) {
    const started = spawn(process.execPath, process.argv.slice(1), {
      stdio: ["ignore", "inherit", "inherit"],
      detached: true,
      env: { ...process.env, [WATCHED_BY]: String(process.pid) },
    })
    current = started
    const code = await new Promise<number>((resolve) => {
      started.on("exit", (code, signal) => resolve(code ?? 128 + (signal ? constants.signals[signal] : 0)))
      started.on("error", (cause) => {
        console.error(`evie serve could not be started again: ${reason(cause)}`)
        resolve(1)
      })
    })
    if (code !== RESTART) process.exit(code)
  }
}

// `evie serve --check`, and the paths of plugin files to add: whether the
// server would start with its settings as they are, and with those plugins
// added, without starting it. The settings files are loaded and the plugins
// set up together, as `evie serve` does, and nothing else is opened. What
// would stop it is printed, and it exits with an error.
async function check(env: Settings, adding: string[]) {
  const settings = await loadSettingsFiles({ env, ...(adding.length > 0 && { adding }) }).catch(fail)
  await loadServerSettings({ env }).catch(fail)
  await setUpPlugins(settings.plugins, null).catch(fail)
  process.exit(0)
}

// The web app, built, or why it could not be. Its module is loaded only
// here, so the terminal client never loads the bundler's plugins, and a
// module that cannot be loaded, as when a package is missing, is treated as
// a build that failed: the server goes on without the page, since the
// terminal and every other client still work. `load` is for tests.
export async function builtWebApp(
  load: () => Promise<{ buildWebApp(): Promise<WebApp> }> = () => import("../server/web-app.ts"),
): Promise<{ app: WebApp } | { problem: string }> {
  try {
    return { app: await (await load()).buildWebApp() }
  } catch (cause) {
    return { problem: `The web app could not be built, so it is not served:\n${reason(cause)}` }
  }
}
