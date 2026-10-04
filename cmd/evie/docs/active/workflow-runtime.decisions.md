# Workflow Runtime implementation decisions

## 2026-10-04 — Activation and Standing Authority (Stage 14, sub-stage 2)

Source: sub-stage 2 of Stage 14 in
[docs/memory-architecture.md](../../../../docs/memory-architecture.md) and
the sub-stage 1 entry below, which left this sub-stage the procedural Git
source, binding to a Workspace with its real Workspace Access, activation of
an exact hash with its requested authority, the worker allowance check and
the diffs between revisions. Binding: ADR 0007, 0008, 0011, 0012, 0016,
0020, 0024, 0025, 0027 and 0033 and this spec's approval decisions. No run
starts in this sub-stage.

### What was built

- **Procedural repository.** `internal/procedural` keeps reviewed
  procedural assets in a private bare Git repository at
  `~/.evie/procedural` (created on first use, mode 0700, branch `master`,
  identity `Evie <evie@localhost>`). That path was already on the file
  tools' denylist, and all of `~/.evie` is outside every explore root and
  sandboxed shell, so no worker can read or write it. Evie writes it only
  with plumbing (`hash-object`, `mktree`, `commit-tree`, then a
  compare-and-swap `update-ref` of master retried on top of a concurrent
  writer's commit): there is no work tree to leave dirty, and a crash
  before the ref moves leaves only unreachable objects. Every command runs
  with a built environment (no inherited `GIT_DIR`, index, global or system
  configuration), hooks off and no transport. A root that is a link, a
  file, or a folder holding anything but this repository is refused; paths
  are clean, relative and never name `.git`. A Workflow Definition lives at
  `workspaces/<workspace-id>/workflows/<name>/` with exactly its reviewed
  files: `workflow.yaml`, `README.md` and the prompts and schemas it
  references (nothing else in the source folder is copied).
- **Git source.** `procedural.Source` implements sub-stage 1's `Source` at
  one exact commit: a file must be a regular blob of at most 1 MiB, and a
  later commit never changes what an earlier commit reads.
- **Proposal.** `evie workflows propose <workspace-id> <dir> [--fixtures
  <file>]` validates the folder against the Workspace's current Workspace
  Access, the runtime's capability catalog and the operator's worker
  ceiling, simulates it with the fixtures (fixture problems refuse before
  anything is written), commits its files, validates the commit again (the
  hash must match), and records the proposal in SQLite with its review
  report and dry run. An invalid definition is not proposed. The proposal
  records the Workspace revision and access revision it was made under.
  Only the owner proposes in this sub-stage (CLI); a model-facing proposal
  tool is deferred, and the store already accepts `evie` as proposer and
  `chat` as surface for it.
- **Workspace Access.** Workspaces had no access record, so this sub-stage
  adds the smallest one the spec defines: per Workspace, Connection IDs each
  with exact resource selectors (opaque, no spaces or controls, sorted, no
  duplicates), stored as immutable revisions in
  `workspace_access_revisions`, written with a compare-and-set on the
  expected revision by the owner only (`evie workspaces access
  <workspace-id> --expect <n> --set <connection>=<resource>,... | --clear`;
  without flags it prints the current access). The latest revision applies
  at once. Access grants nothing by itself: no Account Connection registry
  exists yet, so an ID is only a ceiling entry.
- **Approval.** `evie workflows approve <proposal-id> --hash <executable>
  --authority <authority> --expect-version <n>` and `POST
  /api/workflows/approve` approve exactly what a card showed: the
  proposal's executable hash, its authority hash (`sha256` over a canonical
  document of the requested operations, order-insensitive), and the version
  the review compared with (0 for none). The runtime re-reads the
  proposal's exact commit and re-validates it against the current catalog,
  access and ceiling (the hash must be unchanged); then one SQLite
  transaction requires the hashes, the reviewed version (a stale review is
  refused), a dry run of that proposal that **succeeded** (ADR 0011's
  "completing a dry run"; failed, waiting and missing dry runs refuse), an
  active Workspace, the Workspace's current allowance for every worker
  preset its agent steps run (the same check chat delegation uses), and
  requested authority within the latest Workspace Access. It records version
  N+1 with the commit, both hashes, the authority, workers and schedule,
  the Workspace revision and access revision it was checked against, and
  the approver (`owner`, surface `cli` or `web`, a detail line and time).
  Approval starts no run; a declared schedule is recorded with it and
  starts runs once schedules ship (sub-stage 11).
