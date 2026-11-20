import { join, relative } from "node:path"
import { dataFolder } from "../core/evie.ts"
import type { Plugin } from "../core/types.ts"
import { entriesOf, foldersFor, splitHeader, textOf, tidyName } from "./definitions.ts"

// The built-in skills plugin. A skill is the owner's written instructions
// for one kind of task: how to cut a release, how to review a change. It is
// a folder holding a `SKILL.md`, which is a short header with a name and a
// description followed by the instructions, and any other files those
// instructions refer to, such as a script or a template.
//
// The model is not given every skill in full. It is shown each one's name
// and description, a line apiece, and loads the rest with the `skill` tool
// when a task calls for it. So twenty skills cost twenty lines, not twenty
// documents.
//
// The owner can also start a skill directly, by typing a slash and its
// name. Skills are read from `skills` in the data folder, for every
// project, and from `.evie/skills` in a project the owner has trusted. They
// are read each time they are needed, so one written a moment ago works.

export type SkillsOptions = {
  // The folder Evie keeps its state in. Defaults to the usual place.
  dataFolder?: string
  // Whether the owner has trusted a project, by its folder. A project's own
  // skills are used only when they have. Defaults to no.
  trusted?: (folder: string) => boolean
}

type Skill = { name: string; description: string; instructions: string; folder: string }

// How many of a skill's files are listed when it is loaded.
const MOST_FILES = 40

export function skills(options: SkillsOptions = {}): Plugin {
  const data = options.dataFolder ?? dataFolder({})
  const trusted = options.trusted ?? (() => false)

  return {
    name: "skills",
    setup(host) {
      // The skills there are now. A project's skill replaces the owner's of the same name.
      async function all(): Promise<Skill[]> {
        const found = new Map<string, Skill>()
        for (const root of foldersFor("skills", data, host.project?.folder ?? null, trusted)) {
          for (const entry of await entriesOf(root)) {
            if (!entry.isDirectory()) continue
            const folder = join(root, entry.name)
            const text = await textOf(join(folder, "SKILL.md"))
            if (text === undefined) continue
            const { header, body } = splitHeader(text)
            const name = tidyName(header.name ?? entry.name)
            if (name === "" || body === "") continue
            found.set(name, { name, description: header.description ?? "", instructions: body, folder })
          }
        }
        return [...found.values()].toSorted((a, b) => a.name.localeCompare(b.name))
      }

      // The other files in a skill's folder, which its instructions may refer to.
      async function filesOf(skill: Skill): Promise<string[]> {
        const files: string[] = []
        const walk = async (folder: string, depth: number): Promise<void> => {
          for (const entry of (await entriesOf(folder)).toSorted((a, b) => a.name.localeCompare(b.name))) {
            if (files.length >= MOST_FILES || entry.name.startsWith(".")) continue
            const path = join(folder, entry.name)
            if (entry.isDirectory() && depth < 3) await walk(path, depth + 1)
            else if (entry.isFile() && relative(skill.folder, path) !== "SKILL.md") files.push(path)
          }
        }
        await walk(skill.folder, 0)
        return files
      }

      // A skill as it is put in front of the model.
      async function loaded(skill: Skill): Promise<string> {
        const files = await filesOf(skill)
        const others = files.length > 0 ? `\n\nOther files in this skill, to read or run when its instructions say so:\n${files.map((file) => `- ${file}`).join("\n")}` : ""
        return `The skill "${skill.name}", from ${skill.folder}. Its instructions:\n\n${skill.instructions}${others}`
      }

      host.on("before_request", async (request) => {
        const skills = await all()
        if (skills.length === 0) return
        const about =
          "Skills the owner has set up. Each is their instructions for one kind of task. When a task matches one, load it " +
          `with the skill tool before you start, and follow it:\n${skills.map((skill) => `- ${skill.name}: ${skill.description || "(no description)"}`).join("\n")}`
        return { ...request, system: request.system ? `${request.system}\n\n${about}` : about }
      })

      host.tools.add({
        name: "skill",
        description:
          "Load one of the owner's skills: their written instructions for a kind of task, with the files that go with " +
          "them. Load a skill when the task matches its description, or when the owner names it, before you start the " +
          "task. A skill already in the conversation does not need loading again.\n\n" +
          `A skill is a folder holding a file named SKILL.md, in ${join(data, "skills")} for every project, or in ` +
          '.evie/skills in a project. SKILL.md is a header between two lines of "---" with "name:" and ' +
          '"description:", followed by the instructions.',
        inputSchema: {
          type: "object",
          properties: { name: { type: "string", description: "The name of the skill, as listed" } },
          required: ["name"],
        },
        async run(input) {
          const skills = await all()
          const skill = skills.find((each) => each.name === tidyName(String(input.name ?? "")))
          if (!skill) {
            const names = skills.map((each) => each.name).join(", ")
            return `There is no skill named "${String(input.name ?? "")}". ${names ? `The skills are: ${names}.` : "The owner has set up no skills."}`
          }
          return loaded(skill)
        },
      })

      host.commands.add({
        name: "skills",
        description: "List your skills. To start one, type a slash and its name, with anything you want to add.",
        // A slash and a skill's name starts that skill.
        covers: async (name) => (await all()).some((skill) => skill.name === tidyName(name)),
        async run(args, context) {
          const skills = await all()
          if (context.invokedAs !== "skills") {
            const skill = skills.find((each) => each.name === tidyName(context.invokedAs))
            if (!skill) return `There is no skill named "${context.invokedAs}".`
            // The skill goes into the conversation, and what the owner typed
            // after its name is sent as their message, which starts the turn.
            context.tell(`The owner asked for a skill. Follow it for what they say next. ${await loaded(skill)}`)
            context.send(args.trim() || `Use the skill "${skill.name}".`)
            return `Using the skill ${skill.name}.`
          }
          if (skills.length === 0) return `You have no skills. A skill is a folder holding a SKILL.md, in ${join(data, "skills")} or in .evie/skills in a trusted project.`
          const commands = new Set(host.commands.list().map((command) => command.name))
          return skills
            .map((skill) => {
              // A command of the same name answers to the slash, so the skill is asked for in words.
              const how = commands.has(skill.name) ? `(/${skill.name} is a command, so ask for this one in words)` : `/${skill.name}`
              return `${how}  ${skill.description || "(no description)"}`
            })
            .join("\n")
        },
      })
    },
  }
}
