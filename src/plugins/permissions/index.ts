import { readFileSync } from "node:fs"
import { mkdir, rm, writeFile } from "node:fs/promises"
import { join } from "node:path"
import type { Plugin } from "../../core/types.ts"
import { dataFolder } from "../../core/evie.ts"
import { createGuard, isDefinition, isSavedOutput } from "../guard.ts"
import { AUTO_ASKS, MODES, actsOf, alwaysRule, effectFor, questionFor, type Mode, type Rule } from "./rules.ts"

// The built-in permissions plugin. Before a tool runs, it decides whether
// the call is allowed, denied, or needs the owner's say-so, and asks when it
// does. It sits on the `before_tool` hook and uses nothing else of the core.
//
// The order of things:
//   1. The guard's checks. What they block is denied in every mode.
//   2. The rules: from the global settings, then the project's own, then the
//      owner's remembered "always" answers. A rule that denies always wins;
//      otherwise the last one that matches decides.
//   3. With no rule, the session's mode decides (see rules.ts).
//
// A mode set with /mode is kept in a small file for the session in the
// data folder, so that it holds when evie starts again. The file goes when
// the session is deleted.
//
// When the answer is to ask and nobody can be asked, the call is denied.

export { MODES, type Mode, type Rule } from "./rules.ts"

export type PermissionsOptions = {
  // The mode a session starts in. Defaults to manual.
  mode?: Mode
  // The rules from the settings files, in order.
  rules?: readonly Rule[]
  // The owner's remembered answers, by project folder. A session with no
  // project uses the key "general".
  remembered?: Record<string, Rule[]>
  // Saves a remembered answer, so it is there the next time Evie starts.
  remember?: (project: string, rules: Rule[]) => void | Promise<void>
  // The folder Evie keeps its state and keys in, which no tool may touch.
  dataFolder?: string
}

const ONCE = "Yes, once"
const NO = "No"

