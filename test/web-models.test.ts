import { expect, test } from "bun:test"
import { makersOf, modelName, searchModels } from "../web/src/models.ts"

// The model picker's rules, apart from its screen: the models a session's
// provider offers, grouped by the company that makes them, and searched.

const MODELS = [
  { id: "openai/gpt-5.2", name: "OpenAI: GPT-5.2" },
  { id: "anthropic/claude-sonnet-5", name: "Anthropic: Claude Sonnet 5" },
  { id: "anthropic/claude-haiku-5.5", name: "Anthropic: Claude Haiku 5.5" },
  { id: "openai/gpt-5.2-mini", name: "OpenAI: GPT-5.2 Mini" },
  { id: "local-thing", name: "local-thing" },
]

test("the models are grouped by their maker, named as the provider names it, the makers in order of name and each one's models as listed", () => {
  expect(makersOf(MODELS)).toEqual([
    {
      key: "anthropic",
      name: "Anthropic",
      models: [
        { id: "anthropic/claude-sonnet-5", name: "Claude Sonnet 5" },
        { id: "anthropic/claude-haiku-5.5", name: "Claude Haiku 5.5" },
      ],
    },
    { key: "local-thing", name: "local-thing", models: [{ id: "local-thing", name: "local-thing" }] },
    {
      key: "openai",
      name: "OpenAI",
      models: [
        { id: "openai/gpt-5.2", name: "GPT-5.2" },
        { id: "openai/gpt-5.2-mini", name: "GPT-5.2 Mini" },
      ],
    },
  ])
})

test("a search covers every maker: each word typed is in a model's id or name, in any case, and a name that starts with it comes first", () => {
  expect(searchModels(MODELS, "")).toEqual([])
  expect(searchModels(MODELS, "haiku").map((model) => model.id)).toEqual(["anthropic/claude-haiku-5.5"])
  expect(searchModels(MODELS, "GPT mini")).toEqual([{ id: "openai/gpt-5.2-mini", name: "GPT-5.2 Mini", maker: "OpenAI" }])
  expect(searchModels(MODELS, "claude").map((model) => model.id)).toEqual(["anthropic/claude-sonnet-5", "anthropic/claude-haiku-5.5"])
  expect(searchModels(MODELS, "5.5").map((model) => model.id)).toEqual(["anthropic/claude-haiku-5.5"])
  expect(searchModels(MODELS, "anthropic/").map((model) => model.id)).toEqual(["anthropic/claude-sonnet-5", "anthropic/claude-haiku-5.5"])
  expect(searchModels(MODELS, "mini gpt").map((model) => model.id)).toEqual(["openai/gpt-5.2-mini"])
  expect(searchModels(MODELS, "L").map((model) => model.id)).toEqual(["local-thing", "anthropic/claude-sonnet-5", "anthropic/claude-haiku-5.5"])
  expect(searchModels(MODELS, "nothing like it")).toEqual([])
})

test("the model chip shows the model's own name, as the provider lists it, without its maker's; one not listed, or before the list has come, by its id after the maker's", () => {
  expect(modelName(MODELS, "anthropic/claude-haiku-5.5")).toBe("Claude Haiku 5.5")
  expect(modelName(MODELS, "local-thing")).toBe("local-thing")
  expect(modelName(MODELS, "mistral/large-3")).toBe("large-3")
  expect(modelName(undefined, "anthropic/claude-haiku-5.5")).toBe("claude-haiku-5.5")
})
