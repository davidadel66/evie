import { join } from "node:path"
import { keepOutputTidy } from "../core/tools.ts"
import { createEvie, dataFolder, loadSettingsFiles, startServer, type EvieServer, type Settings } from "../index.ts"
import { ensureOwnToken, openTokens } from "../server/tokens.ts"
import { loadServerSettings } from "../settings-files.ts"
import { EXIT_CODES, fail, print, reason, type StopSignal } from "./common.ts"

// `evie serve`: starts the core with the plugins the settings list, and
// serves it until told to stop. The first time, it makes this computer's
// token. Once it is listening, it tidies away tool output older than a week,
// and again each day while it runs.
export async function serve(env: Settings) {
  const settings = await loadSettingsFiles({ env }).catch(fail)
  const serverSettings = await loadServerSettings({ env }).catch(fail)
  const evie = await createEvie({
    env,
    plugins: settings.plugins,
    projectSettings: settings.projectSettings,
    settingsFile: settings.file,
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

  let server: EvieServer
  try {
    server = startServer({ evie, tokens, host: serverSettings.host, port: serverSettings.port, origins: serverSettings.origins })
  } catch (cause) {
    tokens.close()
    return closeAndFail(
      `Evie could not listen on ${serverSettings.host}, port ${serverSettings.port}: ${reason(cause)} ` +
        `Is another \`evie serve\` running? The port can be changed under "server" in ${settings.file}.`,
    )
  }
  print(`evie serve · listening on ${server.url}\n`)
  // Only now, so that a second `evie serve`, which cannot have the port,
  // never touches the folder of the one already running.
  const stopTidying = keepOutputTidy(join(home, "tool-output"))

  // Being told to stop closes Evie properly: turns are stopped, and so is any
  // command left running in the background. Each step is taken even when the
  // one before it fails, so the core is closed whatever happens to the
  // listener. Told again while that is under way, it stops at once.
  let stopping = false
  for (const [signal, code] of Object.entries(EXIT_CODES) as [StopSignal, number][]) {
    process.on(signal, () => {
      if (stopping) process.exit(code)
      stopping = true
      stopTidying()
      void server
        .stop()
        .finally(() => evie.close())
        .finally(() => tokens.close())
        .catch((cause) => console.error(`Evie did not stop cleanly: ${reason(cause)}`))
        .finally(() => process.exit(code))
    })
  }
}