export function permissions(options: PermissionsOptions = {}): Plugin {
  // This plugin asks before each write to the owner's agents and skills, so the guard may let them through.
  const blockedByGuard = createGuard({ ...(options.dataFolder !== undefined && { dataFolder: options.dataFolder }), definitionsCanBeWritten: true })
  const startsIn = options.mode ?? "manual"
  const newSessions = options.mode === undefined ? "New sessions start in manual, the default." : `New sessions start in ${options.mode}, which "permissions.mode" in evie.json sets.`
  const data = options.dataFolder ?? dataFolder({})
  const keptIn = join(data, "modes")
  // A session's id is the name of its file, so one that could not be is never used.
  const fileOf = (session: string) => (/^[\w-]+$/.test(session) ? join(keptIn, session) : undefined)
  const kept = (session: string): Mode | undefined => {
    const file = fileOf(session)
    try {
      const mode = file === undefined ? "" : readFileSync(file, "utf8").trim()
      return MODES.includes(mode as Mode) ? (mode as Mode) : undefined
    } catch {
      return undefined
    }
  }

  return {
    name: "permissions",
    setup(host) {
      const folder = host.project?.folder ?? null
      const key = folder ?? "general"
      // Shared by every session in this project, and added to by "always" answers.
      const remembered: Rule[] = [...(options.remembered?.[key] ?? [])]
      // The mode of each session, read from its file the first time it is
      // needed and changed by /mode. What is here wins over the file.
      const modes = new Map<string, Mode>()
      const ownMode = (session: string): Mode => {
        if (!modes.has(session)) modes.set(session, kept(session) ?? startsIn)
        return modes.get(session)!
      }
      // A subagent has no mode the owner sets. It works in auto, so that it
      // is not asking about every step from the background; or read-only,
      // when the session that started it was read-only at the time. Which
      // of the two is settled each time it is given work, and not by what
      // the starting session's mode happens to be later: a read-only
      // subagent does not become free to change things because its starter
      // left read-only. It does become read-only at once if its starter does.
      const subagentModes = new Map<string, Mode>()
      const forSubagent = (parent: string): Mode => (ownMode(parent) === "read-only" ? "read-only" : "auto")
      const modeOf = (session: string, parent?: string): Mode => {
        if (parent === undefined) return ownMode(session)
        if (!subagentModes.has(session)) subagentModes.set(session, forSubagent(parent))
        return forSubagent(parent) === "read-only" ? "read-only" : subagentModes.get(session)!
      }

      host.commands.add({
        name: "mode",
        description: "Show or change how freely evie may act: read-only, manual or auto.",
        async run(args, context) {
          const wanted = args.trim()
          if (wanted === "") return `The mode is ${modeOf(context.session)}. ${ABOUT} ${newSessions}`
          if (!MODES.includes(wanted as Mode)) return `There is no mode "${wanted}". ${ABOUT} ${newSessions}`
          modes.set(context.session, wanted as Mode)
          const file = fileOf(context.session)
          const saved =
            file !== undefined &&
            (await mkdir(keptIn, { recursive: true })
              .then(() => writeFile(file, wanted))
              // A file that could not be written over would give the old mode when evie starts again.
              .then(
                () => true,
                () => rm(file, { force: true }).then(() => false, () => false),
              ))
          return `The mode is now ${wanted}, for this session${saved ? "" : " until evie stops, since it could not be saved"}. ${newSessions}`
        },
      })

      host.on("session_deleted", async (session) => {
        modes.delete(session)
        const file = fileOf(session)
        if (file !== undefined) await rm(file, { force: true })
      })

      host.on("before_tool", async (call, context) => {
        const guarded = blockedByGuard(call, folder, context.session, context.parent)
        if (guarded !== undefined) return { deny: guarded }

        // A subagent started as an agent that only reads stays that way.
        const mode = context.readOnly ? "read-only" : modeOf(context.session, context.parent)
        const rules = [...(options.rules ?? []), ...remembered]
        const judged = actsOf(call, folder).map((act) => {
          const effect = effectFor(act, rules, mode, folder)
          // Reading the output Evie saved for this session is not reaching
          // outside the project, so it is not asked about. A rule that
          // denies it still does.
          const ownOutput = (act.action === "read" || act.action === "outside") && effect !== "deny" && isSavedOutput(data, context.session, act.target, context.parent)
          if (ownOutput) return { act, effect: "allow" as const, standing: false }
          // The owner's agents and skills are not another folder to work in:
          // reading them and changing them is asked about as below.
          if (act.action === "outside" && effect !== "deny" && isDefinition(data, act.target)) return { act, effect: "allow" as const, standing: false }
          // An agent or a skill in the data folder applies in every project
          // from then on. Reading one is free. Writing one is asked about
          // every time, whatever the mode and whatever was answered before.
          if (act.kind === "path" && isDefinition(data, act.target) && effect !== "deny") {
            if (act.action === "read") return { act, effect: "allow" as const, standing: false }
            if (mode !== "read-only") return { act, effect: "ask" as const, standing: true }
          }
          return { act, effect, standing: false }
        })

        if (judged.some(({ effect }) => effect === "deny")) {
          return {
            deny:
              mode === "read-only"
                ? "Not allowed: evie is in read-only mode, so nothing outside reading the project can be done. Do not try " +
                  "another way. Tell the owner what you would do; they can change the mode with /mode."
                : "Not allowed: one of the owner's rules denies this. Do not try another way to do the same thing. Tell the owner what you wanted to do.",
          }
        }

        const asking = judged.filter(({ effect }) => effect === "ask").map(({ act }) => act)
        if (asking.length === 0) {
          if (call.name === "subagent_send") subagentModes.set(String(call.input.id ?? ""), forSubagent(context.session))
          return
        }

        // One "always" choice covers everything this call was asked about.
        const always = asking.map((act) => alwaysRule(act, folder, mode)).filter(({ rule }, at, all) => all.findIndex((other) => other.rule.action === rule.action && other.rule.match === rule.match) === at)
        const labels = [...new Set(always.map(({ label }) => label))]
        const ALWAYS = `Always ${folder === null ? "outside a project" : "in this project"}: allow ${labels.join(", and ")}`
        // There is no "always" for a standing instruction: each one is looked at.
        const everyTime = judged.some(({ effect, standing }) => effect === "ask" && standing)
        const text = everyTime
          ? `${questionFor(call, asking, folder)}\n\nThis is an agent or a skill of yours. It applies in every project from now on, so it is asked about each time.`
          : questionFor(call, asking, folder)
        const given = await context.ask({ text, choices: everyTime ? [ONCE, NO] : [ONCE, ALWAYS, NO], freeText: true, kind: "permission" })
        // A plain word typed in place of a number means what it says.
        const word = given?.trim().toLowerCase()
        const answer = word === "y" || word === "yes" ? ONCE : word === "n" || word === "no" ? NO : word === "always" && !everyTime ? ALWAYS : given

        // Work given to a subagent, now that it is allowed: its mode is settled here.
        if ((answer === ONCE || answer === ALWAYS) && call.name === "subagent_send") subagentModes.set(String(call.input.id ?? ""), forSubagent(context.session))
        if (answer === ONCE) return
        if (answer === ALWAYS) {
          const rulesToKeep = always.map(({ rule }) => rule).filter((rule) => !remembered.some((kept) => kept.action === rule.action && kept.match === rule.match))
          remembered.push(...rulesToKeep)
          // If the answer cannot be saved it still holds until Evie stops.
          await Promise.resolve(options.remember?.(key, rulesToKeep)).catch(() => {})
          return
        }
        if (answer === null) {
          return {
            deny:
              "Not done: this needs the owner's say-so, and they could not be asked or did not answer. Do not try another " +
              "way to do the same thing. Carry on with what does not need it, and say what is left for the owner.",
          }
        }
        return {
          deny:
            answer === NO
              ? "The owner said no to this. Do not try another way to do the same thing. Ask what they would like instead."
              : `The owner said no to this, and added: "${answer}". Do not try another way to do the same thing unless that is what they asked for.`,
        }
      })
    },
  }
}

const ABOUT =
  "The modes are: read-only, which changes nothing; manual, which asks before every change and every command; and " +
  `auto, which does anything without asking, pushes and pull requests included, and asks only before ${AUTO_ASKS}; ` +
  '"Always" then allows that folder or file from then on. Change it for this session with /mode and a name.'
