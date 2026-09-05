# New-evidence compiler activation

Engineering record for [#138](https://github.com/davidadel66/evie/issues/138),
under the binding [work contract](memory-stage-4-work-contract.decisions.md)
and [closure contract](memory-stage-4-evidence-contract.decisions.md).

The owner explicitly activates one exact source lineage and destination under
a verified generation. The default CLI selector is one session. `--lineage`
selects all sessions in that session's exact unscoped, Workspace, or project
lineage; it never widens a Workspace or project into global evidence.
`--session-scope` makes the destination the exact source session and cannot be
combined with `--lineage`.

The activation transaction captures the database append position. Its frontier
means selection starts afterward; older and legacy events remain outside live
selection. An event-position trigger writes a coalesced per-session obligation
inside the original append transaction. Notification loss and a full job queue
cannot lose that obligation. Reconciliation separately discovers bounded event
ancestry and revisits root obligations. Queueing, evidence cutoff, lease check,
source seal, generation identity, and new-evidence priority are committed in the
same serialized decision. No extraction runs during event commitment.

Ownership may be materialized out of order. Reconciliation finds the earliest
unowned interval within the activation's own bounds, stops before an existing
owner, and keeps any remainder revisitable. Resuming an older activation cannot
adopt a later suffix in place of its earlier evidence or select the disabled
interval between them. Existing explicit historical jobs retain their scheduling
and pause authority when activation discovers already-owned evidence.

Each discovery transaction reads the selected event once and follows at most
127 ancestors from that cached row. A chain that still requires another ancestor
is an explicit `source_inspection_limit` gap, not invalid lineage or a truncated
successful source. Each sealing transaction reads its root source once and at
most 127 other source events; the cached root participates in the full unchanged
128-event window. Sequence-only index coordinates and aggregate counts locate
the interval without inspecting content, ancestry, or source eligibility. The
boundary tests count every actual source/ancestry projection, including the
cached seed and root, and exercise both 128 and 129-event cases.

An open activation may be replaced by a new generation using its selector's
current revision. Replacement closes the old position interval and keeps its
selected work pinned. Disable closes selection and pauses that exact activation's
incomplete work. Resume requires its exact activation ID and original verified
generation; it resumes prior work without selecting the disabled interval or
reopening live selection. Activation and operation request IDs are idempotent.
The pause epoch prevents a pre-disable attempt from regaining publication
authority after a fast resume. Proven request-specific server release remains
separate from publication authority.

Activation and resume perform bounded metadata-only local endpoint verification
before capturing their frontier. This checks the configured model manifest,
runtime version, template, tokenizer and quantization and never calls inference.
A previously successful request returns its recorded frontier even if the
endpoint subsequently becomes unavailable. An already activated endpoint that
later becomes unavailable uses the worker's bounded retry policy. No configured
extractor creates no new materialized jobs. No production model has passed the
outstanding Stage 4 quality and runtime release gates; this implementation does
not install a configuration, choose a default model, or claim actual-model
acceptance.

## Owner demonstration

Use a complete, explicitly supplied local extractor JSON configuration and an
existing source session. The commands run before conversational provider
construction and exit without draining extraction:

```sh
evie memory-compiler activate --session SESSION --request activate-1 --revision 0 --config /absolute/path/compiler.json
evie memory-compiler status --session SESSION
evie memory-compiler disable --session SESSION --id ACTIVATION_ID --request disable-1 --revision 1
evie memory-compiler resume --session SESSION --id ACTIVATION_ID --request resume-1 --revision 2 --config /absolute/path/compiler.json
```

To start a fresh live interval after disable, use `activate` with a new request
ID and the current selector revision. Resuming a prior segment after replacement
still uses the selector's latest revision and the prior segment's exact ID.

Only long-lived REPL and web entries start configured supervision:

```sh
EVIE_COMPILER_CONFIG=/absolute/path/compiler.json evie
EVIE_COMPILER_CONFIG=/absolute/path/compiler.json evie serve
```

These hosts select and extract independently. Closing the host stops new claims,
cancels clients, and allows at most five seconds for worker cleanup. SQLite
retains unresolved work for a later configured host. Capacity with uncertain
server release stays blocked until the established worker release contract can
prove release; changing an endpoint or restarting Evie is not such proof.

## Deterministic observations

The real SQLite regression suite covers activation/append and replacement races,
rollback, post-frontier root suffixes, reopening without notifications, live
deferral, final/failed/interrupted/later-root/no-lease closures, queue saturation,
disabled intervals, and unavailable/unconfigured extraction. It records elapsed
foreground durable-finalization time while a scripted extractor stalls.
`CompilerReconciliation.DurationNanos` records each bounded scheduling pass.
These observations are fixture timings, not a production latency budget or pilot
measurement. The later pilot must measure realistic history and graph sizes,
foreground finalization, scheduling delay, and the selected model independently.

Review starts at the activation transaction and causal position trigger, then
the obligation reconciliation and worker epoch gates. CLI and runtime adapters
contain no authority or generation fallback.
