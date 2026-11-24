import { mkdir, readFile, writeFile } from "node:fs/promises"
import { join } from "node:path"
import type { Plugin } from "../../src/index.ts"
import { say, startEvie, stored } from "./harness.ts"
import { scriptedProvider, type ScriptedReply } from "./scripted-provider.ts"

// Starts Evie with the given tool plugins and a project holding the given
// files. `call` has the model call one tool, and returns the text it gets
// back, exactly as the model would read it.
export async function toolProject(plugins: Plugin[], contents: Record<string, string> = {}) {
  const script: ScriptedReply[] = []
  const run = await startEvie([scriptedProvider(script).plugin, ...plugins])
  const path = (name: string) => join(run.projectFolder, name)
  for (const [name, text] of Object.entries(contents)) {
    await mkdir(join(path(name), ".."), { recursive: true })
    await writeFile(path(name), text)
  }
  async function call(name: string, input: Record<string, unknown>, session = run.session) {
    script.push({ text: [], calls: [{ name, input }] }, { text: ["done"] })
    await say(run, "go", session)
    const results = stored(run.events).flatMap((event) => (event.type === "tool_result" ? [event.body.text] : []))
    return results.at(-1)!
  }
  return { run, call, path, script, text: (name: string) => readFile(path(name), "utf8") }
}
