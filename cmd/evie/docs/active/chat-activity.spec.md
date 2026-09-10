# Chat activity timeline

Status: implemented and verified. Origin: David's request to replace raw tool cards with
the compact activity presentation in his Codex screenshot.

## Outcome and acceptance

- Each accepted user turn has one collapsible activity section. It opens while
  working and collapses on completion unless the user chooses otherwise.
- Public progress and actual reasoning summaries appear inside it; empty
  reasoning events produce no separate thought timer. The committed final
  answer appears below it. Streamed text is provisional until commitment.
- Tools use compact, readable action labels. Full arguments, results and
  recorded approval decisions remain accessible by keyboard in disclosures.
- Pending approvals, tool failures and discarded-response warnings remain
  visible when activity is collapsed. Requested tools are not called running
  or successful before an outcome is recorded.
- Turn identity, final/progress classification and successful elapsed time
  survive reload and pagination using existing durable events. Missing terminal
  evidence is shown as incomplete. Historical durations are never invented.
- Queued messages, session selection, authorization and approval previews keep
  their current behavior. Model instructions request brief progress for
  substantial work, without narrating every tool call.

## Design

Keep the existing palette: background #0e1113, text #e2e6e3, secondary text
#8b9491, teal #4fb8a5, attention #e8c98a, failures #d9a0a0. IBM Plex Sans
carries prose and activity labels; IBM Plex Mono is reserved for exact details.

```text
                                      User message
  Worked for 24s  v
    I'll inspect the plugin registration and its callers.
    [file] Read file · registry.go
    [terminal] Ran command · go test ./internal/tools
  Final answer, in the existing markdown presentation
```

Use whitespace and alignment, with no surrounding tool cards. Disclosures
have visible keyboard focus and bounded, scrollable details. Long subjects
truncate in the row and remain complete in details. Pending approval previews
retain their existing prominent presentation. Avoid decorative animation.

## Scope, risks and verification

No provider migration, database migration, new dependencies or native subagent
runner. Subagent lifecycle rows await that separate feature. Reasoning summaries
remain provider-dependent and are not persisted by this change. Wall time
includes tools, approvals and provider waits; it is not model compute time.

Verify deterministic SSE/replay metadata, phased assistant reconciliation,
multiple rounds, incomplete/error cases, pagination grouping, readable labels,
and disclosure/approval visibility. Run the UI suite, focused Go tests and
`./scripts/verify-change.sh`; inspect synthetic desktop/mobile conversations
without sending model requests. Rebuild and restart only an idle local server.

## Verification record — September 10, 2026

- `go test ./internal/web ./internal/agent`: passed.
- `go test -race ./internal/agent ./internal/web`: passed.
- `./internal/web/ui/node_modules/.bin/vitest run --root internal/web/ui`:
  204 tests passed in 36 files.
- `./scripts/verify-change.sh`: passed, including Go tests/vet, UI lint/build
  and whitespace checks. Existing warnings: five Fast Refresh export warnings
  in memory presentation/icons and Vite chunks larger than 500 kB.
- Synthetic browser verification: desktop live/completed layout, automatic
  collapse, 390px viewport without horizontal overflow, approvals and failed
  actions visible while collapsed, keyboard disclosure, and an expanded first
  tool remaining open when another tool joins its group. No model requests.
- Parallel standards/spec reviews found no remaining blocking issue. Fixed
  review findings covering empty completed activity, exact argument rendering,
  disclosure stability and bounded reasoning summaries. No required checks skipped.

To demonstrate: refresh Evie, request a multi-step code inspection, then expand
the Worked row and the grouped actions. Approvals remain actionable even if
the outer activity is collapsed. Review starts in `internal/web/activity.go`
(shared public projection) and `internal/web/ui/src/chat/Activity.tsx`
(presentation), with reducer reconciliation in `store/reducer.ts`.