- **Versions and history.** `workflow_proposals`, `workflow_activations`
  and `workflow_deactivations` are immutable (triggers refuse update and
  delete). A later approval supersedes the earlier version; re-approving an
  older proposal is how a rollback is done. The database itself refuses an
  approver other than the owner and a surface other than the CLI or web.
- **README-only changes.** A proposal whose executable hash equals the
  active version's is recorded as documentation of that version
  (`documents_version`) and needs no approval; the active version is
  unchanged (runs read its approved commit).
- **Blocking.** A definition's status is derived on every read and at run
  admission from immutable history: `proposed`, `active`, `inactive`
  (deactivated) or `blocked` with every reason. It is blocked while its
  Workspace is archived, while the Workspace's worker allowance no longer
  covers its agent steps (pinned and current revision and every revocation
  of the pinned one, exactly as for chats, so turning a worker off and on
  again does not restore it), or once any access revision after the pinned
  one failed to cover its authority (the effective access is the
  intersection of the pinned revision and every later one, so restoring
  access does not restore it). Only a new approval clears a block. Nothing
  is stored as blocked, so no path that changes a Workspace can miss it.
  `ActiveWorkflow` (and `authorizeWorkflowActivation` inside a transaction)
  is the run-admission check sub-stage 3 calls.
- **Deactivation.** `evie workflows deactivate <workspace-id> <name>
  --version <n> [--reason]` and `POST /api/workflows/deactivate` end the
  current version (compare-and-set on its number); it stays in history.
- **Diffs and the card.** `evie workflows diff <proposal-id>` and `POST
  /api/workflows/diff` show the approval card: both hashes, the current
  version and its status, the executable diff by report section (graph,
  inputs, outputs, run key, contracts, workers, functions, resources,
  retries, model policies, schedule, retention, limits, prompt and schema
  digests), the authority diff (operations added, removed, or changed in
  resources or call bounds), whether documentation changed, the requested
  authority, workers and schedule (highlighted: approving activates it),
  validation warnings, the dry run's outcome, every reason approval would
  be refused now, the file patch against the current version's commit, and
  the exact approve command. `list` and `show` give status, block reasons,
  versions and proposals.
- **Surfaces.** CLI as above; management routes `POST
  /api/workflows/{list,show,diff,approve,deactivate}` behind the usual
  loopback and JSON guard, with exact, bounded request bodies (an approval
  cannot name its approver or surface). The Workspace home has a Workflows
  section listing definitions with status and block reasons, and an
  approval card that approves exactly what it shows. There is no web
  proposal route, so the browser never names a folder to read. No
  model-facing tool proposes, approves or deactivates a workflow.

### Decisions and known limits

- Workspace Access lives in SQLite revisions, like the Workspace's
  presets, worker settings, folder and merge commands today, not yet as a
  reviewed procedural asset in Git as `workspaces.spec.md` intends for
  Workspace Revisions. Moving Workspace configuration into procedural Git
  is a Workspace slice of its own.
- Git first, then SQLite, with no `procedural_operations` journal: a crash
  between them leaves a commit no proposal names, which is harmless history
  and never approvable. Approvals and blocks are in SQLite only.
- The production capability catalog is still empty (sub-stage 5), so only
  definitions without capability steps can be proposed and approved today;
  Workspace Access matters once providers declare contracts.
- The conversational `bash` tool stays the documented privileged bypass: it
  could run the owner's CLI or call the loopback API, as for every other
  management surface. Approval still needs the exact hashes of a recorded
  proposal and a dry run that succeeded.
- Approval re-validates against the current worker ceiling; lowering the
  operator's ceiling later does not block an approved version (sub-stage 9
  checks budgets at admission).
- Sub-stage 3 must start runs only through `authorizeWorkflowActivation`
  in its run transaction, read the definition from the activation's commit
  through `procedural.Source`, re-hash it, and refuse a mismatch.

