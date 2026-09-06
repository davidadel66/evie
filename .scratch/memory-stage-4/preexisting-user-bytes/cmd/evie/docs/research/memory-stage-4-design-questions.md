# Stage 4 design interview

Date: 2026-09-04. Status: David accepted the recommendations in first-round
Q1–Q10 on 2026-09-04. Their authoritative record is the
[dated Stage 4 decisions](../active/memory.decisions.md), reflected in the
[memory roadmap](../active/memory.spec.md) and ADRs
[0062](../../../../docs/adr/0062-distinguish-candidate-support-from-interpretation-context.md),
[0063](../../../../docs/adr/0063-separate-candidate-progress-from-contiguous-coverage.md),
[0064](../../../../docs/adr/0064-let-owner-candidate-review-outlive-source-sessions.md), and
[0065](../../../../docs/adr/0065-separate-compiler-activation-from-historical-backfill.md).
Later-round recommendations remain provisional. This is not a completed Stage 4
specification or implementation authorization; model selection, the spike,
thresholds, and dependent contracts remain open.

The owner confirmed the shared Kernel compilation/review testing seam, and the
synthesized specification is published as
[issue #131](https://github.com/davidadel66/evie/issues/131) with the
`ready-for-agent` label. The issue retains the prerequisite contracts and
experiments; publication does not mark those deliverables completed.

## Starting point

Stage 3 is merged in commit `5e5e43d` (PR #115). Its cross-surface acceptance
test passed during the preceding status check. Existing authority remains the
[memory roadmap](../active/memory.spec.md), its
[decisions](../active/memory.decisions.md), the
[Stage 3 specification](../active/semantic-memory-stage-3.spec.md), and adjacent
semantic-memory ADRs. The roadmap header now records Stage 3 completion and the
limited scope of the Stage 4 first-round acceptance.

Stage 4 currently means asynchronous, local extraction of unaccepted Memory
Candidates from retained Episodic Memory. Human approval precedes accepted
Semantic Operations. Hybrid retrieval and automatic context injection remain
Stage 5. Accepted graph state remains independent of model and prompt versions.

Research supporting this round:

- [Extraction, scheduling, and scale](memory-stage-4-compiler-design.md).
- [Evaluation design](memory-stage-4-evaluation.md).

The accepted first-round recommendations are engineering judgments, not findings
that a paper has proved for Evie. Pilot scale, freshness, review cadence, and
corpus values remain hypotheses. A synthetic remote comparison is optional.
Dependent questions below still require resolution and are not approved merely
because their first-round prerequisites were accepted.

## First round: accepted product and architecture choices

### Q1. What deserves a memory candidate?

Should extraction seek every expressible fact or focus on information likely to
remain useful across conversations? For example, trying a programming language
once does not establish a lasting preference for it.

Recommendation: begin with enduring preferences, people and relationships,
project/Workspace decisions and constraints, and explicit meaningful changes.
Evaluate noise and missed useful information separately. Preserve original
episodes even when they yield no candidate. Task status and other changing
records should retain their existing authoritative stores; their duplication in
memory needs a specific use case and freshness policy.

### Q2. What can independently support a candidate?

The roadmap allows final assistant text as evidence. Should Evie be able to turn
its own guesses, or text repeated from a web page, into additional support for a
personal fact? What about a user statement whose provider response failed?

Recommendation: initially support direct owner assertions and individually
contracted tool observations. Assistant messages can provide bounded
interpretation context, but do not independently corroborate personal facts.
Handle quotations, hypotheticals, and reported speech explicitly in evaluation;
the user-message role alone does not prove an owner assertion. Durable eligible
evidence remains usable after a failed/cancelled turn. An unfinished tool intent
does not establish a tool outcome. This narrows the existing evidence policy.

### Q3. Is local-only extraction a firm product requirement?

Recommendation: keep private-history extraction local for the first release.
The spike should establish whether that constraint achieves useful quality on
the target hardware. A separately selected remote comparison using synthetic
fixtures could measure the quality trade-off, if desired. Any future use of real
history remotely requires an explicit extraction-specific policy; the existing
remote retrieval opt-in is not interchangeable with it. Never silently fall
back to a remote provider.

### Q4. What growth must the initial design accommodate?

Recommendation: one owner, one machine, multiple processes and Context Scopes,
and years of accumulated evidence. Measure increasing fixture sizes, initially
10,000 / 100,000 / 1,000,000 events with separately varied bytes per event,
accepted Claim counts, candidate counts, and scope/session distribution. These
are proposed stress levels, not capacity promises. Keep SQLite and bounded Go
workers; change storage architecture only in response to a measured limit.
Multi-host or multi-owner operation would change the current feature scope.

### Q5. How fresh should candidates be, and what resource cost is acceptable?

Recommendation: prioritize an interactive desktop and steady processing of new
evidence. Treat candidates becoming available within roughly a minute under
normal load as a hypothesis to test, not a promised gate. Start with one local
model request at a time across cooperating Evie processes; limit model context,
output, queued work, and database batch size. Use remaining capacity for history
backfill. Choose numerical latency/RAM budgets after the hardware spike.

Replace the literal promise that compilation never blocks the response with two
verifiable requirements: turns never await extraction, and measured foreground
latency/commit overhead stays within an agreed budget. SQLite write contention
and shared CPU/RAM still exist with background goroutines.

### Q6. What review experience would remain usable every week?

Recommendation: a scope-level inbox available after the original conversation
closes, with a small daily review session as an initial usability hypothesis.
Each proposed effect shows exact sources, scope, identity choices, and temporal
changes. Support accept, edit, and reject; a bounded batch may approve an exact
preview, with compound dependencies made visible. Preserve explicit acceptance
throughout Stage 4. Measure review seconds per useful accepted change and inbox
age, not only acceptance rate. Let measured burden inform subsequent admission
policy discussions rather than silently enabling automation.

### Q7. How should uncertain identity, meaning, or time appear?

Recommendation: make uncertainty inspectable. Two people named Alex remain
possible distinct identities; a possible future move is not a completed move;
an unknown effective date remains unknown. Offer resolver alternatives and
reviewed new-Predicate proposals instead of silently merging identities,
inventing precision, or changing a global Predicate's meaning. Start with a
small useful vocabulary and evaluate out-of-vocabulary cases. Model confidence
alone is not a calibrated probability or authority to accept a change.

### Q8. Should one failed extraction stop later candidates in the scope?

Recommendation: allow independent jobs to persist unaccepted candidates out of
order, while recording exact completed ranges and visible gaps. A contiguous
coverage frontier cannot cross an unresolved gap. Later work must not treat
missing output as a successfully processed empty result or depend on earlier
unaccepted candidates. Accepted Semantic Operations still serialize with current
revision checks. This explicitly changes the roadmap's blocked-head policy.

### Q9. What should enabling or upgrading extraction do to old history?

Recommendation: activate new evidence processing at an explicit captured
frontier, then offer bounded backfill by scope and history range. A new model,
prompt, or evidence policy creates a pinned generation; its output never
rewrites accepted memory. Backfill and comparison should be intentional and
visible rather than an unlimited automatic historical rerun. Excluded history
must be reported as outside the selected range, never falsely marked processed.
Preserve prior review decisions when presenting equivalent suggestions again.
This changes the existing automatic all-history reconciliation default.

### Q10. What trade-off should choose the extractor?

Recommendation: optimize supported useful candidates and affordable human
review, with precision favored over recall for suggested changes to existing
knowledge. Measure recall independently so abstaining on everything cannot
win. Compare models on the same frozen evidence, with a session-separated
holdout and repeated runs. Choose numerical gates from the pilot before tuning
on the final holdout. Retain exact deterministic conformance as release gates;
learned semantic judgments require human-reviewed labels and measured error
rates rather than claims of perfect accuracy.

## Evaluation recommendations already compatible with the existing ADRs

| Layer | Measurements | Stage 4 role |
| --- | --- | --- |
| Deterministic compiler/acceptance | Scope, source locator/hash, approval binding, idempotence, leases, crash/restart, gaps, atomicity, replay | Exact fixture gates; violations cannot be averaged away |
| Learned extraction | Useful-claim precision/recall, entailment, evidence selection, polarity, temporal interpretation, unsupported inferences | Human-reviewed gold cases; report denominators and error slices |
| Entity/Predicate resolution | False merges, splits, ambiguous abstention, duplicate amplification, Predicate reuse/novelty | Separate from extraction and acceptance correctness |
| Human review | Accept/edit/reject counts, useful accepted changes per review minute, stale previews, inbox size/age | Measure in Stage 4's actual review experience |
| Resources and scaling | Queue age, generation and resolution time, throughput, foreground latency delta, model/host memory, database/WAL growth | Paired compiler-off/on runs at increasing workload sizes |
| Retrieval and answers | Evidence recall, supported answer accuracy, temporal/update cases, abstention, harmful stale answers | Retain fixtures for Stage 5; any Stage 4 oracle-reader probe is diagnostic only |

Exact source matching proves that text is present at a permitted location; it
does not prove that the text entails the proposed proposition. An extractor's
confidence score and a second model's agreement are likewise insufficient by
themselves. Keep deterministic checks and semantic quality distinct.

## Later rounds: dependent questions and provisional recommendations

| Depends on | Question branch | Current recommendation, subject to earlier answers |
| --- | --- | --- |
| Q1, Q2 | Which fact classes, tool fields, quoted speech, and explicit memory commands are eligible? | Closed projection contracts; do not re-extract accepted explicit commands into duplicate suggestions |
| Q1, Q2 | How much neighboring evidence resolves pronouns without quadratic prompt growth? | Bound context within one session lineage; distinguish covered evidence from supporting context; retain every supporting source actually needed for meaning |
| Q2 | What closes crashed or command-only event ranges without invented outcomes? | Durable reconciliation over committed events with an explicit closure rule; successful-turn notifications are a scheduling aid |
| Q2, Q6 | How are field/range locators checked and rendered consistently? | One deterministic evidence resolver shared by extraction validation, review, acceptance, and inspection, with replay preserving the same source contract |
| Q3, Q4, Q5 | Which runtime/model/quantization/context size wins? | Pin candidates and measure; no selection from parameter count or advertised context size alone |
| Q4, Q5 | Where does the compiler run, including when the Memory Plugin is disabled? | Kernel-owned lifecycle in persistent runtime modes; optional explicit drain command; separate compiler configuration from model-visible tool exposure |
| Q4, Q5 | How are process-wide capacity, battery/idle rules, and fair scheduling enforced? | Durable shared capacity/lease accounting and bounded work; fresh evidence gets priority with explicit non-starvation rules |
| Q6 | What authorizes review after the source session closes? | A typed owner-authorized Candidate acceptance transaction, retaining original evidence authority, without reviving the old conversation |
| Q6, Q7 | What does editing a candidate preserve? | Keep original extraction and review lineage; bind approval to the exact normalized effect shown |
| Q6, Q7 | What happens if graph state changes while a preview is open? | Re-resolve against current accepted state, show the changed preview, and require approval of the new exact effect |
| Q6, Q7 | What makes an atomic review batch, and can independent items partially succeed? | Define dependent compound effects explicitly; preserve one inspectable transaction/result contract; decide after reviewing example interactions |
| Q7 | How are duplicate claims, source additions, contradictions, and real changes represented? | Reuse exact existing claims where appropriate; distinguish added support from changed/error correction; no inferred winner or silent cascade |
| Q7 | How may a candidate add a global Predicate definition? | Show new definitions as explicit effects; prohibit implicit redefinition and uncontrolled synonymous tokens |
| Q8 | Can a valid sibling candidate survive an invalid output item? | Persist per-item validation results if the envelope and source coverage are trustworthy; fail the whole job when they are not |
| Q8 | Which errors retry, and what does cancellation mean? | Bounded transient retries; permanent schema/evidence errors need inspection; stop applying after lease loss even if remote server computation continues |
| Q8, Q9 | How are jobs, retries, candidates, and source ranges identified? | Immutable logical source units and generation identities; persisted candidate identities survive retries; timestamps are not cross-session ordering authority |
| Q9 | Can newer evidence reopen a rejected suggestion? | Distinguish same-evidence repetition from materially new support; make rejection intent explicit and visible |
| Q9 | What is retained from old generations and model responses? | Retain enough typed output and identity for audit; bound diagnostics and obsolete staging; define retention before schema/cleanup implementation |
| Q10 | What corpus sizes, annotation rules, and thresholds are credible? | Human-reviewed development/holdout sessions, adversarial cases, explicit ambiguity labels, repeated runs, counts/uncertainty; set targets after the pilot |
| Q10 | Is a small usefulness probe needed before Stage 5? | A frozen offline reader can test representational sufficiency; it must be labeled oracle/diagnostic and cannot claim real retrieval improvement |

## Concrete code findings to resolve in the specification

- Existing `ApplyRememberLiteral` requires the original session lease and
  revalidates whole owner-message content (`internal/eviedb/semantic.go:1613`,
  `:1644`, `:1667`). Durable Candidate acceptance is a new authority/lifecycle
  contract, not simply an extra argument to a background call.
- Local approval reacquires the bound conversation's lease
  (`internal/agent/semantic_memory.go:261`). Approval audit identity must remain
  distinct from the source event supporting the fact.
- Event sequence is currently session-local
  (`internal/eviedb/events.go:313`). Multi-session compiler coverage requires
  explicit stable scheduling identity, not wall-clock ordering.
- Successful conversation completion is a final assistant event without tool
  calls (`internal/agent/turn.go:364`); failure/interruption and manual commands
  follow other paths. The terminal-job requirement needs an explicit eligibility
  and recovery matrix.
- Stage 3 source readers currently return event content; Stage 4's field/range
  evidence needs source-resolution and inspection changes as well as extractor
  validation (`internal/eviedb/semantic_lifecycle.go:898`,
  `internal/eviedb/semantic_correction.go:1079`).
- The existing evaluation report schema has closed Stage 3 metrics/failure
  vocabulary. Extend it with a versioned compatible contract before emitting
  Stage 4 extraction, review, and resource reports.

First-round decisions and the settled Compiler Generation/Compilation Coverage
terms are now recorded. The [Stage 4 specification](../active/semantic-memory-stage-4.spec.md)
synthesizes those decisions and makes the remaining contracts and standalone
model spike prerequisites to dependent implementation. The integrated compiler
and review pilot establishes numerical release gates before final acceptance;
the unrun experiments and provisional branches are not silently treated as
settled by writing the specification.
