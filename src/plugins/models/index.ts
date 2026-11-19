import type { ModelProfile, Plugin, Project } from "../../core/types.ts"
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

      host.on("before_request", (request) => {
        const profile = profileFor(request.model)
        if (!profile) return
        // Every tool for changing files that any profile knows of. Those this
        // profile does not list are left out, so a model is never offered
        // two ways of doing the same thing.
        const fileTools = new Set(host.profiles.list().flatMap((each) => each.fileTools))
        // The profile's prompt comes first. Anything another plugin has
        // already added, such as the owner's instructions, is kept after it.
        const prompt = `${profile.prompt}\n\n${where(host.project)}`
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

// Where the session is working. It is the same for every request in a
// project, so it does not disturb the provider's prompt cache.
function where(project: Project | null): string {
  if (!project) {
    return "This session is not in a project, so there is no project folder. Give full paths to files and folders."
  }
  return `You are working in the project "${project.name}". Its folder is ${project.folder}. A relative path is taken from there.`
}