## 2026-10-04 — Map and agent steps; definition validation and simulation (Stage 14, sub-stage 1)

Source: the owner's decisions recorded under Stage 14 of
[docs/memory-architecture.md](../../../../docs/memory-architecture.md)
("Decided (David, 2026-10-04)"): amend the spec with a bounded parallel map
step and agent steps that run the existing workers, keep the no-tools rule
for plain AI steps, keep external effects in the effect protocol, and parse
`workflow.yaml` strictly with `gopkg.in/yaml.v3`. Sub-stage 1 is the
amendment plus definition validation and simulation.

Superseded wording in [workflow-runtime.spec.md](workflow-runtime.spec.md):
"V1 graph execution supports five node kinds" (now seven), "parallel
branches" in the deferred list and the out-of-scope "Parallel graph
branches" insofar as they would cover the bounded map step, the out-of-scope
"Letting AI Nodes call tools" insofar as it would cover agent steps (which
are not AI Nodes), and the ten-item delivery list (now the twelve
sub-stages). Every ADR stays binding as written; in particular ADR 0010 and
0019 still describe AI Nodes, which keep no tools.

### Amendment

- **Map step.** `over` names a list in an earlier step's output (never a run
  input); `item` is one deterministic, AI or agent step with no `id` or
  `next`; `max_items` (1 to 1,000), `max_parallel` (at most the run's
  `limits.max_parallel`), `on_item_failure` (`fail` or `partial` with
  `min_succeeded`). A longer list fails the step before any item starts.
  Output: `results` (succeeded items in input order, each `index`, `item`,
  `output`), `failures` (`index`, `item`, `error`), `succeeded`, `failed`.
  Nesting stops at one level: no map, branch, interruption or capability
  item. Capability items are left out of v1 because effects inside a fan-out
  would need per-item Resource Conflict Key serialization that sub-stage 6
  has not built; reads could be added later.
- **Agent step.** `worker` is `research`, `explore` or `build` (the existing
  immutable presets, no other and none chosen by a model); `objective` is a
  prompt file; `input` fields are assignment data; `model_policy` names a
  Model Policy, because a run has no parent session whose model a chat child
  would reuse, and ADR 0019's pinning applies to every model a run uses;
  `budget` (`time`, `tokens`) is required and at most the operator's
  subagent policy (`EVIE_SUBAGENTS_DEADLINE`, `EVIE_SUBAGENTS_TOKEN_BUDGET`;
  defaults 15 minutes and 1,000,000 tokens) and the run's limits. A build
  step must pass `task_id`. `output_schema` is optional; with it the worker
  is asked to end its report with a `## Result` section holding one JSON
  object, which is extracted (the last fenced `json` block in that section)
  and validated like AI output. `succeeded` and `partial` attempts succeed;
  an attempt whose typed result is missing or invalid fails and retries by
  its policy.
- **Fences.** Agent steps reuse the worker's own fences, admission checks
  and recovery. The Workflow Run's fenced lease takes the parent turn's
  place; sub-stage 9 builds that admission path, because the supervisor
  today admits children only under a live parent turn.
- **No merge in v1.** A workflow cannot merge a build branch: there is no
  merge step, and validation refuses any `subagents.*` capability node with
  that reason. Build branches stay for the owner's Stage 12 gate. Allowing
  it later needs an amendment declaring the exact repository and target as
  approved Standing Authority. Unattended merge would be the first workflow
  effect on the owner's own checkout, so it waits for the effect protocol
  and an explicit decision rather than riding on this amendment.
- **Authority.** Worker presets are not Standing Authority (ADR 0027: a
  preset exposes, it does not authorize); the run's Workspace allowance for
  each worker is checked at admission exactly as for chat children. Requested
  Standing Authority lists capability operations only.

### Sub-stage 1: what was built

