import { createEvie, dataFolder, loadSettingsFiles, startServer, type EvieServer, type Settings } from "../index.ts"
import { ensureOwnToken, openTokens } from "../server/tokens.ts"
import { loadServerSettings } from "../settings-files.ts"
import { EXIT_CODES, fail, print, type StopSignal } from "./common.ts"

// `evie serve`: starts the core with the plugins the settings list, and
// serves it until told to stop. The first time, it makes this computer's token.
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

  const home = dataFolder(env)
  const tokens = await openTokens(home)
  await ensureOwnToken(tokens, home)

  let server: EvieServer
  try {
    server = startServer({ evie, tokens, host: serverSettings.host, port: serverSettings.port, origins: serverSettings.origins })
  } catch (cause) {
    await evie.close()
    return fail(
      `Evie could not listen on ${serverSettings.host}, port ${serverSettings.port}: ${cause instanceof Error ? cause.message : String(cause)} ` +
        `Is another \`evie serve\` running? The port can be changed under "server" in ${settings.file}.`,
    )
  }
  print(`evie serve · listening on ${server.url}\n`)

  // Being told to stop closes Evie properly: turns are stopped, and so is any
  // command left running in the background. Told again while that is under
  // way, it stops at once.
  let stopping = false
  for (const [signal, code] of Object.entries(EXIT_CODES) as [StopSignal, number][]) {
    process.on(signal, () => {
      if (stopping) process.exit(code)
      stopping = true
      void server
        .stop()
        .then(() => evie.close())
        .finally(() => {
          tokens.close()
          process.exit(code)
        })
    })
  }
}
