# F20 - Language-server diagnostics

Status: deferred, unapproved.

## Purpose

Return fast compiler/editor diagnostics after reads and edits without requiring
the model to run a full project command for every syntax error.

## How OpenCode does it

OpenCode discovers and lazily starts language servers, opens/warmups files after
reads, and can attach diagnostics after edits. This adds process lifecycle,
protocol framing, per-language configuration, workspace management, and output
normalization.

Source: [`lsp/lsp.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/lsp/lsp.ts).

## EVIE assessment

Defer. Go projects already have fast, deterministic `gofmt`, `go test`, and
`go vet` through Bash. EVIE should first build the verification contract and
measure whether waiting for full commands is actually a problem.

## Trigger to promote

- EVIE develops multiple languages regularly.
- Repeated syntax/type errors survive until expensive full gates.
- The project verification loop is measurably too slow for edit feedback.

## Proposed first slice if promoted

- One language only, likely `gopls`.
- Project-scoped server lifecycle and cancellation.
- Diagnostics after an approved write, rendered as advisory tool metadata.
- No rename/refactor/code-action support.
- Compiler/gate remains the authority; LSP failure never blocks saving by
  itself.

## Acceptance evidence

- Server lifecycle does not leak processes across sessions.
- Diagnostics correspond to the post-write file version.
- Missing server degrades clearly to normal verification.
- Untrusted diagnostic text remains data.