- **Package.** `internal/workflow` owns the Definition model, strict YAML
  loading, validation, the review report, canonical hashing, the reviewed
  deterministic function registry and effect-suppressed simulation. It
  imports no other Evie package, no network and no process execution; a test
  enforces that. Definitions are read through a `Source` (a directory today;
  the procedural Git tree at a reviewed commit is sub-stage 2's seam).
- **Accepted YAML subset.** One document, at most 256 KiB. Refused: anchors,
  aliases, merge keys, explicit tags (`!!str`, `!x`), duplicate keys, null
  values (empty, `~`, `null`), non-text mapping keys, and plain scalars that
  YAML 1.1 and 1.2 read differently or that hide a conversion: `yes`, `no`,
  `on`, `off`, `y`, `n` in any case, `True`/`FALSE` (only `true` and `false`),
  dates, hexadecimal, octal, exponents, infinities, `+1`, `1_000`, and
  decimals other than digits with one point. Unknown fields are refused by
  `KnownFields`, then every scalar is checked against the field's type: a text
  field accepts only text (quote `"1.10"`), an integer field only a plain
  decimal integer, a number field an integer or plain decimal, a boolean only
  `true` or `false`. Fields that belong to another step kind are refused.
- **Definition.** `schema_version` (1), `name`, `description`, typed
  `inputs` (`string`, `integer`, `number`, `boolean`; `required` or a
  `default`; `enum`, `minimum`, `maximum`, `max_length`, `sensitive`),
  `run_key.inputs`, `model_policies` (`version`, `provider: openrouter`, an
  exact `model` not ending in `latest`, `temperature`, `max_output_tokens`),
  `limits` (`max_duration`, `max_attempts`, `max_parallel`, `max_tokens`, all
  required), `authority.operations` (`capability`, `connection`,
  `resources`, `max_calls_per_run`), optional `schedule` (five-field cron
  without names, IANA `timezone`, `catch_up` `skip` or `each_missed` with
  `max_runs` up to 31, `inputs` from literals or `$schedule.date` /
  `$schedule.time`), `retention.classification` (`ordinary` or
  `sensitive`), `start`, `steps` and `outputs`. Steps: `deterministic`
  (`function: name@version`), `capability` (`capability`, `contract`,
  `connection`, `resource`), `ai` (`model_policy`, `prompt`,
  `output_schema`), `agent`, `map`, `branch` (ordered `cases` of `{ref, op,
  value}` with `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `empty`, `not_empty`, and
  a required `default`) and `interrupt` (`message`, `response_schema`,
  `input`). Deterministic, capability, AI and agent steps and map items have
  optional `retry` (`max_attempts` 1 to 5, `backoff` only with retries); a
  step without `next` ends the run, and `start` names the first step.
- **References.** `$inputs.<name>…`, `$steps.<id>.output…` and, inside a map
  item, `$item…`, with dotted field names and no indexing. A literal text
  value cannot start with `$`. A step may read only steps that run before it
  on every path (dominators), so a branch cannot leave an input missing;
  outputs may name any step and the report says whether each is always
  produced. Field paths are checked against JSON schemas, function result
  types and capability contracts; function arguments and capability inputs
  must be assignable (a required field must be required by the source).
- **JSON schemas.** A closed subset: `type` (one of the six), `properties`,
  `required`, `items`, `enum`, `minimum`, `maximum`, `minLength`,
  `maxLength`, `minItems`, `maxItems`, `additionalProperties` (must be
  `false` on every object), `title`, `description`. Anything else (`$ref`,
  `oneOf`, `pattern`, `format`…) is refused. AI, agent result and
  interruption response schemas have an object root.
- **Validation rules.** Unknown fields, unsupported `schema_version`,
  a missing `README.md`, prompt (`.md`) and schema (`.json`)
  files (relative, inside the definition, read through `os.Root`), unknown
  or duplicate step IDs, unknown `next`/case targets, unreachable steps,
  any graph cycle (v1 allows none; retries are per step), missing or
  out-of-range limits, branches without cases or `default`, conditions
  whose operand or value type does not fit (or a value outside an enum),
  Run Key rules (empty, unknown, repeated, optional without default, a
  `number` input, more than eight inputs, or a schedule whose run key no
  `$schedule` input feeds), unknown capabilities or contract versions,
  connections and resources outside Workspace Access, capability steps not
  covered by requested authority, requested operations, connections or
  resources no step uses, more calls than `max_calls_per_run`, retried
  effects without proven idempotency, unknown model policies or worker
  presets, agent budgets above the worker ceiling or the run's limits, map
  bounds (including a list whose declared `maxItems` exceeds `max_items`),
  deterministic functions not in the registry, and literal arguments outside
  a function's bounds. Unused model policies and inputs, and map lists whose
  type does not bound their length, are warnings, as are worst-case agent
  tokens and attempts above the run's limits. Every problem names its path
  (`steps.research.max_parallel`) and YAML problems their line.
- **Function registry.** `collect_claims@1` (deduplicates claims from map
  results in order, keeps only sources the worker actually fetched, caps at
  `max_claims` and reports what it dropped), `expand_ballots@1` (one ballot
  per claim and voter, 1 to 7 votes) and `majority_filter@1` (accepts a
  claim with at least `quorum` `supported` votes; missing votes count
  against). They are pure, typed, versioned, and their result types are
  refined from literal arguments so list bounds are checked statically. A
  change to a function's behavior ships as a new version, so the hash of
  every definition that names it changes.
- **Executable hash.** `sha256` over a canonical JSON document: the
  normalized definition (descriptions left out, defaults applied, durations
  in milliseconds, steps, inputs, policies and operations sorted), each
  prompt's exact bytes' digest, each schema's canonical JSON digest, and
  each referenced function's `name@version` with its signature digest.
  `README.md`, comments, key order, quoting and formatting are excluded.
- **Report.** Graph in topological order with each edge, inputs, outputs
  and whether each is always produced, Run Key rule, required Capability
  Contracts, workers, deterministic functions, resources, retry policies,
  model policies, requested Standing Authority, schedule, retention
  classification and sensitive inputs, limits, worst-case bounds, file
  digests (README marked as outside the hash) and warnings. Text by default,
  JSON with `--json`.
- **Simulation.** A fixture file (strict JSON) supplies run inputs and,
  per step, AI outputs, agent worker results (`status`, `summary`,
  `sources`, `result`, `tokens`), capability read results, effect responses
  and interruption responses, with `attempts` for retries and, for maps,
  `items` by index and `item_default`. Deterministic steps run their real
  code and cannot be replaced by fixtures. AI output and agent results are
  validated as the runtime will (an agent result travels through the same
  `## Result` extraction), effects become previews and are never executed
  (their fixture gives only a simulated response; effect failures and
  reconciliation are simulated once sub-stage 6 builds the protocol),
  maps run their items in index order with the failure policy, branches
  evaluate their cases, and a missing interruption response stops the trace
  `waiting_for_human`. Run limits on attempts and tokens apply. The same
  definition and fixtures give a byte-identical trace. Nothing is written,
  no receipt exists and no authority is granted.
- **CLI.** `evie workflows validate <dir> [--json]` and `evie workflows
  simulate <dir> --fixtures <file> [--json]`, dispatched before the
  database opens. Until sub-stage 2 binds a definition to a Workspace and
  sub-stage 5 lets providers declare workflow contracts, the CLI validates
  against an empty capability catalog and an empty Workspace Access, so a
  definition with capability nodes is reported as unauthorized there; the
  worker ceiling is the operator's subagent policy.
- **Example.** `internal/workflow/testdata/deep-research` (plan angles, map
  research workers, collect claims, expand ballots, map verify workers with
  three votes per claim, majority filter, synthesize) with its prompts,
  schemas, README and fixtures; `testdata/notes-digest` covers capability
  reads and effects, branches, interruptions and schedules against a test
  catalog. Sub-stage 10 ships the deep-research workflow for real.

### Known limits

- Agent inputs are assignment data, and a research worker can fetch any URL,
  so data a workflow passes to a research step can leave through a fetched
  URL, as in chat delegation. Validation does not track sensitive data flow
  into agent steps; the report shows every step's inputs for review.
- Worker preset versions are not in the executable hash; like capability
  providers they are pinned by the run's Execution Composition (ADR 0016,
  0025).
- Resource selectors match exactly; no wildcard selectors.
- The report's worst-case token figure counts agent budgets only; AI step
  input tokens are unbounded by declaration and are capped by
  `limits.max_tokens` at run time.
