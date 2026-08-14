# F28 - Explicit source formatting

Status: deferred candidate, unapproved. Recommendation: do not auto-format
after an approved edit.

## Purpose

Keep generated code consistent with project formatting rules without changing
bytes the user never previewed or turning formatter output into an invisible
side effect.

## How OpenCode does it

OpenCode resolves configured/built-in formatters by file extension and runs
their commands after `edit`, `write`, or `apply_patch` has written the file. The
edit tool then re-reads the formatted bytes and reports the resulting diff;
write also runs diagnostics afterward.

This ordering means the permission request previews the pre-format diff, while
the formatter may produce additional final changes after approval. Formatter
spawn/nonzero failures are logged and formatting is not an OS sandbox.

Sources:

- [`format/index.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/format/index.ts)
- [`tool/edit.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/tool/edit.ts#L145-L171)
- [`tool/write.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/tool/write.ts#L53-L90)

## EVIE today

`edit_file` previews complete before/after contents, then rechecks the exact
bytes and permissions immediately before atomic rename. The approved `NewText`
is the final file. An automatic formatter after rename would violate that
contract even if its changes were desirable.

## Proposed EVIE adaptation

Start with formatting as verification, not mutation:

- the project verification contract runs its check-only formatting command;
- a failure is recorded like any other required gate;
- the build agent proposes an explicit prepared edit to correct it;
- no file tool invokes a formatter after an approved write.

If generic automatic formatting later earns its complexity, run the formatter
inside an isolated candidate worktree (F16), collect the complete resulting
diff, and bring that diff back through the normal prepared approval path. The
preview must include every formatter-touched file and the candidate must be
revalidated before application. A second gated format diff is also acceptable;
an unpreviewed post-write formatter is not.

## Not in the first slice

- Automatic formatter discovery or installation.
- Running repository-provided formatter commands merely because a project was
  opened.
- Formatting after write without another prepared preview.
- Treating formatting success as build/test success.

## Acceptance evidence

- Final file bytes exactly match the approved preview.
- A formatter changing multiple files presents all changes before application.
- Formatter commands are trusted project configuration and run through F06/F05
  with timeout, cancellation, output, and environment policy.
- Formatter failure leaves the parent worktree unchanged.
- Check-only formatting integrates into F14 without granting mutation authority.

## Open decisions

1. Which project configuration declares a trusted formatter check/fix command?
2. Is an isolated-worktree formatter worth building before ordinary explicit
   edits prove too cumbersome?
3. Does one approval cover every formatter-touched file or one file at a time?
