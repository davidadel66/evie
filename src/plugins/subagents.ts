import { join } from "node:path"
import { dataFolder } from "../core/evie.ts"
import type { Children, JobWork, Plugin, ToolContext } from "../core/types.ts"
import { entriesOf, foldersFor, readsOnly, splitHeader, textOf, tidyName } from "./definitions.ts"

// The built-in subagents plugin. It lets the model hand a piece of work to
// a subagent: a second conversation that starts empty, works with the same
// tools in the same project, and reports back when it has finished. What
// the subagent read and tried along the way stays out of the main
// conversation; only its report comes back.
//
// The child session, its loop and its questions are the core's. A
// subagent's work is a background job, so the model that started it is
// told, and woken, when the report is ready. This plugin adds the two tools
// and says what a subagent is not given. When Evie restarts, the core
// carries the subagent on, and this plugin follows its round of work again.
//
// A subagent can be started as one of the owner's agents. An agent is a
// Markdown file: a header with a name, a description, and optionally a model
// and `mode: read-only`, and a body that is the agent's instructions. Any
// mode other than "auto" holds the agent to reading, a misspelling included. Agents
// are read from `agents` in the data folder, for every project, and from
// `.evie/agents` in a project the owner has trusted. They are read each
// time they are needed, so one written a moment ago can be used at once.

export type SubagentsOptions = {
  // The folder Evie keeps its state in. Defaults to the usual place.
  dataFolder?: string
  // Whether the owner has trusted a project, by its folder. A project's own
  // agents are used only when they have. Defaults to no.
  trusted?: (folder: string) => boolean
}

// One of the owner's agents.
type Agent = { name: string; description: string; prompt: string; model?: string; readOnly: boolean; file: string }

// How many subagents one session may have working at once.
const MOST_AT_ONCE = 8
// Tools a subagent is not offered: it does not start subagents, and it has
// no owner of its own to put a question to.
const NOT_FOR_SUBAGENTS = ["subagent", "subagent_send", "ask"]

const FOR_SUBAGENTS =
  "You are a subagent: another agent has handed you one piece of work, in the message below. You have none of its " +
  "conversation, only that message. Do the work it describes and nothing beyond it. Your last reply is your report " +
  "to that agent, and the only thing it will see, so make it complete: what you found or did, with the file paths, " +
  "names and values that matter, and anything you could not do and why. You cannot ask the owner questions; if you " +
  "need a decision, say so in your report."

