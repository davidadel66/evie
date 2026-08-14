# F26 - Session export and sharing

Status: local-export candidate, remote sharing rejected by default.

## Purpose

Export a coding session or build report for diagnosis and review without leaking
EVIE's personal context.

## How OpenCode does it

OpenCode can export session JSON with optional redaction and can upload sessions,
messages, parts, diffs, and model metadata to a hosted share service.

Sources:

- [`cli/cmd/export.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/cli/cmd/export.ts)
- [`share/share-next.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/share/share-next.ts)

## Proposed EVIE adaptation

Start with local sanitized export only:

- versioned JSON event export for debugging/replay;
- human-readable Markdown build report;
- optional diff/artifact bundle;
- explicit inclusion/exclusion manifest;
- secret/path/provider-payload redaction report.

Remote sharing is a poor default for a personal assistant with finance tools,
shell environment, memory, and project source. If ever added, it requires an
explicit per-export preview and destination, no hidden upload, and a threat model
separate from local export.

## Acceptance evidence

- Export does not mutate or omit canonical local history.
- Redaction is deterministic and reports fields removed.
- Opaque provider payload/reasoning is excluded by default.
- A fixture containing API keys, home paths, finance rows, and shell output does
  not leak them in a sanitized export.
- Remote network activity is impossible in the local-export path.

## Open decisions

1. Should absolute project paths be replaced with project-relative paths or
   pseudonyms?
2. Which artifacts are safe enough for a default build-report export?
