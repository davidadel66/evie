import type { Plugin, Tool } from "../../core/types.ts"
import { edit } from "./edit.ts"
import { patch } from "./patch.ts"
import { read } from "./read.ts"
import { searchTools } from "./search.ts"
import { write } from "./write.ts"

export type FilesOptions = {
  // The ripgrep program that grep and glob run. Defaults to `rg` on the path.
  ripgrep?: string
}

// The built-in plugin for working with files: reading, changing and finding them.
export function files(options: FilesOptions = {}): Plugin {
  return {
    name: "files",
    setup(host) {
      const tools = [read, write, edit, patch, ...searchTools(options.ripgrep ?? "rg")]
      for (const tool of tools) host.tools.add(reportingFileErrors(tool))
    },
  }
}

// The file system refusing something, such as no permission or a folder
// where a file was expected, is a failure the model can act on. It is
// returned as the tool's result. Anything else is a bug and is thrown.
function reportingFileErrors(tool: Tool): Tool {
  return {
    ...tool,
    async run(input, context) {
      try {
        return await tool.run(input, context)
      } catch (cause) {
        if (cause instanceof Error && typeof (cause as { code?: unknown }).code === "string") {
          return `${tool.name} could not finish: ${cause.message}.`
        }
        throw cause
      }
    },
  }
}
