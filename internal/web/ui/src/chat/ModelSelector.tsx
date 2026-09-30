import { useId } from "react";
import type { ChatModel } from "../api/models";

type Props = {
  models: ChatModel[];
  value?: string;
  disabled?: boolean;
  loading?: boolean;
  saving?: boolean;
  problem?: string;
  onChange: (model: string) => void;
  onRetry: () => void;
};

const providerNames: Record<string, string> = {
  openai: "OpenAI", anthropic: "Anthropic", google: "Google", deepseek: "DeepSeek",
  moonshotai: "Moonshot AI", "meta-llama": "Meta", mistralai: "Mistral AI",
  "x-ai": "xAI", qwen: "Qwen", nvidia: "NVIDIA", cohere: "Cohere",
};

function groupModels(models: ChatModel[], current?: string) {
  const entries = [...models];
  if (current && !entries.some(model => model.id === current)) {
    entries.push({id:current, name:current, provider:current.split("/")[0]});
  }
  const groups = new Map<string, ChatModel[]>();
  for (const model of entries) {
    const provider = providerNames[model.provider] ?? model.provider;
    groups.set(provider, [...(groups.get(provider) ?? []), model]);
  }
  return [...groups].sort(([a], [b]) => a.localeCompare(b)).map(([provider, choices]) => ({
    provider, models: choices.sort((a,b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id)),
  }));
}

export function ModelSelector({ models, value, disabled, loading, saving, problem, onChange, onRetry }: Props) {
  const id = useId();
  return <div className="mt-2 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2 text-xs">
    <label htmlFor={id} className="text-muted-text">Model</label>
    <select id={id} aria-label="Chat model" value={value ?? ""} disabled={disabled || loading || saving || !value}
      onChange={event => onChange(event.target.value)}
      className="bg-card text-body border-hair focus-visible:outline-teal min-w-0 max-w-full flex-1 rounded-md border px-2 py-1.5 sm:max-w-[360px] disabled:opacity-60">
      {!value && <option value="">{loading ? "Loading models…" : "Models unavailable"}</option>}
      {groupModels(models, value).map(group => <optgroup key={group.provider} label={group.provider}>
        {group.models.map(model => <option key={model.id} value={model.id}>{model.name}</option>)}
      </optgroup>)}
    </select>
    {saving && <span role="status" className="text-muted-text">Switching model…</span>}
    {problem && <div role="alert" className="text-amber-ink w-full">
      {problem} <button type="button" disabled={disabled || loading || saving} onClick={onRetry} className="underline">Retry</button>
    </div>}
  </div>;
}
