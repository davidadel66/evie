# Chat context failure — September 29, 2026

The chat stopped because automatic compaction treated an unreachable preferred
target as a hard context overflow. Large fetched pages made the current turn too
large to reach that target, although the complete bounded request still fit the
usable input budget. The model-selection changes were not running in the server
that produced this failure.

## Changes and contract

- `internal/agent/automatic_compaction.go:109`: if no legal compaction prefix
  reaches the target, continue only when the existing complete projected request
  fits. Preserve the retained history, active turn, tool-group bounds, reserve,
  estimation margin, and hard rejection of requests over budget.
- `cmd/evie/docs/active/memory.decisions.md`: explicitly correct the prior rule
  that rejected all requests without a legal cut, including fitting requests.
- `internal/web/ui/src/chat/Activity.tsx:46`: a recorded failure or interruption
  keeps its visible warning and incomplete label, without advising another reload
  for missing terminal evidence. Unknown terminal state still advises a reload.
  This matches the amended `chat-activity.spec.md` acceptance criterion.

## Reproduction and focused validation

The reported request was inspected through read-only SQLite access. Its final
completed snapshot measured 112,919 serialized bytes. The next two successful
fetch results contained 102,400 and 77,859 bytes; the PDF and delegation errors
occurred earlier and did not terminate the turn.

A temporary replay preserved the actual saved event content and calibrated
fixed request overhead to the preceding snapshot size. It measured 241,596 bytes
after existing tool-result bounds, within the 241,664-byte usable budget, but
above the 209,715-byte pressure threshold and 157,286-byte preferred target.
Before the fix it reproduced the exact `no legal automatic compaction` failure;
afterward selection and final composition succeeded without moving the retained
frontier. This is a calibrated replay, not a captured provider request or a live
provider completion. The private export and temporary replay test were deleted.

- `go test ./internal/agent -run 'TestReportedContextReplay|TestSendContinuesAfterLargeToolGroupWhenNoCompactionTargetFits' -count=1 -v`
  passed after the fix. The permanent synthetic post-tool regression also failed
  with the fallback removed and passed when restored.
- `go test ./internal/agent ./internal/web -run 'TestAutomaticCompaction|TestSend.*Automatic|TestSendContinuesAfterLargeToolGroupWhenNoCompactionTargetFits|TestReportedContextReplay|TestActivity' -count=1`
  passed. Permanent coverage includes one byte below/at/above the usable limit,
  real `Send` progression after a large atomic tool group, snapshot/request
  equality, retained history, and durable overflow rejection.
- `./internal/web/ui/node_modules/.bin/vitest run --root internal/web/ui src/chat/activity.test.tsx`
  passed all 16 tests after the recorded-failure regression failed before the UI
  fix. Go replay tests verify failure/interruption timestamps and warnings.
- `go test -race ./internal/agent ./internal/web -run 'TestAutomaticCompaction|TestSend.*Automatic|TestSendContinuesAfterLargeToolGroupWhenNoCompactionTargetFits|TestActivity' -count=1`
  passed both packages.
- `./internal/web/ui/node_modules/.bin/vitest run --root internal/web/ui`
  passed 372 tests across 62 files.
- `./scripts/verify-change.sh` passed: full Go tests and vet, UI lint and build,
  staged and unstaged whitespace checks. Existing warnings remain: five Fast
  Refresh export warnings in `src/memory/presentation.tsx` and `src/ui/Icon.tsx`,
  plus Vite's chunks-larger-than-500-kB warning. No required checks were skipped.
- Independent focused reviews found no actionable issues in context safety,
  retained history, cancellation, recorded failure presentation, or specifications.

## Separate existing limitations

- Workspace delegation is intentionally refused until reviewed research Agent
  Preset allowances and their admission/revocation enforcement are supported.
  `go test ./internal/subagents -run '^TestWorkspaceAdmissionExplainsMissingReviewedPresetAllowance$' -count=1 -v`
  passed and confirms refusal before model calls. The authority guard remains.
- `web_fetch` explicitly supports text, HTML, JSON, and XML, not PDF extraction.
  `go test ./internal/tools -run '^(TestExtractText|TestCapText|TestWebFetch)$' -count=1`
  passed, including the existing PDF refusal. Reading PDFs requires a separate
  extraction feature; the paper's HTML landing page contains only metadata and
  the abstract, not its full text.

## Demonstration and deployment boundary

After rebuilding and restarting an idle local Evie server, reload the failed
conversation: the saved incomplete-turn warning remains, while redundant reload
advice disappears. Send a new follow-up to continue from saved history. Large
tool-result turns now proceed while their full bounded request fits; additional
results can still legitimately exceed the unchanged hard budget.

The running server and installed binary were not replaced. No live model request
was sent, so completion of the original research task is not established.
The footer was tested using the actual React component's rendered output; a
separate browser layout run was not repeated for this conditional-text change.
No dependencies, Workspace allowances, provider limits, or stored conversations
were changed. Existing model-selection and unrelated spending work is preserved.
