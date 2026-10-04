# Memory architecture — research and proposed plan

Status: research complete, plan awaiting David's decisions (2026-10-02).

Trigger: "keep answers concise" was saved as a memory but never reached the
model, because Evie's only path from memory into a request is relevance-based
recall, and no request shares words with a style preference.

## What the research says

### 1. Standing context is always loaded, never retrieved

Every system that handles preferences well keeps a small, bounded tier in
every request and selects it by rule, not relevance.

| System | Always in context | Retrieved on demand |
|---|---|---|
| MemGPT / Letta | Core memory: a fixed-size "working context" for "key facts, preferences" pinned to the system prompt [1][9] | Recall and archival storage, searched via tools |
| MemoryOS | User Profile, Agent Profile and User Traits "all utilized" [2] | User knowledge base, top-10 by relevance |
| Claude Code | CLAUDE.md files (scope-ordered) and the first 200 lines / 25 KB of MEMORY.md [3] | Topic memory files, read when needed |
| ChatGPT | Saved memories and a summary of ~15 recent conversations, plus custom instructions [5] (reverse-engineered) | Chat-history reference |
| Claude.ai | `<userMemories>`: "distilled, stable facts about the user" [6] (reverse-engineered) | `conversation_search`, `recent_chats` tools, called selectively |
| Cursor | Rules with `alwaysApply: true`; user, project and team scopes [7] | Rules attached by file glob, by description, or by @-mention |
| LangMem | Profiles: "a single document that represents the current state", read by key [8] | Collections, by semantic search plus importance and recency |

