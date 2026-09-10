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

## File inspector amendment — September 10, 2026

David requested clicking files in activity to open beside chat, following his
Codex file-pane screenshot. File names for `read_file`, `edit_file`, and tools
with full file approval previews become separate keyboard-accessible buttons.
Opening one selects the existing right inspector and preserves the chat and
composer. The selected action remains pinned as later actions arrive; its
approval/result state updates live. Changing session or workbench view clears
that selection. Close/Escape returns focus to the originating file button, or
its activity header if the action collapsed or disappeared during the turn.

Use the existing palette and typography above: left-aligned code in IBM Plex
Mono, line-number gutter, full path in a compact header, teal selection,
green/red changed lines, no nested cards. Desktop keeps chat and the inspector
side by side; narrow screens use the existing full-width inspector overlay.

```text
  Chat / activity                    | registry.go                 x
    Read file   [registry.go]        | /src/internal/tools/registry.go
    Edited file [registry.go]        | File     Changes
                                    |  1  package tools
  Composer                          |  2  ...
```

The pane is read-only and uses recorded evidence. Successful reads decode the
existing sequential line-number format exactly, including tabs and final
newlines. Bounded/malformed results remain clearly labeled excerpts. Live edit
previews offer complete before/after content and a diff; historical edits
without previews show only the recorded replacement, explicitly labeled as
such. Proposed, declined, expired and failed changes never claim saved content.
Source text renders as inert code, with syntax highlighting using the existing
package and theme. The content pane scrolls independently and supports a
side-by-side Before/After diff, with each column scrolling independently.
File contents, replacement text, diffs and tool details appear only in the
inspector; chat retains the compact file row and pending approval controls.
File inspectors use an overlay only below 640px so chat stays beside them in
smaller desktop windows.

No new filesystem API, tool execution, persistence schema, native editor,
arbitrary-path browsing, or shell-command path inference is introduced. No
dependencies are added. Tests cover decoding, state transitions, replay
fallbacks, selection binding and escaped content; browser verification covers
clicks, keyboard controls, sequential selection, closing and narrow screens.

### File inspector verification

- `./internal/web/ui/node_modules/.bin/vitest run --root internal/web/ui`:
  225 tests passed in 39 files, including exact source decoding, recorded
  approval outcomes, absent inline file diffs, and stale highlight-cache output.
- `./scripts/verify-change.sh`: passed (Go tests/vet, UI lint/build, staged
  and unstaged whitespace). Existing warnings remain: five Fast Refresh export
  warnings and Vite chunks larger than 500 kB.
- Synthetic browser checks passed: compact chat rows beside the inspector,
  edit comparisons defaulting to side-by-side Before/After, switching to a read
  snapshot, syntax highlighting, Escape returning focus, session navigation,
  and a narrow-screen overlay with contained keyboard focus and no page overflow.
  No model requests were sent. Parallel review findings were resolved.
- No required checks skipped. Race checks were not repeated for this frontend
  change; the earlier activity implementation's race verification remains above.

To demonstrate: refresh Evie, expand a turn's activity, and click a read or edit
file row. Edits open Changes in the right pane; File/Replacement and Details
provide the recorded content and exact tool data. Chat keeps only the compact
row and any pending approval controls. Review starts in
`internal/web/ui/src/artifacts/fileInspection.ts`, `artifacts/FileViewer.tsx`,
and `chat/Activity.tsx`.
