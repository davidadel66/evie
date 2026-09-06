# Stage 4 initial contracted clock observation

Ticket #143 implements evidence contract D2, `local-clock-display-v1`. It adds
`owner-clock-observations-v2` as an explicit generation/request evidence policy;
`owner-assertions-v1` continues to omit every tool outcome. Secret, closure,
window, identity and temporal policies retain their independent versions.
No model configuration or capability installation is performed by this change.

Only a durable successful `get_time` with an empty JSON object call is eligible.
The exact assistant-declared call must match its intent. An optional approval
must be approved, match the execution, and have no unrelated prepared-operation
hashes. The outcome must match that call and execution and be the sole terminal
outcome. Ancestry must stay in its original session, scope and root, with exact
recorded metadata pinned by a digest. Invalid linkage fails the source unit;
other named tools and failed/cancelled/resolved outcomes remain excluded.

The result is exactly 19 ASCII bytes, `YYYY-MM-DD HH:MM:SS`, with a valid date and
clock time. The sole projections are whole content and the `0:10` calendar date.
Payloads, JSON pointers, other subranges, envelopes and malformed fields cannot
be evidence. Projection and hash checks are shared by compilation, review,
accepted inspection, promotion, and historical replay. A transaction-local
cache shares each bounded event read between source and control validation.
The maximum 88-source fixture with a clock requires 89 event reads, including
the otherwise unoffered intent; indexed execution-count checks inspect no text.

A candidate using the clock must also cite an owner assertion. This check proves
co-citation and durable correlation, not whether the owner actually refers to
the checked date or the extracted proposition follows. Those remain extraction
and owner-review quality judgments. The clock means only that the runtime's
local clock displayed those bytes during the execution. It supplies no timezone,
location or trusted UTC instant. The fixtures keep Valid Time bounds unknown and
retain the local date as text; ObservedAt remains the original event audit time.

Clock candidates use owner-review preview/effect v4. Their immutable original
CompilerSource manifest pins the named observation contract and ancestry; their
accepted Source Links retain actor `tool`, source type `tool_succeeded`, and
authority `tool_observation`. Owner approval is separate audit authority. Source
origin traversal preserves that manifest through explicit Promotion without a
new operational record or Source Link schema column. V1/v2/v3 canonical fixtures
remain unchanged. V4 canonical bytes have a separate golden fixture.

Current source disclosure/secret policy may redact old accepted evidence and
operation envelopes. Historical replay checks the original bytes, contract and
source binding without invoking a clock, extractor, or conversational model.
A successfully committed clock remains eligible even if its surrounding turn
later failed or was interrupted; an unfinished captured intent does not.

## Deterministic demonstration

`go test ./cmd/evie -run '^TestOwnerReviewClockCLIActualCommittedToolPath$' -count=1`
runs the real agent tool-commit path with scripted provider/clock results, then
closes the session and exercises `memory-review inbox`, `prepare`, `resolve`,
and `operation`. It checks original tool authority and canonical replay. The
compiler and review never invoke the original capability. SQLite boundary tests
also cover exact date/whole projection, source/control mutation, malformed and
duplicate outcomes, approval and argument variants, foreign lineage, scope
visibility, Promotion, and current-policy redaction without rewriting history.

For an already produced candidate, use `go run ./cmd/evie memory-review inbox
--scope <scope>`, inspect the candidate, prepare an exact accept/reject preview,
and resolve that preview's digest and delivery key. Inspect the accepted
operation and Source Links. The deterministic test is sufficient to demonstrate
the engineering path; live extraction quality and human-output judgments remain
separate pending evaluation work.
