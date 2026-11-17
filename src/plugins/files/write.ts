import { mkdir, stat, writeFile } from "node:fs/promises"
import { dirname } from "node:path"
import type { Tool } from "../../core/types.ts"
import { displayPath, resolvePath } from "./paths.ts"

export const write: Tool = {
  name: "write",
  description:
    "Create a file, or replace everything in an existing one. Folders that do not exist yet are created. A " +
    "relative path is taken from the project folder. To change part of a file, use edit instead.",
  inputSchema: {
    type: "object",
    properties: {
      path: { type: "string", description: "The file to write" },
      content: { type: "string", description: "The whole content of the file" },
    },
    required: ["path", "content"],
  },

  async run(input, context) {
    const path = input.path as string
    const content = input.content as string
    const target = resolvePath(path, context)
    if ("failure" in target) return target.failure
    const shown = displayPath(target.absolute, context)

    const existing = await stat(target.absolute).catch(() => undefined)
    if (existing?.isDirectory()) return `${shown} is a folder, not a file. Give the path of a file inside it.`

    await mkdir(dirname(target.absolute), { recursive: true })
    await writeFile(target.absolute, content)
    const lines = content === "" ? 0 : content.split("\n").length - (content.endsWith("\n") ? 1 : 0)
    return `${existing ? "Overwrote" : "Created"} ${shown} (${lines} line${lines === 1 ? "" : "s"}).`
  },
}
