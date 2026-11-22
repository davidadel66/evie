import type { HookContext, Plugin, Provider } from "../core/types.ts"

// The built-in naming plugin. After the owner's first message in a session,
// a small, cheap model writes the session a name of a few words, in the
// background, so that the reply is not held up. Until the name arrives the
// session is called by the opening words of that message, and it keeps
// them when the name does not come: a request that fails or takes too long
// is given up, says nothing to the owner, and is not tried again. A session
// is named once, and never over a name the owner gave it.
//
// The prompt is adapted from the one T3 Code names its threads with,
// rewritten in evie's words. T3 Code's licence (MIT) is in
// LICENSES/t3code-MIT.txt.

export type NamingOptions = {
  // The model that writes the names.
  model?: string
  // How long, in milliseconds, the model is given to write one.
  within?: number
}

export const NAMING_MODEL = "anthropic/claude-haiku-5.5"
const WITHIN = 30_000
// How much of the first message the model is given, in characters.
const LONGEST_MESSAGE = 4000
// A name longer than this, in characters, is not a name but an answer.
const LONGEST_NAME = 60

const prompt = (project: string | undefined) => `Write a name for a conversation that will help its owner recognise it weeks later. You are given the first message the owner sent in it.

Before answering, work out silently:
- Subject: what system, feature, question or problem is this really about?
- Outcome: what does the owner want to understand, decide or change?
- Incidental instructions: what only says how the work should be done?

Name the subject and the outcome. Leave out the incidental instructions.

Rules:
- 3 to 8 words, fewer than 40 characters.
- A short noun phrase or a clear action phrase, in sentence case.
- When the message lists several steps or symptoms, name the goal they share.
- Name what is to be changed, not the plan, report, branch or pull request used to change it.
- Models, agents, tools and output formats do not belong in the name unless they are the subject.
- For a review, name what is reviewed and the concern. For research, name the question, not the research.
- Do not say the work is done.
- Do not copy the message, or cut it short.
- ${project ? `Never include the project's name, ${JSON.stringify(project)}: it is shown beside the name.` : "Leave out the names of projects."} No quotes, labels, filler or full stop at the end.
- Do not answer the message or start on the work.

Reply with the name alone, on one line.`

export function naming(options: NamingOptions = {}): Plugin {
  const model = options.model ?? NAMING_MODEL
  const within = options.within ?? WITHIN
  return {
    name: "naming",
    setup(host) {
      // The sessions a name has been asked for since evie started, so that
      // none is asked for twice, whatever comes of the first.
      const asked = new Set<string>()
      host.on("before_request", (_request, context) => {
        // A subagent's session is not listed, so it needs no name. Nor is a
        // session that has one, or has had a reply: it is past its start.
        if (context.parent !== undefined || asked.has(context.session)) return
        if (context.events.some((event) => event.type === "named" || event.type === "assistant_message")) return
        const first = context.events.find((event) => event.type === "user_message")
        const provider = host.providers.list()[0]
        if (first?.type !== "user_message" || !provider) return
        asked.add(context.session)
        void nameSession(provider, first.body.text, context)
      })
      async function nameSession(provider: Provider, message: string, context: HookContext) {
        try {
          let written = ""
          let finished = false
          const request = {
            model,
            system: prompt(host.project?.name),
            maxTokens: 100,
            messages: [{ role: "user" as const, text: message.slice(0, LONGEST_MESSAGE) }],
            tools: [],
          }
          const signal = AbortSignal.timeout(within)
          for await (const chunk of provider.stream(request, signal)) {
            if (signal.aborted) return
            if (chunk.type === "text") written += chunk.text
            if (chunk.type === "finish") finished = chunk.reason === "stop"
          }
          // A provider that pays no heed to the time running out has its late answer dropped.
          const name = finished && !signal.aborted ? tidy(written) : ""
          if (name !== "" && name.length <= LONGEST_NAME) context.name(name)
        } catch {
          // Given up: the session keeps the opening words of its first message.
        }
      }
    },
  }
}

// The name as the model wrote it, without what a small model adds around
// it: marks such as quotes or asterisks, a label inside or outside them, as
// in `**Title:** Fix login`, and a full stop at the end.
function tidy(written: string): string {
  const line = written.trim().split("\n")[0] ?? ""
  const unmarked = (text: string) => text.replace(/^["'`*_\s]+|["'`*_\s]+$/g, "")
  const unlabelled = unmarked(line).replace(/^(name|title)\s*:\s*/i, "")
  return unmarked(unmarked(unlabelled).replace(/[.!?:;,]+$/, ""))
}