CoALA treats the retrieval method as a per-memory-type choice ("rule-based,
sparse, or dense") [10]. Rule-based always-include is the standard answer for
preferences and identity.

**Evie today:** no such tier. Repository instructions are always loaded, but
memory claims only arrive via lexical/dense recall. That is the gap.

### 2. Scopes combine deterministically

Claude Code concatenates policy, user, project and local instructions from
broadest to most specific, so specific ones come later and win by position [3].
Cursor orders team, then project, then user [7]. This maps directly onto
Evie's Global, Workspace and session scopes.

### 3. How memories get written

- **Hot path, explicit:** the user says "remember X" (every system).
- **Hot path, model-proposed:** the Claude API memory tool lets the model
  issue file commands that the application executes [4]. ChatGPT proposes and
  the user confirms in conversation [5].
- **Background:** Letta sleep-time agents consolidate "without interrupting
  your active work", triggered after N steps or at compaction [9]. LangMem's
  "subconscious" formation runs after a conversation goes idle [8]. Claude.ai
  "periodically" updates memories in the background [6].
- **Update operations:** Mem0 extracts candidate facts (using a conversation
  summary plus the 10 most recent messages), retrieves the 10 most similar
  existing memories, and has the LLM choose ADD / UPDATE / DELETE / NOOP. Its
  graph variant marks conflicting relations invalid instead of deleting them
  "to enable temporal reasoning" [11].

### 4. Contradictions: invalidate, don't delete

Zep/Graphiti stores four timestamps per fact (created/expired, valid/invalid).
When facts conflict, it sets the old fact's invalid time and keeps it [12].
Evie's supersession and valid-time model already does this; keep it.

### 5. Retrieval quality (LongMemEval [13])

- Commercial assistants lost 30–64% accuracy versus reading the full history.
- Indexing at round granularity beats session granularity.
- Fact-augmented keys gave +9.4% recall@k and +5.4% accuracy.
- Time-aware query expansion gave +6.8–11.3% recall on temporal questions.
- Chain-of-note reading with JSON structure gave up to +10 points.
- Mem0 on LoCoMo: 91% lower p95 latency than full context (1.44 s vs ~17 s)
  and over 90% fewer tokens, for a few points of judge accuracy [11].

### What not to adopt

- A-MEM's "memory evolution" rewrites existing memories in place [14]. This
  conflicts with Evie's provenance and approval model.
- Mem0's physical DELETE. Use invalidation, as Mem0's own graph variant does.
- The Claude API memory tool's pull-only design [4]. A preference arrives only
  if the model obeys "view memory first", which is a behavioral guarantee, not
  a structural one. AGENTS.md asks for boundaries enforced in code.

## Proposed architecture for Evie

Four tiers, each with a named precedent.

1. **Standing context (new).** Approved standing memories (preferences,
   instructions, core profile facts) are rendered in full on every request.
   - Order: Global, then Workspace, then session.
   - Bounded size, placed in the stable cached prefix.
   - Selected by rule, never by relevance.
   - Precedents: MemGPT core memory, MemoryOS traits, CLAUDE.md, ChatGPT saved
     memories.
2. **Relevance recall (existing, improved).** Facts and conversation evidence
   via automatic recall. Add LongMemEval's fact-augmented keys and time-aware
   query expansion, measured on `internal/memoryeval`.
3. **On-demand tools (existing).** `memory_search`, `memory_query_claims` and
   the others, like Claude's `conversation_search`. Add one tool that lists the
   standing set and the owner's profile, so "what do you remember about me" is
   one call.
4. **Background consolidation (existing compiler, promoted).**
   - After a session goes idle or at compaction (Letta, LangMem), extract
     candidates, compare them with similar existing claims, and propose
     ADD / UPDATE / INVALIDATE / NOOP (Mem0).
   - Proposals land in the existing review inbox; nothing is accepted without
     approval.

Unchanged: approval-gated writes, provenance and source binding, scope rules,
supersession by invalidation.

## Plan (stages, each one reviewable change)

1. **Standing tier.**
   - Data: standing kind (by type, with an override), Global → Workspace →
     session order, hard budget with "memory full".
   - Rendering: every request, in the cached prefix, labelled as the owner's
     standing context.
   - Eval: a standing preference reaches every request, including ones that
     share no words with it.
   - Migrate David's two existing preferences.
2. **Profile and Workspace memory responsibilities.**
   - The system prompt states both duties.
   - Proactive proposals route to the review inbox; an explicit request still
     shows the approval card.
   - The card and memory UI show "Always applies" with the toggle; the Workspace
     brief renders in its Workspace.
3. **"What do you know about me / this project."** One read tool returning the
   standing set plus profile or Workspace facts.
4. **Always-on background pass.** Upgrade the memory compiler as in decision
   4, with the DeepSeek Flash default and an eval on the scale corpus for
   proposal precision.
5. **Episodic session summaries.** One stored summary per session, searchable
   on demand.
6. **Recall quality.** Fact-augmented keys and time-aware query expansion,
   gated on `internal/memoryeval` gains.

Context-management stages (after the memory stages; rationale and sources
in `docs/context-management-comparison.md`):

7. **General delegation.** Worker presets beyond web research: a read-only
   explore worker, optionally a task worker for long tool-heavy jobs.
8. **In-turn compaction.** Compact completed tool groups inside the active
   turn when clearing isn't enough.
9. **Tighter tool output.** Lower per-result admission and protect memory,
   todo and task results from clearing.
10. **Memory flush before compaction.** Trigger the Stage 4 background pass
    just before a compaction.

Orchestration stages (after context management; modelled on how this
review was run — isolated workers, verify, merge — and on Claude Code
sub-agents and worktrees):

11. **Build worker.** A general-purpose sub-agent whose writes are fenced to
    its own git worktree and branch, with a Task grant for its claimed
    subtree.
12. **Verify-and-merge gate.** Run the project's check command on the
    worker's branch; rebase, rerun; David approves the merge.
13. **Task Tree orchestration.** Parallel workers driven from the Task Tree;
    the orchestrator reconciles results and merges as each finishes.
14. **Workflow Runtime.** Deterministic large fan-outs (like a 100-agent
    research run), per `cmd/evie/docs/active/workflow-runtime.spec.md`.

Order: 1 and 5 in parallel; then 2 and 3; then 4; then 6; then 7–10 (7 first;
8 and 9 in parallel; 10 once Stage 4 has landed); then 11–14 in order (14 may
start after 11).

### Acceptance criteria

**1. Standing tier**
- Claims carry a deterministic kind: preference, instruction, profile fact,
  Workspace brief, or other fact. The model proposes it, the card shows it,
  and validation enforces it. Standing defaults to true for preference,
  instruction, profile fact and Workspace brief, and David can toggle it
  before approving.
- Every request renders active, approved, standing claims visible in the
  current scope, in Global → Workspace → session order, as a labelled data
  block in the stable prefix.
  - It changes only when the standing set changes, so cache hits survive.
  - It is never subject to relevance gating.
  - Retired, superseded and out-of-scope claims never render.
- Budgets: about 500 tokens for the profile (Global plus session) and about
  500 per Workspace brief, using the calibrated estimator. Approving a claim
  past the budget fails with "memory full", naming the current items and
  their sizes. Nothing renders truncated.
- A test proves a standing preference reaches a request that shares no words
  with it, in a new session, in another Workspace (for Global), and after
  compaction.
- David's two existing General-workspace preferences can be marked standing
  through the normal approval path; nothing is rewritten silently.
- Spec and decisions are amended, and old sessions' receipts still resume.

**5. Episodic session summaries**
- When a session is compacted, or goes idle after N turns, its summary is
  stored as one episode per session, with scope, time range and source event
  IDs.
- Episodes are searchable on demand through a read-only memory tool, under
  the same scope and egress rules as conversation search. They are never
  standing.
- Provenance points back to the session's events. The summary text is framed
  as data.
- No model call happens just to create an episode: it reuses the existing
  compaction summary or the background pass's output.

**7. General delegation**
- The orchestrator can delegate local, read-heavy work (reading many files,
  querying databases, investigating a repository) to an explore worker with a
  clean context.
- The worker returns the Stage 8 sub-agent result contract: summary inline,
  full report through `read_subagent_report`, `continue_research`-style
  continuation.
- Workers get only read-only capabilities. Writes, approvals and memory writes
  stay with the orchestrator.
- The same budgets, wrap-up, per-turn cap, framing and authority checks apply
  as for research workers.
- Prompt guidance: delegate when a task needs many tool calls or large outputs
  the orchestrator doesn't need to see.
- **Decided (David, 2026-10-03):** the explore worker gets `read_file`,
  read-only `query_db`, and a bash restricted to vetted read-only commands
  (for example `ls`, `rg`, `cat`, `git log`/`diff`/`show`,
  `sqlite3 -readonly`), enforced in code. The allowlist must reject
  write-capable flags and option-embedded execution (`git -c`, `--output`,
  pipes into writers, redirections).
- **Implemented** (a82bb8f).
- **Decided (David, 2026-10-03):** Workspaces with a local folder allow
  explore by default, confined to that folder, and a settings toggle can turn
  it off. Workspaces without a folder need the toggle. This is a follow-up:
  as merged, every Workspace still needs `SetWorkspaceExplore`.

Follow-ups found during Stages 6–7:
- Explore defaults and settings toggle, as decided above.
- Privacy in accepted-memory recall. Automatic recall doesn't
  relevance-filter accepted claims, and the newer-statement follow-up can add
  a private later message: "Write a short story about a therapist…" pulled in
  a private therapist message (scale corpus v5, default tier). Gate both on
  the same relevance and privacy rules as conversation excerpts. (Done
  2026-10-03, with dense-only hits gated too; see the retrieval decisions.)

**8. In-turn compaction**
- When tool-result clearing can't bring the active turn under budget, the
  completed tool groups of the active turn are summarized. The turn's user
  message stays verbatim.
- The cut never splits a tool call from its result.
- Persistence validators accept the new shape. Recovery and the C8 retry still
  hold.
- A test: one long agentic turn on a 128k model keeps working past the point
  where clearing alone would overflow.

**9. Tighter tool output**
- The per-result admission limit drops from 100 KiB (target set by
  measurement). Oversized outputs go to files with an excerpt plus paging,
  as bash and web_fetch already do.
- Results from memory, todo/task and sub-agent report tools are never cleared
  by projection (an `exclude_tools` equivalent).
- Cache stability is unchanged.

**10. Memory flush before compaction**
- Just before an automatic or manual compaction, the Stage 4 background pass
  runs over the turns about to be summarized.
- Its proposals land in the review inbox before the summary replaces those
  turns.
- It doesn't block the turn: it runs asynchronously over stored events, which
  remain in SQLite.
- **Implemented**, with the second point amended: the flush runs just after
  the compaction commits, as soon as the conversation is free (an automatic
  compaction's turn holds it, and the pass never commits during a live
  turn). Every owner message a committed compaction summarized stays owed to
  the pass until a pass covers it; see the Stage 10 entry in
  `cmd/evie/docs/active/memory.decisions.md`.

**11. Build worker** (decided 2026-10-03: free edits inside the worktree,
approval once at merge)
- New worker preset `build`. Its `edit_file`, file writes and bash run only
  inside a git worktree on a fresh branch created for the task. The fence is
  enforced in code: path resolution, symlinks and bash working directory.
  Writes outside the worktree, to the owner's working tree, or to `master`
  are refused.
- No per-edit approval inside the worktree. Approval-gated tools outside the
  fence (memory writes, finance edits, cron, network-changing actions) are
  not available to the worker.
- The worker gets a Task access grant scoped to its claimed task. It can
  update that task's status and result and add children under it, using
  revision checks. The orchestrator owns the rest of the tree.
- Same budgets, wrap-up, framing, per-turn cap and result contract as other
  workers. Its report lists the branch, commits and files changed.
- Tests:
  - fence escapes (`..`, symlinks, absolute paths, bash `cd`) are refused;
  - the grant can't touch sibling or parent tasks;
  - a crash mid-task leaves the worktree recoverable;
  - old receipts still resume.
- **Implemented** (2026-10-03). `delegate_build` (contract 1.0.0, Standard
  `sha256:535755ad…`; earlier versions reopen unchanged) takes a required
  Task and an optional base. The harness creates the worktree at
  `<repo>/.evie/worktrees/<execution>` on branch `evie/<id>-<slug>`,
  excluded through the repository's local `info/exclude`. The worker's file
  tools are confined through `os.Root`; its bash runs only under Seatbelt or
  bubblewrap (writes to the worktree and a private temp folder, no network),
  and a host without a sandbox gives it no bash. The worker never runs a
  git command that writes: the harness commits its work on its branch when
  the attempt ends, whatever ended it. Workspaces need the owner's explicit
  build setting (off by default). A crash leaves the worktree for `evie
  worktrees list|clean`. Binding record: the Stage 11 entry in
  `cmd/evie/docs/active/subagents.decisions.md`.

Decided (David, 2026-10-04) after Stage 11 landed:
- Project sessions may delegate build without a toggle; registering the
  project folder is the permission, as for research and explore. Only
  Workspaces need an explicit "on".
- Worker bash stays offline. Dependencies are installed before
  worker-modified code runs (see Stage 12's setup phase), or the worker
  reports what it needs.

**12. Verify-and-merge gate**
- Projects declare a verification command. Repository instructions or
  Workspace configuration can name it, for example
  `./scripts/verify-change.sh`.
- Before proposing a merge, Evie:
  1. runs the command on the worker's branch;
  2. rebases onto the target branch;
  3. reruns the command;
  4. shows David the diff summary and the results.
- The merge (fast-forward or rebase) happens only on David's approval. The
  worktree and branch are cleaned up after merge or abandon.
- A failing check returns to the worker through continuation with the
  failure output, bounded by budget.
- Running agents are told when the target branch moves.
- **Implemented** (2026-10-04). A Workspace or project has owner-set
  `setup` and `verify` commands (`evie workspaces verify`, `evie projects
  verify`, `POST /api/workspaces/verify`, the Workspace's "Merge checks";
  immutable revisions, latest applies at once). Evie only suggests values
  from `AGENTS.md`/`CLAUDE.md`. The setup command runs with network in each
  fresh build worktree on its base commit, before the worker, in the
  sandbox variant that still confines writes and credential reads (and, on
  macOS, closes this host's own addresses), and only on the owner's history;
  the worker stays offline. `propose_build_merge` (Standard `sha256:6566fc2a…`;
  earlier versions reopen unchanged) verifies the branch offline, rebases it
  onto the target inside the worktree (conflicts aborted), verifies again,
  and only then shows David "Merge evie/… into <target> (fast-forward)".
  On approval the harness fast-forwards the target, by `git merge
  --ff-only` in the checkout that has it (git refuses to overwrite his
  uncommitted changes; nothing is stashed or pushed) or by a
  compare-and-swap of its ref, and removes the worktree and branch. A
  failing check returns framed output for `continue_research`; a target
  that moved before approval is re-verified; other branches on the moved
  target are marked and running workers told; interrupted gate steps
  recover. Binding record: the Stage 12 entry in
  `cmd/evie/docs/active/subagents.decisions.md`.

**13. Task Tree orchestration**
- Given a Task Tree, the orchestrator dispatches independent ready tasks to
  build or explore workers in parallel, within the concurrency caps. Tasks
  that conflict on files or have dependencies run in order.
- Each worker claims its task. Results flow back as worker reports and Task
  updates; workers don't talk to each other.
- The orchestrator verifies, merges (through Stage 12) and moves to the next
  ready task.
- Progress is visible in the UI's task view.
- Restart-safe: claims, leases and worktrees survive a crash and resume or
  release cleanly.
- **Decided (David, 2026-10-04):** the orchestrator is the conversational
  model inside a turn, supported by deterministic harness tools, not a
  background scheduler; workers stay bound to the parent turn's authority.
  Orchestration that outlives a turn is Stage 14's.
- **Implemented** (2026-10-04). Tasks gain optional `depends_on` (Tasks of
  the same tree, cycle-checked: a subtask waits for its ancestors'
  dependencies and a parent for its subtasks, so ancestors and
  descendants are refused) and `paths` (relative files or folders), set by
  the orchestrating conversation only, through Todo 1.8.0 (`todo_add`,
  `todo_update`, `todo_decompose` with `depends_on_children`; earlier
  contracts reopen unchanged and refuse the fields). `todo_ready(tree_id,
  max?)` (Standard `sha256:377b51c0…`; `6566fc2a…` reopens unchanged)
  returns active leaf Tasks whose dependencies are completed and merged,
  unclaimed or claimed only by dead executions, outside running work, in
  first-fit batches with no two members conflicting, sized to the caps
  (`max_batch`, free `per_parent`, `runtime`, this turn's remaining
  `per_turn`); plus builds in flight with their next step, blocked Tasks
  with every reason (dependencies, claims, running work, conflicts with
  running or unmerged builds including those needing a rebase, marked
  blocked, no capacity) and Tasks that look finished. Two Tasks conflict
  when their paths overlap by component prefix (case-insensitive) or
  either has none. Build admission enforces the same rules in its
  transaction (dependencies, overlap within the call and with running
  builds in the repository, subtasks of running work); chats pinned
  before `todo_ready` keep parallel builds of path-less Tasks. Sessions
  with `todo_ready` and `delegate_build` get one guidance line: plan, read,
  dispatch the first batch, merge through Stage 12, update, repeat.
  Restart safety needs no new state: after a crash dead claims read stale,
  interrupted builds are offered on their kept branch and dependents wait.
  Progress: `todo_ready` for the model and `todo show <tree-id>` for the
  owner (status, plan, claim, worker execution, branch, gate step). The
  web UI has no Task view yet; adding one needs the owner's shell files,
  so it is deferred. Binding record: the Stage 13 entry in
  `cmd/evie/docs/active/subagents.decisions.md`.

**14. Workflow Runtime**
- Implement the reviewed, deterministic Workflow Runtime from
  `workflow-runtime.spec.md` and its ADRs. Large fan-outs (dozens to
  hundreds of AI nodes) run as a durable script with phases, bounded retries,
  structured node outputs and aggregation, not as a chat turn.
- AI nodes stay away from effects (ADR 0010). Concurrency is bounded by
  provider rate limits and cost caps, and each run's cost is reported.
- First workflow to ship: a deep-research workflow (search, fetch, verify,
  synthesize) equivalent to the one used for this plan.
- **Decided (David, 2026-10-04):**
  - Amend `workflow-runtime.spec.md`, which excluded both parallel branches
    and tool-using AI steps. Add a bounded parallel **map** step, plus
    **agent** steps that run the existing research, explore and build
    workers with their own fenced tools.
  - Plain AI steps keep the no-tools rule (ADR 0010). External effects still
    go only through the effect protocol.
  - Add the `gopkg.in/yaml.v3` dependency, parsed strictly, for
    `workflow.yaml`.
- **Sub-stages.** The spec's own delivery stages, plus the amendment, run
  in order. Each is one reviewable change, validated and merged without
  waiting for David (David, 2026-10-04):
  1. Spec amendment, then definition validation and simulation (strict YAML,
     canonical hashing).
  2. Activation and Standing Authority.
  3. Manual deterministic runs.
  4. Durable restart and leases.
  5. Capability reads.
  6. Effect protocol and reconciliation.
  7. Interruptions and AI steps.
  8. Cancellation and recovery.
  9. Agent steps and bounded map fan-out (the amendment).
  10. Deep-research workflow.
  11. Schedules and notifications.
  12. Migration and retention.
- **Sub-stage 1 implemented** (2026-10-04). `workflow-runtime.spec.md` is
  amended with map and agent steps (workflows cannot merge in v1; plain AI
  steps keep no tools). The new `internal/workflow` package parses
  `workflow.yaml` strictly (yaml.v3 with `KnownFields`; no anchors, aliases,
  merge keys, tags, duplicate keys, nulls or implicit conversions), validates
  the graph, references, types, limits, Run Key, schedule, capabilities
  against a catalog and Workspace Access, worker presets and budgets, map
  bounds and a reviewed function registry, hashes everything behavior-
  affecting (README, comments and descriptions excluded), prints a review
  report, and simulates over fixtures with effects as previews. `evie
  workflows validate <dir>` and `evie workflows simulate <dir> --fixtures
  <file>` expose it. The deep-research example lives in
  `internal/workflow/testdata/deep-research`. Binding record: the 2026-10-04
  entry in `cmd/evie/docs/active/workflow-runtime.decisions.md`.
- **Sub-stage 2 implemented** (2026-10-04). Definitions are proposed into a
  private bare procedural Git repository at `~/.evie/procedural`
  (`workspaces/<workspace-id>/workflows/<name>/`, written only with Git
  plumbing and a compare-and-swap of master) with their validation report
  and dry run, validated against the Workspace's Workspace Access (a new
  owner-set record of Connection IDs and exact resources, `evie workspaces
  access`). One owner approval (`evie workflows approve` or the web card)
  activates one exact executable hash and authority hash after a dry run
  that succeeded, checking authority within current access and the
  Workspace's worker allowances in one transaction; it records the schedule
  and starts no run. Versions are immutable, README-only proposals are
  recorded without approval, and access narrowing, worker revocation or
  archiving blocks a version until it is approved again. `evie workflows
  propose|list|show|diff|approve|deactivate`, `POST /api/workflows/…` and a
  Workflows section on the Workspace home. Binding record: the sub-stage 2
  entry in `cmd/evie/docs/active/workflow-runtime.decisions.md`.
- **Sub-stage 3 implemented** (2026-10-04). The owner starts a run of a
  Workspace's active workflow (`evie workflows run <workspace-id> <name>
  --input k=v | --input-json <file>`, `POST /api/workflows/start`, or the
  Workflow runs section on the Workspace home). Each start re-reads the
  approved commit from procedural Git and refuses it unless it still hashes
  to the approval, refuses step kinds this runtime cannot execute yet (only
  deterministic and branch steps run; nothing is recorded), validates the
  input, derives the Run Key, and records the run in the transaction that
  applies run admission (`authorizeWorkflowActivation`). The run pins the
  version, both hashes, the Workspace and access revisions and its
  Execution Composition. The database allows one active or succeeded run
  per Run Key; a duplicate start returns that run, and a failed one frees
  the key. Runs execute in the foreground: each node attempt is recorded
  before it runs and its output with the next checkpoint before the next
  node, deterministic steps retry within their policy and the run's limits,
  and terminal history is immutable. `evie workflows runs|inspect` and
  `POST /api/workflows/{runs,inspect}` show runs with every attempt. A
  crash leaves the run at its last checkpoint for sub-stage 4's recovery.
  Binding record: the sub-stage 3 entry in
  `cmd/evie/docs/active/workflow-runtime.decisions.md`.
- **Sub-stage 4 implemented** (2026-10-04). Runs are advanced under a
  fenced lease (holder, growing fencing token, expiry renewed by
  heartbeat), so the REPL, `evie serve` and a CLI start can share one
  database and exactly one of them records a run's progress at a time;
  every attempt, checkpoint, failure, pause and resume is refused for a
  stale holder. `evie serve` and the REPL run a background scheduler
  (`EVIE_WORKFLOW_WORKERS`, default 2) whose every poll, starting at
  startup, is also the recovery scan: a run whose process died is claimed
  once its lease expires, the attempt it left in flight is recorded
  `interrupted`, and the step retries from the same checkpoint within its
  Retry Policy. Retry backoff is a durable not-before time. Before a run
  continues, its pinned runtime must be declared compatible (ADR 0025),
  run admission must still hold and its approved commit must still hash
  to the approval; otherwise it pauses visibly (waiting for a dependency,
  with the reason) and resumes by itself once the cause clears. `evie
  workflows run` waits by default and takes `--detach`; inspection shows
  the lease and recovery events. Binding record: the sub-stage 4 entry in
  `cmd/evie/docs/active/workflow-runtime.decisions.md`.
- **Sub-stage 5 implemented** (2026-10-04). Plugins contribute capability
  reads through a focused workflow provider role, registered and validated
  by the Plugin Manager and callable only while the plugin and its
  connection are ready. The production catalog is two fixed reads of the
  local finance database, `finance.month_cash_flow` and
  `finance.month_transactions` (Finance 1.1.0, connection `finance-local`,
  resource `ledger`); web, YouTube and memory reads are left out on
  purpose. Definitions validate against the compiled catalog (the CLI's
  standalone review too), and runs with read steps execute: each run pins
  its providers (Capability ID, contract and schema hash, plugin version,
  Connection IDs), every claim pauses it visibly when a pinned provider is
  missing, changed or replaced, and `evie workflows substitute` records the
  owner's audited approval of a replacement that declares it continues the
  pinned one with the same contract and schema. Before each read, per-node
  admission re-applies run admission, the approved Standing Authority and
  current Workspace Access; the attempt is recorded under the fenced lease,
  the lease is checked again, and the provider is called with a context
  that a lost lease, shutdown or the read timeout cancels. Output is
  validated against the contract; failures retry with the step's durable
  backoff. Interrupted attempts no longer spend a step's Retry Policy or
  the run's attempt limit; a per-node bound (default 3) fails a run that
  keeps crashing instead (effects will never rerun after interruption).
  Binding record: the sub-stage 5 entry in
  `cmd/evie/docs/active/workflow-runtime.decisions.md`.
- **Sub-stage 6 implemented** (2026-10-04). Plugins contribute effects
  through a separate effect provider role: contract, idempotency, a
  Resource Conflict Key rule, a perform function whose only proof of
  non-performance is an explicit "not performed" error, and an optional
  reconciliation (lookup or idempotent replay by key); runs pin them in the
  same composition entries as reads. Each effect follows the durable
  sequence: per-node admission, the run's `max_calls_per_run` (counted by
  Effect Intents) and its conflict keys are checked in the transaction that
  records the attempt with its Effect Intent (stable idempotency key from
  the Workspace, Run Key, node and input; the authority evaluation); the
  lease is checked again right before the call; the response is recorded
  as the Effect Receipt with the output and next checkpoint in one
  transaction. A timeout, an unproven error, a malformed response or a
  crash between intent and receipt is Outcome Unknown: the run needs
  reconciliation, no dependent node, pause, resume or failure is recorded,
  and nothing retries it. A reconciling provider is asked at once (and
  later while it is unavailable); otherwise the owner reconciles with
  evidence (`evie workflows reconcile <run> <step> --outcome
  happened|did-not-happen`, `POST /api/workflows/reconcile`). A happened
  receipt supplies the step's output without another call; did-not-happen
  retries only within the Retry Policy. Effects sharing a conflict key are
  serialized across runs (and Workspaces) by a database-enforced hold;
  computation and reads proceed. No production effect provider ships: the
  candidates cannot prove what one request did, so the protocol is tested
  with fakes. Binding record: the sub-stage 6 entry in
  `cmd/evie/docs/active/workflow-runtime.decisions.md`.
- **Sub-stage 7 implemented** (2026-10-04). Interrupt steps record a
  durable typed question (message, declared input, answer schema, the
  run's originating session) and the run waits `waiting_for_human` with no
  lease. No scheduler resumes it, and nothing pauses, fails or advances it
  meanwhile. Any later session answers through `evie workflows answer
  <run> <step> --answer <json>` or `POST /api/workflows/answer`, or from
  the Workspace home's run view. The answer is checked against the step's
  schema, recorded once with its surface and session, and checked again
  before the graph advances. `evie workflows interruptions` and `POST
  /api/workflows/interruptions` list what is waiting by Workspace,
  session or run. Waiting for an answer does not count toward
  `limits.max_duration`. An effect whose outcome is unknown opens an
  `effect_reconciliation` interruption. Answering it and `evie workflows
  reconcile` record the same answer with the Effect Receipt in one
  transaction; the run state stays `needs_reconciliation`. AI steps send
  their pinned model (exact provider, model and parameters of a versioned
  Model Policy) only the reviewed prompt and the declared input fields,
  framed as data with the output schema. They send no tools, transcript,
  memory or credentials, which a capturing fake proves byte for byte. The
  answer must be one JSON object inside the schema; malformed ones retry
  within the Retry Policy. Transport failures that generated nothing retry
  at most twice. Each attempt records its model call before the call and
  its tokens and cost after, and the run's `limits.max_tokens` bounds its
  AI spend (not sent, or failed, visibly). A crash mid-call reruns the
  step and counts the lost call at its estimate. AI steps need
  `EVIE_WORKFLOW_REMOTE_MODELS=on` with `OPENROUTER_API_KEY`: without it a
  start is refused and a running run pauses before an AI step. Runs pin
  `evie.workflow.runtime/4`; `/1`–`/3` stay compatible; agent and map
  steps wait for sub-stage 9. Binding record: the sub-stage 7 entry in
  `cmd/evie/docs/active/workflow-runtime.decisions.md`.
- **Sub-stage 8 implemented** (2026-10-04). The owner cancels a run with
  `evie workflows cancel <run-id> [--reason]`, `POST
  /api/workflows/cancel` or the run view's Cancel run control. The
  cancellation is recorded once and moves the run to `cancelling`: no
  further node starts. A deterministic node in flight finishes; a read or
  AI call is stopped through its context and recorded interrupted (the
  call counts at its estimate); a question is withdrawn. An effect
  completes, or its outcome becomes unknown and the run stays
  `cancelling` until the receipt. Queued, paused and waiting runs are
  `cancelled` at once. A run that needed reconciliation is cancelled once
  its effect is reconciled, never retried. Cancelled runs keep their
  history and free their Run Key; finished runs are refused and never
  changed. `cancelling` is schedulable, so whichever Evie claims the run
  next, after a restart too, finishes it. Every node and question now
  applies run admission in the transaction that records it, so a
  deactivation or block pauses a running run at its next node boundary
  (before an AI step sends anything), and the owner can cancel the paused
  run. ADR 0021's audited override (`evie workflows run ... --rerun-of
  <run> --reason <text>`, or `rerun_of` and `reason` on the start route)
  reruns a succeeded logical execution as a new run. Its Run Key, and so
  every effect's idempotency key, includes the override, so effects are
  sent again rather than suppressed, and the override is recorded with
  the run. The decisions entry holds the state-to-recovery table. Binding
  record: the sub-stage 8 entry in
  `cmd/evie/docs/active/workflow-runtime.decisions.md`.
- **Sub-stage 9 implemented** (2026-10-04). Agent steps run the existing
  research, explore and build workers through the Kernel subagent
  supervisor as run-owned attempts: admitted and authorized by the Workflow
  Run's fenced lease and the node attempt instead of a parent turn, with
  the Workspace's pinned and current allowance for the preset, run
  admission and the worker's plugins checked at admission, at start, on
  every child write and by the watchdog, so losing the lease, the owner's
  cancellation, a pause or a revocation ends the worker. Each worker gets
  the reviewed objective with the step's input framed as data, the step's
  pinned Model Policy, a budget within the operator's policy and the run's
  limits, and the report contract; its result (with the typed `## Result`
  block when the step declares a schema; `partial` succeeds) is the
  step's output. An interrupted research or explore worker starts afresh,
  an interrupted build worker is continued in its kept worktree, and a
  worker that finished before its attempt was recorded is adopted.
  Workers are hidden children of a per-run worker session and inspection
  lists each one with its execution, usage and cost. Map steps run their
  items as per-item node attempts under the map's attempt: `max_items`
  checked before any item starts, at most `max_parallel` (and the worker
  caps) at once, input-ordered output, `fail` and `partial` policies,
  per-item retries and interruption bounds, per-item recovery after a
  crash, and the owner's cancellation reaching every item in flight before
  the run is cancelled. Every worker counts against `limits.max_tokens`
  (an item that would pass it is not started; the run fails visibly).
  Runs pin `evie.workflow.runtime/5` with each worker preset's
  composition; `/1`–`/4` stay compatible, an older Evie pauses a `/5` run,
  and only an Evie that runs workers can finish a run with a worker in
  flight (a database trigger). Workers need
  `EVIE_WORKFLOW_REMOTE_MODELS=on` and the Subagents plugin. The
  deep-research example runs end to end with scripted fakes; no workflow
  can merge a build branch. Binding record: the sub-stage 9 entry in
  `cmd/evie/docs/active/workflow-runtime.decisions.md` and the run-owned
  workers entry in `cmd/evie/docs/active/subagents.decisions.md`.
- **Sub-stage 10 implemented** (2026-10-04). Deep-research ships as a
  template bundled in the binary (`evie workflows templates`; `evie
  workflows propose <workspace-id> --template deep-research` proposes it
  with its bundled dry run, and the owner approves it as any definition).
  It plans at most four angles, researches each with a research worker,
  keeps at most twelve claims with sources a researcher fetched, gives each
  three independent verifiers that vote supported, refuted or
  insufficient with evidence they fetched (a vote without fetched
  evidence counts as insufficient), accepts a claim with two supported
  votes, writes the report from accepted claims only, and checks it:
  `check_citations@1` keeps only citations of sources a verifier fetched
  and relied on for an accepted claim, removes other citations, markers
  and links, and reports each. One pinned model
  (`deepseek/deepseek-v4.1-flash`), `limits.max_tokens` 6,000,000: at most
  $7.20 at its catalog price, about $0.70–$1.00 expected; the README gives
  the manual command for a measured live run. Agent step outputs keep the
  worker's summary, sources and typed result and name the execution whose
  report the report store keeps (`evie workflows report`), so checkpoints
  stay bounded (runtime `/6`; `/5` runs keep full reports). A new
  Workflows plugin gives chats `workflow_start` (a start of the
  Workspace's active workflow, through every start gate, proved against
  the chat's live turn and recorded with origin "chat") and
  `workflow_status` (bounded, framed status and outputs), with guidance to
  start deep-research for broad research questions; nothing model-facing
  proposes, approves, cancels, answers or reconciles. The web run view
  shows map items and worker calls, and a chat in the run's Workspace can
  take a finished run's build branch through the Stage 12 gate with the
  owner's approval. Binding record: the sub-stage 10 entry in
  `cmd/evie/docs/active/workflow-runtime.decisions.md`.

## Decisions (David, 2026-10-03)

1. **Standing by type, visible on approval.** Preferences and instructions
   about David become standing by default (the MemoryOS / LangMem profile
   rule). The approval card shows "Always applies", and David can switch it
   off before approving.
2. **Hard cap.** The standing tier has a fixed budget (about 500 tokens).
   Saving past it reports "memory full" and asks David to drop or merge first
   (ChatGPT style).
3. **Two memory responsibilities, stated in the system prompt.** The model
   proactively proposes what is worth remembering without being told
   (MemGPT, Claude Code auto-memory, ChatGPT):
   - **Profile:** David's preferences and facts about him, standing by type.
   - **Workspace memory:** the active Workspace's brief, decisions and
     conventions. The brief is standing in that Workspace; topic facts go
     to relevance recall.

   Proactive proposals go to the review inbox, not a blocking approval card.
   An explicit "remember this" still shows the card. Approval, scope and
   source binding stay enforced in code.
4. **Background pass: propose, David approves.** Upgrade the existing memory
   compiler:
   - from per-session opt-in to always-on, triggered at idle or every N turns
     across all conversations;
   - comparing candidates with similar existing claims and proposing
     add / update / invalidate / no-op (Mem0);
   - routing each proposal to profile or Workspace scope;
   - landing in the review inbox.

   It keeps reading only owner messages, so third-party text can't plant
   facts about David. Default model: `deepseek/deepseek-v4.1-flash`, pinned
   rather than a floating `-latest` alias. Optionally use its `:batch` variant
   (about 60% cheaper) because background latency doesn't matter. Local Ollama
   stays available.
5. **No recent-conversations digest.** Past conversations stay reachable on
   demand through recall and search tools (Claude.ai style).
6. **Project history.** Store one summary per session (reusing compaction
   summaries) as searchable episodic memory (Zep episode layer, LongMemEval
   session indexing). It is fetched on demand, never standing.

## Sources

1. MemGPT — https://arxiv.org/abs/2310.08560
2. MemoryOS — https://arxiv.org/abs/2506.06326
3. Claude Code memory — https://code.claude.com/docs/en/memory
4. Claude API memory tool — https://platform.claude.com/docs/en/agents-and-tools/tool-use/memory-tool
5. ChatGPT memory, reverse-engineered — https://llmrefs.com/blog/reverse-engineering-chatgpt-memory
6. Claude.ai memory, reverse-engineered — https://manthanguptaa.in/posts/claude_memory
7. Cursor rules — https://cursor.com/docs/context/rules
8. LangMem conceptual guide — https://langchain-ai.github.io/langmem/concepts/conceptual_guide/
9. Letta memory and sleep-time — https://docs.letta.com/guides/agents/memory, https://docs.letta.com/guides/agents/architectures/sleeptime
10. CoALA — https://arxiv.org/abs/2309.02427
11. Mem0 — https://arxiv.org/html/2504.19413v1
12. Zep/Graphiti — https://arxiv.org/html/2501.13956v1
13. LongMemEval — https://arxiv.org/html/2410.10813v2
14. A-MEM — https://arxiv.org/abs/2502.12110

Caveats: the ChatGPT and Claude.ai descriptions are reverse-engineered, not
official. OpenAI's own memory FAQ could not be fetched (HTTP 403). Benchmark
gains are self-reported by each paper.
