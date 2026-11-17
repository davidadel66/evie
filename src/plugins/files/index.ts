import type { Plugin } from "../../core/types.ts"
import { edit } from "./edit.ts"
import { read } from "./read.ts"
import { write } from "./write.ts"

// The built-in plugin for working with files.
export function files(): Plugin {
  return {
    name: "files",
    setup(host) {
      host.tools.add(read)
      host.tools.add(write)
      host.tools.add(edit)
    },
  }
}
