import type { Job, Plugin } from "../core/types.ts"

// The built-in plugin for the owner's and the model's side of background
// jobs. The jobs themselves are kept by the core; this adds the `/jobs` and
// `/background` commands for the owner and the `job_stop` tool for the model.

export function jobs(): Plugin {
  return {
    name: "jobs",
    setup(host) {
      host.commands.add({
        name: "jobs",
        description: "List what is running in the background. /jobs stop and an id stops one.",
        run(args, context) {
          const [word, id] = args.trim().split(/\s+/)
          if (word === "stop") {
            if (!id) return "Say which job to stop: /jobs stop and its id."
            const job = context.jobs.stop(id)
            if (!job) return `There is no job ${id}. /jobs lists them.`
            return job.status === "running" ? `Stopping job ${id} (${job.title}).` : `Job ${id} had already ended.`
          }
          const all = context.jobs.list()
          if (all.length === 0) return "Nothing has been run in the background in this session."
          return all.map(line).join("\n")
        },
      })

      host.commands.add({
        name: "background",
        description: "Move the command Evie is waiting on to the background, so it carries on with other work.",
        run(_args, context) {
          const moved = context.jobs.background()
          if (moved === 0) return context.busy ? "Evie is not waiting on anything that can be moved to the background." : "Nothing is running. This is for when Evie is waiting on a long command."
          return "Moved to the background. Evie carries on, and is told when it finishes."
        },
      })

      host.tools.add({
        name: "job_stop",
        description: "Stop a background job, such as a command that was started in or moved to the background.",
        inputSchema: {
          type: "object",
          properties: { id: { type: "string", description: "The id of the job, as given when it went to the background" } },
          required: ["id"],
        },
        async run(input, context) {
          const id = String(input.id)
          const job = context.jobs.stop(id)
          if (!job) {
            const running = context.jobs.list().filter((other) => other.status === "running")
            return `There is no job "${id}". ${running.length > 0 ? `The running jobs are: ${running.map((other) => `${other.id} (${other.title})`).join(", ")}.` : "No job is running."}`
          }
          if (job.status !== "running") return `Job ${id} had already ended, so there was nothing to stop.`
          return `Job ${id} (${job.title}) is being stopped. Its output so far is in ${job.outputFile}.`
        },
      })
    },
  }
}

const seconds = (job: Job) => Math.max(0, Math.round(((job.ended ?? Date.now()) - job.started) / 1000))
const line = (job: Job) => `${job.id}  ${job.status === "running" ? `running for ${seconds(job)}s` : `${job.status} after ${seconds(job)}s`}  ${job.title}`
