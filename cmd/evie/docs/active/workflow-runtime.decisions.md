# Workflow Runtime implementation decisions

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
