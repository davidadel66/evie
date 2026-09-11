# Memory Stage 5: bounded relationship discovery (#164)

The existing `memory_search` tool can find an accepted fact through one or two
accepted relationships. For example, an approved Maya → sister → Nora Claim and
Nora → prefers → baklava Claim let a Maya search supply both exact owner sources.
This implements [#164](https://github.com/davidadel66/evie/issues/164), the Stage 5
retrieval specification's query/ranking and provenance contracts, and ADR 0055's
separation of world Claims from structural Graph Links.

## Query, eligibility and ranking

One read transaction resolves the durable requester's scope and temporal pins
before candidate discovery. Its internal query plan fixes Global, the current
Workspace/project, and the current session; query text; temporal intent;
explicit Valid Time constraints; source authorities; graph bounds; and result
budgets. Exact identity/accepted Alias, lexical FTS, explicit-time, and graph
generators share the caller's cancellation and 500 ms foreground deadline.
These generators have separate bounded candidate lists; they run against the
same SQLite snapshot. Search performs no backfill and has no persistent
in-process graph cache.

Every candidate must pass accepted-state, original/current lifecycle, temporal
applicability, scope availability, and exact source eligibility before it gets
a rank. Reciprocal Rank Fusion uses `1 / (60 + eligible_rank)`. Exact and lexical
matches provide separate votes. An explicit `valid_at` enables a temporal
generator over lexically relevant Claims with recorded validity bounds;
unknown bounds receive no temporal vote. Its ordering uses applicable validity
start specificity, never source observation time as a fabricated fact date.

Direct matches precede proximity-only results. Within each class, fused rank
and supporting-source count break relevance ties before stable Claim IDs.
Graph-only selection gives each scoped subject/predicate group a place before
another result in that same group. This prevents many similar preferences from
crowding out a different relevant relationship. Accepted conflicting Claims
remain independent, source-bearing records with the existing Kernel conflict
metadata. The graph does not choose truth or accept an inferred relationship.

Graph anchors come from eligible entity identifiers, accepted aliases,
canonical-name matches, or an Entity-object relationship predicate matching the
query. A generic literal hit does not fan out through the owner's entire memory.
World relationships are accepted Claims, not structural Graph Links. Each hop
checks the original scope list, active applicable entity state, Claim lifecycle,
time, and owner-statement/tool-observation source policy. Traversal can read
either endpoint of a relationship without reversing or rewriting the accepted
proposition. A denied Claim can appear as marked evidence but cannot establish
an intermediate relationship. Repeated nodes and repeated Claims cannot form a
path. A Global anchor never grants entry to another Workspace, project, or
sibling session; raw conversation access remains governed by its own scope
matrix.

## Path support and bounded coverage

`graph_paths` on evidence and content-free durable references contain
`anchor_entity_id` and ordered `claim_ids`. The last Claim is the retrieved fact;
every earlier Claim is its accepted relationship support. Each support Claim
also appears with its exact sources in the selected evidence set. The existing
`paths` reasons distinguish direct/lexical/temporal discovery from
`graph_one_hop` and `graph_two_hop`. A directly discovered Claim does not acquire
a redundant graph reason merely because traversal visits it.

Dispatch revalidation repeats current source and lifecycle checks, then removes
paths whose supporting Claims did not survive. A proximity-only fact without
complete support is removed from model input. Explicit source inspection keeps
an independently inspectable terminal source while pruning an unavailable
bridge from the path display. Original request references stay immutable.

All selected path members must share the same intent, `valid_at`, `as_known_at`,
and explicit-validity flag. If a later targeted search replaces a supporting
Claim with another temporal read, the earlier path and its proximity-only
terminal are withheld. A new targeted search must refresh the complete
supported graph view before that path can be supplied again; a matching Claim
ID alone does not substitute another temporal view.

The engineering defaults are:

| Boundary | Limit |
| --- | ---: |
| Graph depth | 2 Claims |
| Eligible entity anchors | 4 |
| Raw incident Claims per frontier | 16 |
| Distinct Claim candidates across generators and supplements | 64 |
| Raw exact / lexical / explicit-time candidates | 16 / 24 / 8 |
| Distinct displayed support paths per Claim | 2 |
| Selected evidence records | 8 |
| Storage result serialization | 24 KiB |
| Existing complete-turn memory budget | 36 KiB |
| Shared foreground deadline | 500 ms |

Raw bounds may omit eligible records beyond a candidate window. Candidate,
result, and byte limits report truncation; incomplete index coverage keeps its
existing explicit status. A search does not pad to eight records when support
or context bytes cannot fit. These are bounded local defaults, not a claim that
all reachable relationships were enumerated.

## Local experiment

The [frozen configuration](../fixtures/memory-stage5-graph/v2/freeze.json) records
input file hashes and the separately compiled test executable SHA-256. Input
hashes matched before and after compilation. The executable hash was checked
again after measurement; later working-tree edits could not change the measured
program. The [report](../fixtures/memory-stage5-graph/v2/report.json) includes
every accepted fixture, all 120 timing samples, and serialized request sizes.
The [verification record](../fixtures/memory-stage5-graph/v2/verification.json)
contains the exact command and artifact hashes. An initial harness-only setup
failure, before measurements, is retained in `v1/setup-attempt-1.json`. The earlier successful v1 measurement is also retained; v2 includes the final same-temporal-view support check.

Each profile used real SQLite, owner-approved Entity Claims, and a complete
scripted-provider turn, with Automatic Recall explicitly disabled to isolate
manual search. Maya's single sister relationship led to Nora's 1, 4, 12, or 24
accepted preferences. Each profile used three warmups and 30 measured turns in
fresh reader sessions. Timing includes the search turn, revalidation, provider
request composition, durable writes, and loading the resulting events; it does
not measure network/model latency or fixture setup. Percentiles use nearest
rank. These are development measurements, not a held-out answer-quality score.

| Preference fanout | p50 whole turn | p95 whole turn | Max complete request | Max memory data | Delivered Claims |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1 | 7.327 ms | 8.356 ms | 25,319 B | 3,876 B | 2 |
| 4 | 11.603 ms | 12.950 ms | 31,385 B | 9,354 B | 5 |
| 12 | 16.274 ms | 16.815 ms | 33,406 B | 11,179 B | 6 |
| 24 | 19.164 ms | 20.795 ms | 33,406 B | 11,179 B | 6 |

All delivered graph facts had complete exact source support. Fanout 12 and 24
reported truncation in all 30 samples; fanout 1 and 4 reported none. The complete
request includes unrelated fixed prompt/tool overhead, while “memory data” is
the synthetic evidence message content, not the full charged escaped message.
The measured bounds keep these fixtures below the existing budgets; this small
local dataset does not establish a production latency percentile for every
graph shape or database size.

To rerun on a specified checkout, compile an isolated executable and record its
hash before executing it:

```sh
go test -c ./internal/agent -o /tmp/evie-graph-tests
EVIE_GRAPH_REPORT_DIR="$PWD/cmd/evie/docs/fixtures/memory-stage5-graph/new-run" \
  /tmp/evie-graph-tests -test.run '^TestMemoryGraphBoundsExperiment$' -test.count=1 -test.v
```

## Verification and review entry points

The complete-turn regressions cover exact two-hop source/path support, Global /
General / second Workspace / Project A / Project B scope matrices with own and
sibling sessions, duplicate paths and cycles, direct-match competition,
contradictory polarity, retired entities/Claims, source retraction between
search and dispatch, actual versus unknown Valid Time, and result diversity.
The experiment reuses that public turn seam. Normal CI skips only the opt-in
performance experiment unless `EVIE_GRAPH_REPORT_DIR` is set.

Review `internal/eviedb/retrieval_graph.go` for query planning and bounded
discovery, `internal/eviedb/retrieval.go` for current dispatch eligibility,
`internal/eviedb/retrieval_inspection.go` for original sources, and
`internal/agent/retrieval_graph_acceptance_test.go` for observable boundaries.

Focused checks at this slice:

```text
go test ./internal/agent -run '^(TestMemorySearch|TestHistoricalMemorySearch|TestMemoryCorrectionBeforeDispatch|TestOriginalMemoryReceipt)' -count=1
PASS (2.706 s)
go test ./internal/web -run '^(TestMemoryEvidenceHTTP|TestHistorical|TestConversationEvidenceHTTP)' -count=1
PASS (0.773 s)
go test -race ./internal/agent -run '^TestMemorySearchTurn(Graph|FindsSourceBearing|ExplicitTime)' -count=1
PASS (15.582 s)
go vet ./internal/eviedb ./internal/memory
PASS
git diff --check
PASS
```

The Stage 5 PR handoff record owns the final full `verify-change.sh` result and
the remaining cross-ticket reader-quality gates; these local checks do not
claim those gates have passed.

### Compact source inspection

The existing source panel renders the frozen ordered `Reference.graph_paths`
separately from the currently inspectable `Evidence.graph_paths`. It displays
the stable anchor Entity ID and links each available supporting Claim to that
Claim's evidence/source section. Source event IDs remain attributable to their
own Claim. An unavailable bridge keeps its original content-free path position
but has no link or restricted source details; the panel reports that current
path support is unavailable. Relationship proximity is never presented as a new
accepted fact. Single-hop paths use the same presentation, and direct evidence
without graph support adds no path panel.

The three component tests cover ordered two-hop support mounted in the real
source view, browser fragment/target encoding, unavailable support, and a single
hop versus direct evidence. The mount first failed because the source panel did
not render the path or link targets, then passed after the minimal integration.

Commands run from `internal/web/ui`:

```text
npx vitest run src/artifacts/GraphEvidencePath.test.tsx
PASS (3 tests; 128 ms total, 8 ms tests)
npx tsc -b --pretty false
PASS
npx oxlint src/api/memoryEvidence.ts src/artifacts/MemoryEvidence.tsx src/artifacts/GraphEvidencePath.tsx src/artifacts/GraphEvidencePath.test.tsx src/artifacts/memoryEvidenceAnchor.ts
PASS (no warnings)
```

UI review entry points are `GraphEvidencePath.tsx`, its component test, and the
small mount in `MemoryEvidence.tsx`. `memoryEvidenceAnchor.ts` keeps source link
targets consistent; only the URI fragment, not the DOM ID, is percent encoded.

The staged #164 tree was also exported without later Automatic Recall edits.
The same focused agent command passed in 2.900s; the focused HTTP command and
`go vet ./internal/eviedb ./internal/memory` passed. The graph and source-panel
UI files passed together (12 tests, 141 ms total). The exported UI production
build passed with the existing large-chunk warning.

## Historical source preservation audit

The [preservation record](../fixtures/memory-stage5-graph/reproduction/README.md)
retains the exact recoverable source subset for both originally measured runs.
The original binaries and frozen hashes are unchanged, but v1 is missing four
frozen source files and v2 is missing two. Both included disabled draft Automatic
Recall code absent from the owning graph implementation commit. An exact rebuild
of those original experimental binaries cannot currently be demonstrated from the
preserved source; the manifests list the precise gaps without substituting later
code. The separately recorded deterministic owning-commit checks and original
measurement results remain distinct from that reproducibility limitation.
