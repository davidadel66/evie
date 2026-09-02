# Semantic Memory evaluation fixtures

`v1/manifest.json` is the Stage 3 evaluation corpus. It composes the frozen
operation fixtures in `../../v1` through `../../v5` and adds fixed conformance
cases, source events, operation IDs, generated IDs, nanosecond clocks, expected
Scope Revisions, snapshots, exact queries and paths, and rejected operations.
Each case declares a fresh-database policy and its complete typed
Scope/Workspace/project/session registry. Named, kind-specific requests drive
the real Store prepare/apply APIs; expectations are checked independently of
the manifest's self-digest, including by a refreshed-digest tamper test.
Each closed snapshot pairs readable exact ID/lifecycle sets with gold
per-Scope canonical projection hashes and accepted-operation frontiers. Those
hashes cover every stable column in predicates, entities, aliases, Claims,
typed values, source links and evidence locations, graph links, corrections,
promotions, and state events, including valid/transaction times and operation
references; replay must reproduce the same rows exactly.
`v1/manifest.schema.json` closes that input contract. `v1/report.schema.json`
closes the shared output envelope while reserving distinct semantic,
learned-extraction, retrieval/provenance, and answer/abstention panels. Later
stages populate their reserved panel and component identities; they do not
reinterpret Stage 3 release gates.

The ordinary Go suite executes every deterministic release gate with a real
temporary SQLite database and no model, Capability, network, or external-effect
adapter:

```sh
go test -run TestSemanticEvaluation -v ./internal/eviedb
```

Full-text-search equivalence is intentionally absent: the binding Stage 3
specification defers FTS to Stage 5. Stage 3 covers typed inspection, direct
SQL, recursive-CTE traversal, replay, reopen, rollback, and rebuild
equivalence.

The test logs both the machine-readable JSON report and its Markdown summary.
Performance observations establish a local initial baseline only; they have no
absolute pass threshold. Persist a report externally when comparing runs, then
provide its run identity and metric values as the paired baseline. Stage 4 and
Stage 5 add results to their reserved report panels without changing Stage 3
case outcomes.
