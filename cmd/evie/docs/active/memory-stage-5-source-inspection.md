# Memory Stage 5: original answer sources (#163)

Implements [issue #163](https://github.com/davidadel66/evie/issues/163) under the
[Stage 5 retrieval specification](memory-stage-5-retrieval.spec.md), its
[decisions](memory-stage-5-retrieval.decisions.md), and
[ADR 0056](../../../../docs/adr/0056-preserve-semantic-history-through-lifecycle-changes.md).

## Original request association

The existing fenced Context Snapshot append remains the receipt boundary. The
snapshot, content-free memory references, exact serialized request size, and
request SHA-256 are committed before the provider is called. Failure to commit
the snapshot prevents dispatch. No second conversation episode or source-text
diagnostic is introduced.

`POST /api/memory/evidence` retains `sessionId` plus `snapshotId` selection and
also accepts an `answerId`. Selecting an answer returns its ordered original
provider requests, each with its snapshot ID, response ID when committed,
iteration, exact request hash and size, rendering version, retrieval outcome,
and independently inspected original references. The existing top-level
snapshot/evidence fields remain available. An answer selects the request that
produced its final committed assistant event. When both selectors are present,
the snapshot must belong to that answer's causal root and precede its answer.

Association uses durable parent IDs and event order. A response pairs with the
nearest preceding snapshot having the same causal parent. An unrelated later
turn cannot acquire an earlier answer's receipts. Source inspection reads the
committed history while a request is in progress; it does not require the mutable
agent turn to become idle. The selected-session check still precedes every read.

The request status is separate from retrieval status:

- `prepared`: the snapshot exists and no matching committed response or terminal
  interruption exists. This does not establish provider delivery.
- `completed`: the matching assistant response committed, including an
  intermediate response requesting tools.
- `interrupted`: a durable failed/interrupted terminal event belongs to the same
  causal root and follows an uncompleted snapshot. Source use and delivery remain
  unconfirmed. A merely unpaired request is never assumed interrupted.

The compact source panel lists the original requests separately, displays the
selected request's exact evidence, and distinguishes completed, prepared, and
interrupted requests. Live activity says “recorded” until an assistant response
commits. Supplied evidence is not presented as an answer citation or proof that
the model relied on a source; no hidden reasoning is inferred.

## Later memory changes and current source access

Each inspection resolves its saved Claim operation/read pins or Conversation
Excerpt event, UTF-8 locator, and hash. It never reruns relevance search. Original
references remain unchanged while current correction and lifecycle state are
shown separately. Claim IDs and original acceptance-operation IDs are available
in the panel; conversation excerpts never acquire fabricated Claim identities.

Current source eligibility and scope are reapplied for every request. Retraction
returns an unavailable entry with its original content-free reference and omits
source/Claim text. Unrelated available sources remain inspectable. All evidence
responses use `Cache-Control: no-store`.

Source retraction previously required the owning Claim to be active, which
prevented withdrawing an old answer's original source after correction or
retirement. The retraction path now accepts an eligible Source Link even when its
Claim is retired or superseded. The exact eligible-source, scope, approval,
revision, and append-only guards remain. Restoration still requires an active
Claim and cannot revive a superseded or retired Claim through its source. This
matches the Source Link transition contract in
[semantic encodings](semantic-memory-encodings.spec.md); it does not relax Claim
restoration rules.

## Deterministic demonstration and verification

`TestMemoryReceiptHTTPKeepsOriginalVersionsThroughCorrectionRetirementRestrictionAndRestart`
creates accepted Claims and uncompiled conversation evidence through public
agent turns. It records an answer, corrects one Claim, retires another, inspects
both original versions before/after SQLite reopen, retracts both original
sources, verifies restoration remains denied, and reopens again. The exact
references survive and restricted source text does not.

`TestMemoryReceiptIsDurableBeforeEachProviderRequestIncludingInterruption`
checks real SQLite from inside the scripted provider callback, matching each
already committed snapshot's bytes/hash and references to the complete actual
request. It covers successful and cancelled turns and exact reopen history.
`TestMemoryReceiptInspectionRejectsChangedOriginalProvenance` rejects altered
hash, locator, event, authority, and observed-time fields for both evidence kinds.

The HTTP tests also cover an answer's multiple requests after a later unrelated
turn, current selected-session authorization, and a request held inside the
provider while inspecting prepared and subsequently interrupted states. Focused
UI tests cover request labels, original Claim versions, attribution, historical
state, current unavailability, and live activity transitions.

For a manual demonstration, open a completed answer's memory activity, switch
between its original requests, and expand a source reference. Correct or retire
the accepted Claim, reopen the same sources, then retract its Source Link and
open them again. Original state remains distinct from current state, and the
retracted source becomes unavailable. Reopening the application preserves the
same attribution and restrictions.

The staged #163 tree was exported separately from later work for verification:

- `go test ./internal/agent -run '^TestMemoryReceipt' -count=1`: PASS, 0.509s.
- `go test ./internal/web -count=1`: PASS, 5.791s.
- `go test ./internal/eviedb -run 'TestSemantic.*Lifecycle|TestSemantic.*Correction|TestSemantic.*Transition' -count=1`: PASS, 0.265s.
- `go vet ./internal/agent ./internal/eviedb ./internal/web`: PASS.
- UI `npx vitest run src/chat/MemoryActivity.test.tsx src/artifacts/MemoryEvidence.test.tsx src/api/memoryEvidence.test.ts`: PASS, 3 files / 17 tests.
- `npm --prefix internal/web/ui run build`: PASS; existing large-chunk warning.
- `git diff --cached --check`: PASS.

The first exported-tree HTTP attempt could not compile because the generated
embedded UI distribution was absent. Building the UI resolved that setup error;
the full HTTP package then passed. A final assertion checks the exact inactive
Claim rejection on source restoration, followed by another focused HTTP run.
The repository-wide `./scripts/verify-change.sh` remains required before the PR
handoff; these focused checks do not replace it.
