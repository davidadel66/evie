# Chat activity timeline

The shared tabbed inspector extension is specified in
[workspace-local-folder.spec.md](workspace-local-folder.spec.md). Live disk files
and Git comparisons use separate tabs; the recorded-evidence rules below remain
binding for file/tool/memory inspections opened from conversation activity.

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
  Recorded failures and interruptions retain their incomplete label and warning;
  reload advice appears only while terminal evidence is missing.
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

### File presentation refinement — September 10, 2026

David's follow-up screenshot supersedes the metadata-heavy file header and
Details tab. A read file now shows one breadcrumb path header and source code,
without Read, Recorded file contents, duplicate filename headers, or tool-data
tabs. Edit previews keep File/Changes controls and side-by-side comparison.
Incomplete snapshots and unsuccessful proposals retain only a short qualifier
so they cannot be mistaken for a complete saved file. Approval decisions remain
available in the path tooltip, without a separate metadata block.

The file pane uses a neutral #181818 editor background, #d4d4d4 source text,
#8b8b8b comments/gutters, #c586ff names/types, #f47076 keywords, #85d77a
strings, and #ee994d variables. IBM Plex Mono remains the source face, with
13px code and 1.8 line height. Paths use IBM Plex Sans, muted directory segments
and a brighter filename. Source aligns left with a fixed line-number gutter.
The rest of Evie's palette remains unchanged. This follows the supplied editor
reference directly; no extra toolbar, cards, or explanatory labels are added.

Verification covers the absent read metadata/tabs, inert and exact highlighted
source, edit comparison and partial/error qualifiers, plus browser screenshots.

### Tool inspection amendment — September 11, 2026

David requested less tool and debug noise in chat, with readable information in
the inspector and raw logs behind a button. This supersedes the earlier inline
placement of generic tool arguments and results. Expanded activity retains
compact action rows; selecting a generic tool opens its details beside chat.
The inspector leads with the readable action, subject, outcome, and recorded
approval state. Exact arguments and results remain inert, unchanged strings
under a keyboard-accessible **Debug details** disclosure, closed by default.
Opening the inspector does not itself expose raw logs.

Selection is bound to the session ID and tool item key. The selected action
stays pinned as later actions arrive and reflects live approval and result
updates. Session or workbench navigation clears selection. Close/Escape returns
focus to the originating action, its activity header, or the inspector toggle
when the original control is no longer visible. Narrow-screen inspection keeps
keyboard focus within the overlay.

Pending approvals retain their readable effect previews and actionable controls.
Failures, declined or expired approvals, discarded-response warnings, and
incomplete turns remain visible when activity is collapsed. A requested or
approved action without a recorded result never claims successful execution.
The existing file viewer keeps its breadcrumb and source presentation,
File/Changes controls, recorded-content boundaries, and concise qualifiers.

Both collapsed and expanded activity show the latest routine memory receipt.
Earlier receipts with warnings, interrupted requests, historical or retired
evidence, or conflicts remain visible. The source inspector retains the full
original request sequence under Debug details. Accepted
memory and Conversation excerpt labels, recorded-versus-supplied status, source
availability, and original-versus-current evidence state remain explicit.

Verification covers absent inline raw payloads, closed initial debug details,
exact inert argument/result rendering, session-bound selection and live updates,
approval and failure visibility, file inspection, and memory receipt grouping.
Synthetic desktop/mobile checks cover opening actions, closing and focus
restoration, and bounded scrolling without remote model requests. No backend,
retrieval, persistence, or dependency changes are required.

Verification on September 11, 2026:

- `./internal/web/ui/node_modules/.bin/vitest run --root internal/web/ui` passed:
  48 files, 263 tests. Regression cases cover exact logs, accessible action
  names, independent memory warning states, and session-bound live selection.
- `./scripts/verify-change.sh` passed: UI lint/build, full Go tests/vet, staged
  and unstaged whitespace checks. Existing warnings remain: five Fast Refresh
  export warnings and Vite chunks larger than 500 kB. No required check skipped.
- A disposable real SQLite fixture with a scripted provider was inspected in
  the browser at desktop and 390 px width. Confirmed compact completed activity,
  closed initial debug details, keyboard expansion, reset on another action,
  Escape focus restoration after collapsing activity, mobile Tab/Shift+Tab
  containment, original request/source navigation, session isolation, and no
  browser warnings or errors. The running personal server was not changed.

To demonstrate: expand a completed turn's Worked row, select a tool's Inspect
action, then open Debug details. Close the inspector or press Escape to return
to chat. The memory badge opens the original sources and request sequence.


### Useful memory results — September 11, 2026

David reported that hiding raw payloads left the inspector showing an empty
source receipt even when a memory listing returned saved preferences. The
inspector must show useful recorded results before diagnostic details:

- Memory record listings, exact Claim queries, traversals, and inspections show
  readable values, subject/predicate, scope, and recorded lifecycle state.
  Preserve denied polarity and entity-valued Claims; mark additional pages.
  Use the final recorded lifecycle entry for exact Claim queries. When an
  inspection records only an entity reference, explicitly qualify its missing
  name and keep the exact identifier in Debug details.
