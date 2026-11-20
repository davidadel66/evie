import type { Plugin, Tool } from "../core/types.ts"

// The built-in plugin that lets the model ask the owner a question with
// choices. It adds one tool. The asking and the waiting are the core's: the
// tool only passes the question on and words the answer for the model.

const MOST_CHOICES = 6

export function ask(): Plugin {
  return {
    name: "ask",
    setup(host) {
      host.tools.add(askTool)
    },
  }
}

const askTool: Tool = {
  name: "ask",
  description:
    "Ask the owner a question and wait for the answer. Give two to six short choices; the owner picks one or answers " +
    "in their own words. Use it when the work cannot go on well without a decision that is the owner's to make, such " +
    "as which of several approaches to take. Do not use it for something you can find out yourself, or to ask whether " +
    "to carry on. Ask one question at a time.",
  inputSchema: {
    type: "object",
    properties: {
      question: { type: "string", description: "The question, complete enough to answer without reading anything else" },
      choices: {
        type: "array",
        items: { type: "string" },
        description: "Two to six answers to choose from, each a few words. Put the one you recommend first",
      },
    },
    required: ["question", "choices"],
  },
  async run(input, context) {
    const question = typeof input.question === "string" ? input.question.trim() : ""
    const choices = Array.isArray(input.choices) ? input.choices.filter((choice): choice is string => typeof choice === "string" && choice.trim() !== "") : []
    if (question === "") return "The question was empty. Call ask again with the question in words."
    if (choices.length < 2 || choices.length > MOST_CHOICES) {
      return `Give between two and ${MOST_CHOICES} choices. Call ask again, or ask in plain text in your reply if the question has no fixed answers.`
    }

    const answer = await context.ask({ text: question, choices, freeText: true })
    if (answer === null) {
      return (
        "The owner did not answer: the question could not be shown, or it was dismissed. Do not ask it again with " +
        "this tool. Carry on with your best judgement and say what you assumed, or ask in plain text in your reply."
      )
    }
    return choices.includes(answer) ? `The owner chose: ${answer}` : `The owner answered in their own words: ${answer}`
  },
}
