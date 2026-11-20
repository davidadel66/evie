import type { Plugin } from "../../core/types.ts"
import { dataFolder } from "../../core/evie.ts"
import { createGuard, isSavedOutput } from "../guard.ts"
import { MODES, actsOf, alwaysRule, effectFor, questionFor, type Mode, type Rule } from "./rules.ts"

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
  const blockedByGuard = createGuard(options.dataFolder === undefined ? {} : { dataFolder: options.dataFolder })
  const startsIn = options.mode ?? "manual"
  const data = options.dataFolder ?? dataFolder({})

  return {
    name: "permissions",
    setup(host) {
      const folder = host.project?.folder ?? null
      const key = folder ?? "general"
      // Shared by every session in this project, and added to by "always" answers.
      const remembered: Rule[] = [...(options.remembered?.[key] ?? [])]
      // The mode of each session that has changed it.
      const modes = new Map<string, Mode>()
      // A subagent has no mode of its own. It works in auto, so that it is
      // not asking about every step from the background, unless the session
      // that started it is read-only, in which case it is read-only too.
      const modeOf = (session: string, parent?: string): Mode =>
        parent === undefined ? (modes.get(session) ?? startsIn) : (modes.get(parent) ?? startsIn) === "read-only" ? "read-only" : "auto"

      host.commands.add({
        name: "mode",
        description: "Show or change how freely Evie may act: read-only, manual or auto.",
        run(args, context) {
          const wanted = args.trim()
          if (wanted === "") return `The mode is ${modeOf(context.session)}. ${ABOUT}`
          if (!MODES.includes(wanted as Mode)) return `There is no mode "${wanted}". ${ABOUT}`
          modes.set(context.session, wanted as Mode)
          return `The mode is now ${wanted}.`
        },
      })

      host.on("before_tool", async (call, context) => {
        const guarded = blockedByGuard(call, folder, context.session)
        if (guarded !== undefined) return { deny: guarded }

        const mode = modeOf(context.session, context.parent)
        const rules = [...(options.rules ?? []), ...remembered]
        const judged = actsOf(call, folder).map((act) => {
          const effect = effectFor(act, rules, mode, folder)
          // Reading the output Evie saved for this session is not reaching
          // outside the project, so it is not asked about. A rule that
          // denies it still does.
          const ownOutput = act.action === "read" && effect !== "deny" && isSavedOutput(data, context.session, act.target)
          return { act, effect: ownOutput ? ("allow" as const) : effect }
        })

        if (judged.some(({ effect }) => effect === "deny")) {
          return {
            deny:
              mode === "read-only"
                ? "Not allowed: Evie is in read-only mode, so nothing outside reading the project can be done. Do not try " +
                  "another way. Tell the owner what you would do; they can change the mode with /mode."
                : "Not allowed: one of the owner's rules denies this. Do not try another way to do the same thing. Tell the owner what you wanted to do.",
          }
        }

        const asking = judged.filter(({ effect }) => effect === "ask").map(({ act }) => act)
        if (asking.length === 0) return

        // One "always" choice covers everything this call was asked about.
        const always = asking.map((act) => alwaysRule(act, folder))
        const labels = [...new Set(always.map(({ label }) => label))]
        const ALWAYS = `Always ${folder === null ? "outside a project" : "in this project"}: allow ${labels.join(", and ")}`
        const given = await context.ask({ text: questionFor(call, asking, folder), choices: [ONCE, ALWAYS, NO], freeText: true })
        // A plain word typed in place of a number means what it says.
        const word = given?.trim().toLowerCase()
        const answer = word === "y" || word === "yes" ? ONCE : word === "n" || word === "no" ? NO : word === "always" ? ALWAYS : given

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
  "auto, which works freely inside the project and asks before reaching outside it. Change it with /mode and a name."
