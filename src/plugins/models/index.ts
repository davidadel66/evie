import type { ModelProfile, Plugin, Project, StoredEvent } from "../../core/types.ts"
import { BASE_PROMPT, GPT_PROMPT } from "./prompts.ts"

// The built-in plugin for the differences between models. A gateway makes
// every model reachable the same way, but not behave the same way: this
// gives each family the system prompt and the file-changing tools it works
// best with, and lets the owner switch model with /model.

const GPT: ModelProfile = { name: "gpt", prefix: "openai/gpt-", prompt: GPT_PROMPT, fileTools: ["patch"] }
const DEFAULT: ModelProfile = { name: "default", prefix: "", prompt: BASE_PROMPT, fileTools: ["edit", "write"] }

export function models(): Plugin {
  return {
    name: "models",
    setup(host) {
      host.profiles.add(GPT)
      host.profiles.add(DEFAULT)

      // The profile for a model: of those that cover it, the most specific.
      const profileFor = (model: string) =>
        host.profiles
          .list()
          .filter((profile) => model.startsWith(profile.prefix))
          .toSorted((a, b) => b.prefix.length - a.prefix.length)[0]

      host.on("before_request", (request, context) => {
        const profile = profileFor(request.model)
        if (!profile) return
        // Every tool for changing files that any profile knows of. Those this
        // profile does not list are left out, so a model is never offered
        // two ways of doing the same thing.
        const fileTools = new Set(host.profiles.list().flatMap((each) => each.fileTools))
        // The facts about the session come first, then the profile's
        // prompt. Anything another plugin has already added, such as the
        // owner's instructions, is kept after it.
        const prompt = `${facts(request.model, host.providers.list()[0]?.name, context.events, host.project)}\n\n${profile.prompt}`
        return {
          ...request,
          system: request.system ? `${prompt}\n\n${request.system}` : prompt,
          tools: request.tools.filter((tool) => !fileTools.has(tool.name) || profile.fileTools.includes(tool.name)),
        }
      })

      host.commands.add({
        name: "model",
        description: "Show the model in use, or switch to the one named.",
        run(args, context) {
          const wanted = args.trim()
          if (wanted === "") return `The model is ${context.model} (profile: ${profileFor(context.model)?.name ?? "none"}).`
          context.setModel(wanted)
          return `The model is now ${wanted} (profile: ${profileFor(wanted)?.name ?? "none"}).`
        },
      })
    },
  }
}

// What the model is told about the session it is in: which model it is,
// when the session started, what machine it is on and where it is working.
//
// Every line is fixed for the life of the session, so the text is the same
// in every request and the provider's prompt cache is not disturbed. The
// date is the session's start, read from its first event, and the model is
// told it may be much later now. Only switching model changes the text, and
// that changes the profile's prompt anyway.
//
// The model is named because some models, asked what they are, answer with
// the name of another.
function facts(model: string, provider: string | undefined, events: readonly StoredEvent[], project: Project | null): string {
  const through = provider === undefined ? "" : `, reached through ${provider === "openrouter" ? "OpenRouter" : provider}`
  const started = new Date(events[0]?.time ?? Date.now())
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const part = Object.fromEntries(
    new Intl.DateTimeFormat("en-GB", { weekday: "long", day: "numeric", month: "long", year: "numeric", hour: "2-digit", minute: "2-digit", hour12: false })
      .formatToParts(started)
      .map(({ type, value }) => [type, value]),
  )
  const machine = { darwin: "macOS", linux: "Linux", win32: "Windows" }[process.platform as string] ?? process.platform
  return [
    "Facts about this session. They were set when it started and are not updated.",
    "",
    `- The model you are running on is ${model}${through}. If you believe you are some other model, that belief comes from your training and is not true here.`,
    `- The session started on ${part.weekday} ${part.day} ${part.month} ${part.year} at ${part.hour}:${part.minute}, in the time zone ${zone}. It may be much later now. When the present date or time matters, run \`date\`. Do not guess.`,
    `- The computer runs ${machine}.`,
    `- ${where(project)}`,
  ].join("\n")
}

// Where the session is working.
function where(project: Project | null): string {
  if (!project) {
    return "This session is not in a project, so there is no project folder. Give full paths to files and folders."
  }
  return `You are working in the project "${project.name}". Its folder is ${project.folder}. A relative path is taken from there. A word such as @src/app.ts in the owner's message names a file by its path from there.`
}
