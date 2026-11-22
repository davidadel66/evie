// The model picker's rules, apart from its screen: the models a session's
// provider offers, grouped by the company that makes them, as T3 Code's
// picker groups its models by provider, and searched across them all.

export type Model = { id: string; name: string }

// A company that makes models, by the first part of its models' ids, such
// as "anthropic" in anthropic/claude-sonnet-5, with its name as the
// provider gives it in theirs ("Anthropic: Claude Sonnet 5"), and its
// models, each by the rest of its name.
export type Maker = { key: string; name: string; models: Model[] }

// The parts of a model's id and name: the maker's key and name, and the
// model's own name without its maker's.
function partsOf(model: Model): { key: string; maker: string; name: string } {
  const key = model.id.split("/")[0]!
  const colon = model.name.indexOf(": ")
  return colon === -1 ? { key, maker: key, name: model.name } : { key, maker: model.name.slice(0, colon), name: model.name.slice(colon + 2) }
}

// The makers, in order of name, each with its models as they were listed.
export function makersOf(models: readonly Model[]): Maker[] {
  const makers = new Map<string, Maker>()
  for (const model of models) {
    const { key, maker, name } = partsOf(model)
    if (!makers.has(key)) makers.set(key, { key, name: maker, models: [] })
    makers.get(key)!.models.push({ id: model.id, name })
  }
  return [...makers.values()].sort((a, b) => a.name.localeCompare(b.name))
}

// The models with every word typed in their id or name, in any case, each
// with its maker's name: those whose own name starts with the first word
// first, then the rest, each as listed. Nothing typed finds nothing.
export function searchModels(models: readonly Model[], typed: string): (Model & { maker: string })[] {
  const words = typed.toLowerCase().split(/\s+/).filter((word) => word !== "")
  if (words.length === 0) return []
  const found = models.flatMap((model) => {
    const { maker, name } = partsOf(model)
    const text = `${model.id} ${model.name}`.toLowerCase()
    return words.every((word) => text.includes(word)) ? [{ id: model.id, name, maker, starts: name.toLowerCase().startsWith(words[0]!) }] : []
  })
  return [...found.filter((model) => model.starts), ...found.filter((model) => !model.starts)].map(({ starts: _, ...model }) => model)
}

// The name the model chip shows for a model, as T3 Code's chip shows its
// display name: its own, as the provider lists it, without its maker's;
// or, for one not listed, or before the list has come, its id after the
// maker's.
export function modelName(models: readonly Model[] | undefined, id: string): string {
  const listed = models?.find((model) => model.id === id)
  return listed === undefined ? id.slice(id.indexOf("/") + 1) : partsOf(listed).name
}