- Search actions show their returned count and partial, failed, unavailable,
  cancelled, exhausted, or truncated outcome. Zero search matches do not mean
  that no saved records exist. Plain-text tool results and errors get bounded
  readable previews; unsupported structured output retains an explicit summary
  limitation and its exact debug payload.
- Opening a memory badge also shows **Memory results for this turn**, derived
  from the selected snapshot's own session and turn. These are recorded tool
  results, distinct from request-specific retrieval evidence. Later tool results
  must not be attributed to an earlier provider request or called answer citations.
- Retrieval evidence still comes from the authorized source-inspection API.
  Current source access, historical/retired/conflict state, and original versus
  current evidence remain explicit. Recorded tool results never replace an
  unavailable source in that API's evidence list.
- Request selection, IDs, hashes, and exact tool logs sit under **Debug details**.
  Both open and collapsed chat keep one routine receipt plus important warnings;
  individual memory actions show returned counts and remain directly inspectable.
  A receipt containing no accepted memory is labeled Memory retrieval.

The disposable browser fixture includes **09 Listed preferences after empty
search**: a real SQLite turn with a scripted provider, one empty search, two
listed preferences, and three empty retrieval receipts. Its assertions run
against reopened history before the browser server starts. Demonstrate by
opening that turn's memory badge: both preference values must appear without
opening Debug details; the raw listing and request sequence remain inside it.

Correction verification on September 11, 2026:

- `./internal/web/ui/node_modules/.bin/vitest run --root internal/web/ui` passed:
  48 files, 286 tests. New regression cases cover readable record values,
  recorded lifecycle and validity, missing entity names, negative Claims,
  failures, paging, and session/turn association.
- `./scripts/verify-change.sh` passed: full Go tests/vet, UI lint/build, and
  staged/unstaged whitespace checks. Existing warnings remain: five Fast Refresh
  export warnings and Vite chunks larger than 500 kB. No required check skipped.
- The opt-in `TestStage5BrowserFixture` passed with real SQLite and a scripted
  provider. Desktop and 390 px browser checks confirmed both listed preferences
  appear before empty-search information, request navigation preserves distinct
  turn results, exact logs stay behind closed Debug details, and individual
  actions show counts. Mobile keyboard containment, Escape focus restoration,
  bounded log scrolling, and no page overflow were verified. No browser warnings
  or errors were recorded; the personal server was not changed.
- Standards and Spec reviews have no remaining actionable findings. Query
  lifecycle and entity-reference display gaps found during review were fixed
  with failing-then-passing regression tests before handoff.

## Live status line amendment — October 3, 2026

David asked for one updating line while Evie works instead of the full trace,
following T3 Code's work log. This supersedes "opens while working" above and
the per-row visibility of settled tool failures while collapsed.

- Activity starts collapsed while working and stays collapsed on completion
  unless the user expands it. The full trace, including commentary, reasoning
  summaries and every action row, remains one click away and is unchanged.
- While working and collapsed, one line below the header shows: assistant
  text as it streams; otherwise the latest requested tool without a result,
  as its existing compact row (label, subject, Inspect); otherwise
  "Thinking…". It is derived from events only, never from reasoning text, so
  it reads the same whether or not the model exposes reasoning. Committed
  commentary folds into the trace.
- Pending approvals keep their actionable rows and replace the live line.
  Latest and flagged memory receipts, discarded-response warnings and notices
  stay visible as before.
- Settled tool problems (failed, declined or expired actions and flagged memory
  searches) collapse into one amber line such as "2 actions failed" or
  "1 action needs attention". Selecting it expands the trace, where each row
  keeps its label and Inspect action.

- Inside the expanded trace, settled progress text renders as muted 13px
  markdown so it reads as a log. Text still streaming keeps answer size until
  its commit settles whether it is progress or the final answer.

No backend, persistence, event or dependency changes. Verification covers the
collapsed live line for a requested tool, Thinking between tools, streamed
text replacing the line, approvals replacing it, the problem summary, and
quiet versus streaming progress text.

## Empty memory receipt amendment — October 12, 2026

David asked to see memory activity only when retrieval supplied something.
This supersedes "Both collapsed and expanded activity show the latest routine
memory receipt" above: both now show the latest receipt that supplied accepted
memory or conversation excerpts, and a receipt that supplied nothing is not
shown. Receipts with warnings (failed, partial, unavailable, cancelled,
exhausted or interrupted requests, and historical, retired or conflicting
evidence) stay visible as before. Receipts are still recorded; the source
inspector, memory tool rows and their returned counts are unchanged.
Trade-off: an empty retrieval's request details are no longer reachable from
chat.

Verification covers empty receipts hidden while working and after completion,
the latest supplying receipt kept over a later empty one, and earlier warnings
kept visible.

## Live reasoning line amendment — October 12, 2026

