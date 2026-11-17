import type { Plugin, Tool } from "../../core/types.ts"
import { edit } from "./edit.ts"
import { read } from "./read.ts"
import { write } from "./write.ts"

// The built-in plugin for working with files.
export function files(): Plugin {
  return {
    name: "files",
    setup(host) {
      for (const tool of [read, write, edit]) host.tools.add(reportingFileErrors(tool))
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
