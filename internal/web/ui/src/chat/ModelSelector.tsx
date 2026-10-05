// The chat model picker, opened by `/model`. It floats above the composer and
// hands focus back to wherever it was opened from.

import { useEffect, useId, useRef, useState } from "react";
import type { ChatModel } from "../api/models";
import { Check } from "../ui/Icon";

type Props = {
  models: ChatModel[];
  value?: string;
  disabled?: boolean;
  loading?: boolean;
  onChange: (model: string) => void;
  onClose: () => void;
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

export function ModelPicker({ models, value, disabled, loading, onChange, onClose }: Props) {
  const id = useId();
  const [query, setQuery] = useState("");
  const root = useRef<HTMLDivElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const returnTo = useRef<Element | null>(null);
  useEffect(() => {
    returnTo.current = document.activeElement;
    search.current?.focus();
  }, []);
  useEffect(() => {
    const dismiss = (event: PointerEvent) => {
      if (!(event.target instanceof Node) || !root.current?.contains(event.target)) onClose();
    };
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, [onClose]);
  const finish = () => {
    onClose();
    if (returnTo.current instanceof HTMLElement) returnTo.current.focus();
  };
  const terms = query.trim().toLowerCase();
  const groups = groupModels(models, value)
    .map(group => ({...group, models: group.models.filter(model => `${group.provider} ${model.name} ${model.id}`.toLowerCase().includes(terms))}))
    .filter(group => group.models.length > 0);
  const options = () => [...(list.current?.querySelectorAll<HTMLButtonElement>("button[data-model]:not(:disabled)") ?? [])];
  const move = (from: EventTarget, step: number) => {
    const all = options();
    if (all.length === 0) return;
    const at = all.indexOf(from as HTMLButtonElement);
    all[at < 0 ? (step > 0 ? 0 : all.length - 1) : Math.min(all.length - 1, Math.max(0, at + step))].focus();
  };

  return <div ref={root} id={id} role="dialog" aria-label="Choose model"
    onKeyDown={event => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); finish(); }
      if (event.key === "ArrowDown" || event.key === "ArrowUp") { event.preventDefault(); move(event.target, event.key === "ArrowDown" ? 1 : -1); }
    }}
    className="bg-card border-hair-input absolute bottom-full left-0 z-30 mb-2 flex w-[min(360px,100%)] flex-col overflow-hidden rounded-xl border shadow-[0_12px_32px_rgba(0,0,0,0.45)]">
    <input ref={search} value={query} onChange={event => setQuery(event.target.value)} aria-label="Search models" placeholder="Search models"
      onKeyDown={event => { if (event.key === "Enter") { event.preventDefault(); options()[0]?.click(); } }}
      className="border-hair text-ink placeholder:text-fainter border-b bg-transparent px-3 py-2.5 text-[13px] outline-none" />
    {disabled && <p className="text-amber-ink px-3 pt-2 text-xs">The model can change once the current reply finishes.</p>}
    <div ref={list} className="max-h-[300px] overflow-y-auto p-1.5">
      {groups.map(group => <div key={group.provider} role="group" aria-label={group.provider}>
        <div className="text-faint px-2 pt-2 pb-1 text-[11px] font-medium">{group.provider}</div>
        {group.models.map(model => <button key={model.id} type="button" data-model disabled={disabled} title={model.id}
          aria-current={model.id === value ? "true" : undefined} onClick={() => { if (model.id !== value) onChange(model.id); finish(); }}
          className={`${model.id === value ? "text-teal" : "text-body"} hover:bg-hover focus-visible:bg-hover flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px] outline-none disabled:cursor-not-allowed disabled:opacity-50`}>
          <span className="min-w-0 flex-1 truncate">{model.name}</span>
          {model.id === value && <Check size={13} />}
        </button>)}
      </div>)}
      {groups.length === 0 && <p className="text-muted-text px-2 py-3 text-xs">{loading ? "Loading models…" : models.length > 0 ? "No models match." : "Models unavailable."}</p>}
    </div>
  </div>;
}

export function ModelProblem({ problem, disabled, onRetry }: { problem?: string; disabled?: boolean; onRetry: () => void }) {
  if (!problem) return null;
  return <div role="alert" className="text-amber-ink mt-2 text-xs">
    {problem} <button type="button" disabled={disabled} onClick={onRetry} className="underline">Retry</button>
  </div>;
}
