# F24 - Providers, models, and capability metadata

Status: deferred breadth, unapproved. Priority: keep narrow.

## Purpose

Select models deliberately per profile and expose the limits the runtime needs
without recreating OpenCode's provider marketplace.

## How OpenCode does it

OpenCode maintains a broad provider/model catalog with SDK adapters, model
discovery, auth/OAuth plugins, capability flags, context/output limits, pricing,
variants, and provider-specific request transforms. Profiles can override the
model and options; child agents otherwise inherit the caller's model.

Source: [`provider/provider.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/provider/provider.ts).

## EVIE assessment

Remain OpenRouter-first. F03's neutral protocol is valuable now; a provider
catalog is not. Add another adapter only after selecting a real second provider,
such as an explicitly configured local OpenAI-compatible endpoint.

## Proposed capability record

Each configured model needs only facts consumed by EVIE:

- provider/model ID and display name;
- context and maximum output tokens;
- tool calling, streaming, reasoning, attachments, and structured-output support;
- usage/cost availability;
- privacy route and local/remote status;
- optional small-model role suitability.

Model choice can vary by profile:

- `build`/review: strongest approved coding model;
- `explore`: cheaper model after eval evidence;
- `compaction`: active model or tested summarizer;
- `title`: cheap model or deterministic fallback.

Never silently fall back from a selected local/private route to a remote
provider.

## Acceptance evidence

- Unsupported profile/model combinations fail before a run.
- Context management reads model limits from one explicit source.
- Provider fallback cannot change privacy without user approval.
- Model/profile choices and costs are persisted per step.

## Open decisions

1. Which second provider earns the adapter first?
2. Are model prices trusted from OpenRouter metadata or configured locally?
3. Which roles may automatically choose a cheaper model?