export function subagents(options: SubagentsOptions = {}): Plugin {
  const data = options.dataFolder ?? dataFolder({})
  const trusted = options.trusted ?? (() => false)

  return {
    name: "subagents",
    setup(host) {
      // The agents there are now. A project's agent replaces the owner's of the same name.
      async function agents(): Promise<Agent[]> {
        const found = new Map<string, Agent>()
        for (const folder of foldersFor("agents", data, host.project?.folder ?? null, trusted)) {
          for (const entry of await entriesOf(folder)) {
            if (!entry.isFile() || !entry.name.endsWith(".md")) continue
            const file = join(folder, entry.name)
            const text = await textOf(file, folder)
            if (text === undefined) continue
            const { header, body } = splitHeader(text)
            const name = tidyName(header.name ?? entry.name.slice(0, -3))
            if (name === "" || body === "") continue
            found.set(name, {
              name,
              description: header.description ?? "",
              prompt: body,
              ...(header.model !== undefined && { model: header.model }),
              // A project's agent can narrow what the owner's of the same
              // name may do, and never widens it.
              readOnly: readsOnly(header.mode) || (found.get(name)?.readOnly ?? false),
              file,
            })
          }
        }
        return [...found.values()].toSorted((a, b) => a.name.localeCompare(b.name))
      }

      // The subagents this plugin has started, with a few words on what each is doing.
      const started = new Map<string, string>()
      // Each subagent whose present round of work a job follows, and whether it was stopped.
      const following = new Map<string, { stopped: boolean }>()
      const tooMany = (context: ToolContext) =>
        context.jobs.list().filter((job) => job.kind === "subagent" && job.status === "running").length >= MOST_AT_ONCE
          ? `${MOST_AT_ONCE} subagents are already working, which is the most at once. Wait for one to report before starting another.`
          : undefined

      // A subagent's round of work, as a job. The model is told its report
      // when the round ends.
      const round = (child: string, children: Children, title: string): JobWork => {
        const entry = { stopped: false }
        following.set(child, entry)
        return {
          kind: "subagent",
          title,
          keep: { child },
          subagent: child,
          done: children
            .whenIdle(child)
            .finally(() => {
              if (following.get(child) === entry) following.delete(child)
            })
            .then(({ reply, ended, error }) => {
              const again = `To send this subagent a further message, use subagent_send with the id "${child}".`
              if (ended === "completed") return `Its report:\n\n${reply || "(It ended without saying anything.)"}\n\n${again}`
              const why = ended === "cancelled" ? "It was stopped before it finished." : `It failed: ${(error ?? "no reason was given").replace(/\.$/, "")}.`
              throw new Error(`${why}${reply ? ` The last thing it said:\n\n${reply}` : ""}\n\n${again}`)
            }),
          stop: () => {
            entry.stopped = true
            children.stop(child)
          },
        }
      }
      // A round is listed by what its subagent was started as, or, for one
      // started before Evie last restarted, by its name, from its task.
      const follow = (child: string, context: ToolContext) =>
        context.jobs.start(round(child, context.children, `subagent: ${started.get(child) ?? context.children.name(child) ?? child}`))
      // A round an earlier run of Evie left, which the core has carried on.
      host.jobKinds.add({ name: "subagent", resume: (job, { children }) => round(String(job.keep.child), children, job.title) })

      host.on("before_request", async (request, context) => {
        const all = await agents()
        // The main agent is told which agents it can start a subagent as.
        if (context.parent === undefined) {
          if (all.length === 0) return
          const list = all.map((agent) => `- ${agent.name}: ${agent.description || "(no description)"}${agent.readOnly ? " (reads only)" : ""}`)
          const about = `Agents the owner has defined. Give one's name as "agent" to the subagent tool to start a subagent as it:\n${list.join("\n")}`
          return { ...request, system: request.system ? `${request.system}\n\n${about}` : about }
        }
        // A subagent is told what it is, and what its agent's instructions are.
        const agent = all.find((each) => each.name === context.agent)
        const told = agent ? `${FOR_SUBAGENTS}\n\nYou are working as the agent "${agent.name}". Its instructions:\n\n${agent.prompt}` : FOR_SUBAGENTS
        return {
          ...request,
          system: request.system ? `${request.system}\n\n${told}` : told,
          tools: request.tools.filter((tool) => !NOT_FOR_SUBAGENTS.includes(tool.name)),
        }
      })

      host.tools.add({
        name: "subagent",
        description:
          "Hand a piece of work to a subagent: another agent that starts with an empty conversation, works with the " +
          "same tools in the same project, and reports back. Use it for work whose detail you do not need to keep, " +
          "such as searching a codebase, researching a question, reviewing a change or running and reading tests, and " +
          "for pieces of work that do not depend on each other, which you can start several of at once. Keep changes " +
          "to files that depend on each other in your own hands.\n\n" +
          "A subagent always works in the background. This call returns at once, without the report. You are woken " +
          "with the report when it has finished, so until then you do not have it: do not guess it, and do not ask " +
          "the subagent for it. Carry on with work that does not overlap with its task, or say briefly what you have " +
          "started and end your reply.\n\n" +
          "The subagent sees none of this conversation, only your task. Write the task so it stands alone: the goal, " +
          "what you already know, where to look, what is out of scope, and what the report should contain.\n\n" +
          "The owner can define agents: roles with their own instructions. When they have, you are given the list, " +
          "and can start a subagent as one by its name. An agent is a Markdown file in " +
          `${join(data, "agents")}, for every project, or in .evie/agents in a project: a header between two lines of ` +
          '"---" with "name:", "description:", and optionally "model:" and "mode: read-only", followed by the ' +
          "agent's instructions.",
        inputSchema: {
          type: "object",
          properties: {
            description: { type: "string", description: "Three to five words naming the task, shown to the owner" },
            task: { type: "string", description: "The whole task, written for an agent that has seen nothing else" },
            agent: { type: "string", description: "The name of one of the owner's agents to start the subagent as. Leave out for a general one" },
            model: {
              type: "string",
              description: "The model for the subagent. Leave this out unless the owner asks for a particular model",
            },
          },
          required: ["description", "task"],
        },
        async run(input, context) {
          const task = typeof input.task === "string" ? input.task.trim() : ""
          const description = typeof input.description === "string" && input.description.trim() !== "" ? input.description.trim() : "a task"
          if (task === "") return "The task was empty. Call subagent again with the whole task in words."
          const refusal = tooMany(context)
          if (refusal !== undefined) return refusal

          // Started as one of the owner's agents, when one is named.
          let agent: Agent | undefined
          if (typeof input.agent === "string" && input.agent.trim() !== "") {
            const all = await agents()
            agent = all.find((each) => each.name === tidyName(input.agent as string))
            if (!agent) {
              const names = all.map((each) => each.name).join(", ")
              return `There is no agent named "${input.agent}". ${names ? `The agents are: ${names}.` : "The owner has defined no agents."} Leave "agent" out for a general subagent.`
            }
          }
          // A model the owner asked for wins over the agent's own.
          const model = typeof input.model === "string" && input.model.trim() !== "" ? input.model : agent?.model

          let child: string
          try {
            child = await context.children.start({
              text: task,
              ...(model !== undefined && { model }),
              ...(agent && { agent: agent.name }),
              ...(agent?.readOnly && { readOnly: true }),
            })
          } catch (cause) {
            return `The subagent was not started: ${cause instanceof Error ? cause.message : String(cause)}`
          }
          started.set(child, agent ? `${agent.name}: ${description}` : description)
          const job = follow(child, context)
          return (
            `A subagent has started on this, as background job ${job.id}. Its id is "${child}".\n\n` +
            "You do not have its report yet, and you do not know what it will find. Do not guess the report, do not write " +
            "one yourself, and do not send the subagent a message to ask for it. Carry on with work that does not overlap " +
            "with its task, or say briefly what you have started and end your reply: you will be woken with its report " +
            "when it has finished. To give it more work later, use subagent_send with its id. To stop it, use job_stop " +
            `with the id "${job.id}".`
          )
        },
      })

      host.tools.add({
        name: "subagent_send",
        description:
          "Send a further message to a subagent you started. It still has its whole conversation, so the message can " +
          "build on what it has done: a follow-up question, a correction, or more work on the same thing. If it is " +
          "still working, the message joins what it is doing. You are told its next report when it has finished.",
        inputSchema: {
          type: "object",
          properties: {
            id: { type: "string", description: "The subagent's id, as given when it was started" },
            message: { type: "string", description: "What to tell it" },
          },
          required: ["id", "message"],
        },
        async run(input, context) {
          const child = String(input.id ?? "").trim()
          const message = typeof input.message === "string" ? input.message.trim() : ""
          if (message === "") return "The message was empty. Call subagent_send again with what to tell the subagent."
          // A round of work that is being followed, and has not been
          // stopped, takes the message in and reports once. Otherwise the
          // message starts a round, which counts towards how many may work at once.
          const followed = following.get(child)
          const joins = followed !== undefined && !followed.stopped && context.children.busy(child)
          if (!joins) {
            const refusal = tooMany(context)
            if (refusal !== undefined) return refusal
          }
          try {
            await context.children.send(child, message)
          } catch (cause) {
            return `The message was not sent: ${cause instanceof Error ? cause.message : String(cause)} Use the id given when the subagent was started, or start a new one.`
          }
          if (joins) return "The message was sent. The subagent was still working, and takes it in as it goes. You will be told its report when it has finished."
          const job = follow(child, context)
          return `The message was sent, and the subagent is working on it as background job ${job.id}. You will be told its report when it has finished.`
        },
      })
    },
  }
}
