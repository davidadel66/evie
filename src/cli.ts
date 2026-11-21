#!/usr/bin/env bun
// The `evie` command. It reads which program is asked for and hands over to
// it; each lives in a file of its own in `programs/`.
//
//   evie serve                 runs Evie: the core behind its server
//   evie                       the terminal client, in the folder it is run
//                              in; it connects to the server
//   evie token add <name>      makes a token for a device; list and remove
//                              list them and take one back
//   evie service install       runs `evie serve` at login, on macOS;
//                              uninstall and status remove it and report on it
import { loadSettings } from "./settings.ts"
import { serve } from "./programs/serve.ts"
import { service } from "./programs/service.ts"
import { terminal } from "./programs/terminal.ts"
import { token } from "./programs/token.ts"

// If whatever was reading Evie's output has gone away, as when it is piped
// into a program that stops early, there is no one left to print for, so
// Evie quits quietly.
process.stdout.on("error", (cause: { code?: string }) => {
  if (cause.code === "EPIPE") process.exit(0)
})

// Keys come from the environment and the .env file in the data folder.
const env = await loadSettings(process.env)

const [command, ...rest] = process.argv.slice(2)
if (command === "serve") await serve(env)
else if (command === "token") await token(env, rest)
else if (command === "service") await service(env, rest)
else await terminal(env, process.argv.slice(2))