David asked for a one-line summary of what the model is doing, updating as it
works, like ChatGPT's thinking line. This supersedes "It is derived from events
only, never from reasoning text" in the October 3 live status line amendment.

- While working and collapsed, when streamed reasoning is the latest activity,
  the live line shows its latest bold heading if the model writes headings,
  otherwise its latest complete sentence, as one muted, truncated line with the
  full sentence in its tooltip. A sentence still being written is never shown.
- Each shown sentence stays at least two seconds before the line moves to the
  newest, so fast reasoning does not flicker.
- Streamed assistant text, then a newer requested tool, still take over as
  before; with neither, and no reasoning sentence, the line reads "Thinking…".
  A model that does not expose reasoning therefore behaves exactly as before.
- Pending approvals still replace the line. The expanded trace is unchanged.

No backend, persistence, event or dependency changes. Deferred, by David's
choice: a very cheap model writing a short headline instead of the sentence
(BACKLOG Ideas). Verification covers withholding an unfinished sentence, the
latest complete sentence and heading preference, abbreviations not splitting a
sentence, the tool row taking over, and Thinking after a settled tool.

## Subagent inspector amendment — October 12, 2026

David saw `delegate_research` sit at "requested" / "No result recorded" while
its four research children ran, with no way to see them. He asked to see
subagents working, which finished, and what each was asked: in the chat UI
and its inspector, not in Runs. Decision record: the 2026-10-12 entry in
[subagents.decisions.md](subagents.decisions.md).

- Delegate rows (`delegate_research`, `delegate_explore`, `delegate_build`,
  `continue_research`) read as "Research with subagents", "Explore with
  subagents", "Build with a subagent" and "Continue a subagent". While the
  turn is live and the row has no result, its label reports its children,
  such as "Researching · 2 of 4 done", read every two seconds. This applies in
  the expanded trace and in the collapsed live line.
- Inspecting a delegate row opens **Subagents** in the inspector (a build row
  that names its Changes still opens them). It lists each child: state
  (Queued, Working, Done, Partial report, Failed, Cancelled, Interrupted),
  its assignment clamped to two lines, elapsed time, its latest requested
  action as its chat row would read, and its action count. The list refreshes
  every two seconds while the call is unresolved or any child is unfinished,
  and keeps retrying after a failed read until then. Debug details keep the
  parent call's exact arguments and result. A call that started no children,
  including one refused for reusing an idempotency key, shows the generic tool
  inspector.
- Selecting a child shows its full assignment, any context Evie gave it
  (collapsed), and its own trace for that attempt only (commentary, grouped
  actions and its report) using the chat's activity rows, refreshed every three
  seconds while it works; selecting one of its actions shows that action's
  inspector, with Back at each level. A continued child's earlier attempt
  never shows the continuation's work. Traces cut tool results to 4 KiB and
  child text to 64 KiB, and shorten long argument values while keeping the
  arguments JSON so each row keeps its subject; this is for display only.
- Reads go through `/api/subagents/list` (one call's children, identified by
  turn and provider call ID, since call IDs can repeat across turns) and
  `/api/subagents/trace` (one child), answered only for the open conversation.
  Child sessions stay out of session lists and cannot be opened as chats.

No persistence, migration, event, preset or dependency changes. Not included:
stopping or continuing a child from the UI, or showing children in Runs.

### Verification — October 12, 2026

- `go test ./internal/eviedb -run 'TestSubagentsOfCall|TestSubagentProgress|TestBoundToolArguments'`
  and `go test ./internal/web -run 'TestSubagent|TestHistory'` passed, also
  under `-race`, with real SQLite: queued → running → latest action; another
  top-level session with the same call ID and key reads nothing (a mutation
  dropping the parent filter fails it); a later web search or delegation
  reusing a call ID, a replayed key and a refused key; a continuation kept out
  of the original attempt; the open-conversation gate; a live child's action
  staying requested; bounded arguments keeping their subject.
- UI suite: 597 tests in 91 files passed, including the live reasoning line
  (abbreviations, initials, numbers and years), empty memory receipts, no
  per-turn instructions control, delegate labels, the subagent list and child
  views with context, the request shape and the subagents pane tab. The
  polling hooks and the two-second hold have no automated test (the UI suite
  has no DOM environment).
- A fresh-context review found no data-boundary issue; its two correctness
  findings (continuations leaking into the original attempt, and call IDs
  reused across turns) and its smaller ones were fixed with the tests above.
- Browser check without model requests: a disposable copy of the live
  database served offline (no credentials, all outbound HTTP refused) and
  driven with headless Chromium at 1440 px and 390 px. The October 12
  superintelligence turn showed "Research with subagents", its four real
  children (Done, Partial report ×2, Done; 38–81 actions; 49s–2m 45s), one
  child's trace, and one of its web searches, with no console errors and no
  horizontal overflow. The live states (polling a running child, the waiting
  row label, the reasoning line) were not exercised in a browser: they need a
  live turn, and the copy's recovery correctly settled a simulated running
  child at once.
