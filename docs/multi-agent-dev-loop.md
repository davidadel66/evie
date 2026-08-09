# Multi-agent development loops, and making them verifiable

A design doc, not a spec. The question: how does evie run *multiple* agents
against a coding task, in a way where you can trust the result without reading
every line yourself?

The short answer is that you already have the loop — you ran it for `web_fetch`
and `web_search` on 2026-08-03, and `web-fetch.decisions.md` records what it
caught. This doc is about (1) naming *why* that loop works so it can be
reproduced rather than remembered, (2) what Prime Agent's runtime does that's
worth stealing, and (3) how it becomes a thing inside evie rather than a thing
you drive by hand in Claude Code.

The load-bearing word is **verifiable**. Most "multi-agent" designs are several
LLMs agreeing with each other, which is worse than one LLM because the agreement
reads as evidence. The whole design below is organized around keeping a
deterministic checker at the center and treating agent output as untrusted until
it clears one.

---

## Start from what already worked

The `web_fetch` build ran four roles in sequence:

1. **spec review** — fresh-context agent reads the spec, no code
2. **test writing** — independent agent derives failing tests from the spec
3. **staged implementation** — the code gets written
4. **code review** — fresh-context agent reads diff + spec only

Recorded results, from `cmd/evie/docs/done/web-fetch.decisions.md`:

- The spec reviewer found **19 places an executor would have guessed**; three
  changed the design before a line was written (prompt injection as the real
  threat, URL credential stripping, HTTPS-upgrade exemption for private hosts).
- The independent tests caught a real bug (table cell separators emitted after
  each cell rather than between).
- The code review found **four real bugs**, including a credential leak through
  an echoed redirect `Location` header and a spill-file collision that would
  have had the model confidently grepping a stale path.

Note what bug #2 was: *"a spec bug faithfully implemented — the review caught
what the spec review didn't."* Different roles catch different classes of
defect. That's not a nice property, it's the entire justification for the
expense.

### Why it works — three separate mechanisms

Worth separating, because they have different costs and you can adopt them
independently:

**1. Context isolation.** A reviewer that watched the code get written shares
the author's blind spots — it has already accepted every premise. Fresh context
can't rationalize a decision it wasn't present for. This is cheap: it's just a
new session with a narrow input.

**2. Blindness constraints.** The test-writer *must not* read the
implementation. If it can, its tests describe what the code does; if it can't,
they describe what the spec requires — and the gap between those is exactly the
bug you're looking for. This is the single most important constraint in the
design, and see below: it needs *mechanical* enforcement, not a polite request.

**3. A deterministic gate.** `go build ./... && go vet ./... && go test ./...`
is not an opinion. Everything else on this page is a language model judging a
language model; this is the one place the loop touches ground.

The design rule that follows: **never let an agent vote on something a compiler
can decide.**

---

## The verifiability ladder

Concrete tiers, cheapest and hardest-to-fake first. A build's honest status is
the highest tier it actually cleared:

| Tier | Check | Cost | Fakeable? |
|---|---|---|---|
| 0 | `go build ./...` | free | no |
| 1 | `go vet ./...` clean | free | no |
| 2 | Pre-existing tests still pass (no regression) | free | no |
| 3 | **Spec-derived tests, written blind, now pass** | one agent | only by breaking blindness |
| 4 | Adversarial fresh-context review finds nothing new | one agent | yes — see failure modes |
| 5 | Live-fire demo against real input | your attention | no |

Tiers 0–2 are mechanical and should run on every iteration, automatically,
always. Tier 3 is the one that carries the actual verification weight. Tiers 4–5
are judgment, and tier 5 is the one you already insist on — a real
input/output demo rather than a list of passing test names.

**The vacuous-pass trap, which is the first thing to get right:** an empty test
file passes. So does a test that asserts nothing. Tier 3 needs an assertion
*about the tests themselves* — test count went up, the new tests fail against
the pre-change tree, coverage of the new file is non-zero. A gate that can be
satisfied by writing no tests is not a gate. The cheap mechanical version:

```
run new tests against the OLD tree  → must FAIL
run new tests against the NEW tree  → must PASS
```

If step one passes, the tests don't test the feature. This is worth building
before anything else on this page, because it's the check that makes tier 3
mean something, and it's twenty lines of shell.

---

## What Prime Agent does, and what's worth taking

Prime Agent (`github.com/PrimeIntellect-ai/prime-agent`, open source,
TypeScript) is the most complete public example of a recursive multi-agent
harness. Its subagent mechanism, from `docs/rlm.md`:

```python
handle = await rlm("Review the authentication flow for security issues",
                   name="auth-reviewer")
print(handle.rlm_child_id, handle.name, handle.session_dir, handle.model)
```

### Adopt: spawn returns immediately, never the answer

> "The call returns immediately after task admission with a child handle; it
> never waits for or returns the child's answer. Results arrive only through
> explicit `agent_message` replies or files, never as an `rlm()` return value."

This is the design decision to copy, and the reasoning is not obvious until you
try the alternative. A *blocking* spawn that returns the child's full output
puts the child's entire trajectory into the parent's context — which defeats the
whole purpose. You spawned a sub-agent to keep work *out* of the parent context;
returning everything it did puts it right back.

The pattern that falls out: children report **verdicts, not transcripts**.
Anthropic's context-engineering guidance says the same thing from the other
direction — sub-agents should return condensed summaries in the 1–2k token
range. For a code task, the artifact is the file and the git diff; the child's
message back should be `PASS`, `FAIL + 3 findings`, or `here's the path I wrote`.
Never the reasoning that got there.

### Adopt: bounded recursion depth

Default in Prime Agent: a root agent may create children; descendants need
raised config to recurse further. Depth 1 covers every role in the loop above.
Depth 2 is where cost becomes unpredictable and a bug becomes a fork bomb with
your API key.

### Adopt: family-scoped messaging

Prime Agent restricts messaging to the "nuclear family" — parent, sibling,
child — "to prevent undesired cross-session communication." Adopt as a
constraint from day one; it's free now and awkward later.

### Adopt: a child registry that survives restarts

> "The parent-scoped child registry survives compaction, kernel restart, and
> parent restoration."

Depends on JSONL transcripts (`harness-improvements.md` #4). Which is a real
ordering constraint: **persistence comes before sub-agents**, because a
multi-agent build that can't survive a crash isn't usable for anything long
enough to need multiple agents.

### Reject: the persistent IPython kernel

Prime Agent's central bet is that a persistent IPython kernel is the *only* tool
interface, and the model writes Python that calls tools as functions —
intermediate results live in kernel variables instead of the context window.
Genuinely elegant. Wrong for evie: Jupyter/ZeroMQ transport, kernel lifecycle,
a Python runtime dependency, namespace snapshotting. Already argued in
`harness-improvements.md`; evie's `bash` tool with persistent cwd gets a slice
of the benefit for none of the cost.

### The gate, from their autonomous mode

```bash
prime-agent --autonomous --autonomous-gate "npm run check" --autonomous-max-turns 20
```

Plus `--autonomous-max-tokens` and `--autonomous-timeout-ms`. The gate is the
load-bearing flag and the bounds are the safety rail. **Bounds without a gate is
a runaway loop with a timer** — the agent stops, but not because it succeeded.

Evie's equivalent is `go build ./... && go vet ./... && go test ./...`, which is
fast enough to run every single turn. That's a real advantage of a Go project
over a JS one and worth exploiting: the gate can be part of the loop rather than
a phase at the end.

### Meta's Muse Code — one idea worth lifting

Not open-sourced (neither harness nor weights), so this is
architecture-by-description. Two useful bits from the announcement:

- An append-only event log of "every model call, tool run, approval, and edit,"
  which they credit for making the runtime "replay-exact and restart-safe."
  Applied here: the build report below.
- A bundled **`/grill`** skill that stress-tests a plan before executing it.
  That's your spec-review step with a better name and an explicitly adversarial
  framing. Cheap to adopt.

---

## Designing it for evie

The good news, and it's genuinely good: `agent.Session` is already the right
unit. A sub-agent is a `Session` with a different system prompt, a different
tool set, and its own transcript. `Events` is already the seam that captures
what it produced. Nothing here is a rewrite.

Four things are missing.

### 1. Sessions can't be configured per-role

`New(client, model)` hardcodes the system prompt as `messages[0]`, and
`Send(..., extra ...tools.Tool)` is purely **additive** — a frontend can add
tools but never remove them. Every role in the loop needs *fewer* tools than
the default, not more:

| Role | Needs | Must NOT have |
|---|---|---|
| spec-reviewer | `read_file` (spec only) | any write tool |
| test-writer | `read_file`, `edit_file`, `bash` | read access to the implementation |
| executor | everything | — |
| reviewer | `read_file`, `bash` (for `git diff`) | any write tool |

So the shape needed is subtractive. Something like a role/profile that carries
a prompt and a tool allowlist:

```go
// Role is what makes one Session different from another: the system
// prompt it opens with and the tools it may call. The zero Role is the
// default assistant — every field empty means "use the defaults".
type Role struct {
    Name    string   // "reviewer" — names the transcript, not sent to the model
    Prompt  string   // replaces the default system prompt
    Allow   []string // tool names; nil means every registered tool
}
```

And `tools` grows the filter to match — note it should return an error on an
unknown name rather than silently ignoring it, or a typo'd allowlist becomes a
security hole that looks like a working config:

```go
// SchemasFor is Schemas restricted to allow. An unknown name is an
// error, not a skip: a typo in a role's allowlist must fail loudly
// rather than quietly granting the default set.
func SchemasFor(allow []string) ([]openrouter.Tool, error)
```

The design question worth sitting with: is `Allow` an allowlist or a denylist?
Allowlist. Adding a tool to the registry should never silently widen what a
reviewer can do — a new write tool must not become available to a read-only
role because nobody remembered to update a denylist. This is the same reasoning
as `Decision`'s zero value being `Declined`: **fail closed on the axis that
matters**, and the repo already made that call once.

### 2. There's no spawn

A `spawn_agent` tool, or a code-driven orchestrator, needs to construct a child
`Session` and run it. The key constraint from Prime Agent: what comes back is
the child's *last message*, not its transcript. The child's full trajectory goes
to its own JSONL file, where you can read it if the verdict looks wrong.

Note that this is currently impossible to do well without the transcript work —
a child whose output you can't inspect after the fact is a child whose verdict
you can't trust. Ordering constraint again.

### 3. There's no isolation

Two agents editing files in the same directory corrupt each other, and the
failure is confusing rather than loud. Git worktrees are the answer, and since
`bash` already exists it's nearly free:

```
git worktree add /tmp/evie-build-<id> HEAD
```

Each child gets its own checkout. The parent reads results via `git diff`
between worktrees. Cheap, no new dependency, and it makes the blindness
constraint mechanical rather than aspirational — which is the next point.

### 4. Blindness needs to be enforced, not requested

This is the part most designs get wrong. "Don't read the implementation" in a
system prompt is a *request*, and an agent that reads the file anyway produces
tests that look great and verify nothing. You will not be able to tell from the
output.

Two mechanical enforcements, in increasing order of strength:

- **Tool-level:** the test-writer's `Allow` list excludes `read_file` and
  `bash`, so it *cannot* reach the file. Simple, but it also can't run the tests
  it's writing, which hurts.
- **Filesystem-level (better):** the test-writer works in a worktree checked out
  at the pre-change commit. The implementation file **does not exist there**. It
  keeps `bash` and `read_file` — it can explore the repo, read the spec, run the
  existing suite — and blindness holds because there's nothing to peek at.

The second is strictly better and costs one `git worktree add`. It also gives
tier 3's must-fail check for free: the tests it writes run against a tree
without the implementation, so "these tests fail here" is observable in the same
worktree where they were authored.

---

## Orchestration: workflow, not agent

Two ways to drive this:

**(a) Model-driven.** Evie gets a `spawn_agent` tool and decides for itself when
to delegate. Flexible; unpredictable; hard to reproduce; hard to put a gate in.

**(b) Code-driven pipeline.** An `evie build <spec>` subcommand that runs the
fixed sequence in Go, with the gate between stages.

**Start with (b).** Anthropic's guidance is explicit — use a workflow where a
workflow suffices, and reserve agents for cases that genuinely need model-driven
decisions. The autopilot sequence isn't discovered per task; it's the same four
roles every time. Encoding it in Go buys reproducibility, a natural place for
the gate, and a build report that's the same shape every run. (a) is the
generalization to build later, if the fixed pipeline turns out to be too rigid —
and you'll know, because you'll be fighting it.

Sketch of the pipeline, with the gate as a first-class step rather than an
afterthought:

```
evie build cmd/evie/docs/active/<feature>.spec.md

  stage 0  worktree: git worktree add <base>  (pre-change HEAD)
  stage 1  grill      → findings on the spec; STOP if any are blocking
  stage 2  tests      → in <base>, blind by construction
           gate       → new tests must FAIL here  (else vacuous)
  stage 3  build      → executor writes code (David, or an agent)
           gate       → build + vet + old tests + new tests must PASS
  stage 4  review     → fresh context, diff + spec only
  stage 5  demo       → real input, real output, your call
  report   →  ~/.evie/builds/<id>.md
```

Stage 3 is where the tutor stance bites, and it's the right place for it:
**for anything David is learning, stage 3 is David.** The pipeline's value isn't
writing the code — it's stages 1, 2, and 4, which are the ones that are tedious,
context-hungry, and mechanical. Keeping stage 3 human is a feature of this
design, not a limitation of it. For genuinely rote work (a mechanical rename
across 30 files) stage 3 can be an agent, and the gate is what makes that safe.

---

## The build report is the deliverable

"Verifiable" has to produce an artifact, or it's just a good feeling at the end
of a session. Every run writes `~/.evie/builds/<id>.md` containing:

- The spec, by content hash — so you can tell whether the spec changed mid-build
- Each stage: role, model, token cost, the **verdict only** (the transcript is a
  separate JSONL file, linked)
- Every gate invocation: exact command, exit code, output — verbatim, not
  summarized
- **Highest tier cleared**, and which tiers were skipped and why
- Known gaps shipped deliberately

That last field is doing real work. `web-fetch.decisions.md` has a *"Known gaps,
shipped deliberately"* section listing lone-space lines, unstripped nav chrome,
UTF-8 only, no SSRF denylist. A build report without that section reads as
"everything passed," which is a different and false claim. The gaps section is
what makes the report honest, and it's the field most likely to get dropped
because it's the only one an agent has no incentive to fill in.

This is the same idea as Muse Code's event log, scoped to one build instead of
one session.

---

## The RL framing, briefly

Prime Intellect's other product is `verifiers` and the Environments Hub, where
"environments define the world, rules and feedback loop of state, action and
reward." The gate command *is* a verifiable reward function: program in, scalar
out, no human in the loop.

Worth one paragraph even though you will not be training anything: designing the
gate as if it were a reward function forces the definition of "done" to be
**mechanical**. If you can't write the check as a program, you don't have a
spec — you have a preference. That's a useful test to apply to specs before
building, and it's free.

The speculative version, parked: with JSONL transcripts (`harness-improvements`
#4) plus a gate, every build produces a labeled trajectory — actions, and a
verifiable outcome. That's training data. Nothing to do about it now, but the
data is worth *keeping* from the start, because retrofitting it is impossible.

---

## Failure modes, and what to do about each

These are the ways this design fails in practice. Each is worth designing
against explicitly, because each one produces output that *looks* like success.

**Reviewer finds nothing.** The default failure. A reviewer asked "is this good?"
says yes. Fixes: prompt it adversarially — *try to refute this diff* — require a
verdict per finding rather than a summary, and treat a clean review with
suspicion when the diff is large. `web-fetch`'s reviewer found four bugs in one
tool; a clean review on a comparable diff is more likely a bad reviewer than
good code.

**Vacuous gate.** Covered above — no tests means tests pass. The must-fail-first
check is the answer and it's the highest-value item here.

**Blindness broken.** Covered above — enforce with a worktree, not a prompt.

**Tests that restate the spec's examples.** If the spec contains a table of
cases and the test file is that table, you've verified transcription, not
behavior. Ask the test-writer for edge cases the spec *doesn't* enumerate — that
also surfaces spec gaps, which is the spec reviewer's job done twice from a
different angle, and cheap.

**Agents sharing a worktree.** Loud-looking bug, confusing in practice. One
worktree per child, enforced at spawn.

**Infinite fix loops.** Bound turns and tokens per stage. A stage that exhausts
its budget is a *failure to report*, not a reason to raise the budget.

**Cost.** Four roles against a real task is expensive, and the sequence is
serial so it's slow too. Mitigations: the mechanical gates are free and should
absorb as much verification as possible; cheap models for cheap roles (spec
review and test writing tolerate a smaller model far better than review does);
stages 1 and 2 can run concurrently since neither depends on the other.

---

## Where this sits relative to the other doc

`harness-improvements.md` has hard prerequisites for this one:

- **#4 JSONL transcripts** — required. A child agent whose trajectory you can't
  inspect after the fact is a child whose verdict you can't trust, and the child
  registry needs somewhere to live.
- **#1 usage capture** — required for cost accounting in the build report. Four
  agents per build makes cost a real number you'll want to see.
- **#2 system prompt in a file** — the role prompts are the same mechanism
  applied four more times. Do it once for the default and the pattern is set.
- **#5 compaction** — a long build stage will hit the window. Not required for a
  first version if stages are short.

So the honest sequencing is: **transcripts and usage first, then this.** Which
is convenient, because those are the two cheapest items on the other list.

---

## Open questions

Things a spec would need to settle, flagged rather than guessed:

1. **Does the gate run between stages, or continuously?** Go's build+test is
   fast enough to run every turn, which is a real advantage over a JS project.
   Continuous is stricter and catches breakage earlier; per-stage is simpler.
2. **Who writes stage 3 by default?** Tutor stance says David for anything with
   learning value. Is there a `--auto` flag for rote work, and what makes work
   rote enough to qualify?
3. **What's a *blocking* spec finding?** The spec reviewer found 19 issues on
   `web_fetch`; 3 changed the design. If all 19 block, the pipeline never
   advances. Severity needs to come from the reviewer, and severity is exactly
   the thing models are unreliable about.
4. **Concurrency.** Stages 1 and 2 are independent. Worth the complexity in v1,
   or serial until it's annoying?
5. **`evie build` vs. a tool.** A subcommand is a workflow (recommended above).
   But evie-in-chat can't invoke it, which means you'd drive builds from the
   shell rather than by asking. Is that fine, or does chat need a door?
6. **Does the spawned child get the approval gate?** A child with `edit_file`
   hits `NeedsApproval`, and there's no human attached to a background child.
   `Decision.Expired` exists for exactly this case — no human ever saw the
   request — so the machinery is there. But the *policy* isn't: does a child in
   an isolated worktree get auto-approval on the grounds that the worktree is
   the sandbox? That's the most security-relevant open question here, and it
   deserves its own decision entry.

---

## Sources

**Prime Agent** (Prime Intellect, open source, TypeScript,
`github.com/PrimeIntellect-ai/prime-agent`) — the primary source:
- `packages/coding-agent/docs/rlm.md` — the RLM programming model, `rlm()`
  spawn semantics, fire-and-forget admission handles, `agent_message` replies,
  child registry and lifecycle, recursion depth
- `.../rlm-runtime.md` — host/kernel split, typed host requests, why admission
  responses use the Jupyter control channel (a deadlock story worth reading even
  though evie won't have a kernel)
- `.../long-running-agents.md` — daemon-backed sessions, goals, heartbeats,
  autonomous mode with `--autonomous-gate`, agent-to-agent messaging
- `.../extensions.md` — `tool_call` interception with `{block: true, reason}`,
  which is a permission gate with a different name than evie's
- Blog: `primeintellect.ai/blog/prime-agent`
- `primeintellect.ai/blog/environments` — `verifiers`, the Environments Hub, and
  the state/action/reward framing behind the "gate as reward function" section

**Anthropic:**
- *Building effective agents* — workflow vs. agent; use the simpler one. The
  direct justification for the code-driven pipeline over a `spawn_agent` tool
- *Effective context engineering for AI agents* — sub-agents return 1–2k-token
  condensed summaries, not transcripts

**Meta Muse Code / Muse Spark 1.2** (`research.meta.ai/blog/introducing-muse-code-and-muse-spark-1-2`,
announced 2026-08-05, beta, **not open-sourced**) — append-only event log for
replay-exact restart-safe runs; `/plan`, `/grill`, `/goal` bundled skills.
Architecture-by-description only.

**This repo — the empirical evidence, and the best source on the list:**
- `cmd/evie/docs/done/web-fetch.decisions.md` — 19 spec ambiguities found before
  code, 4 real bugs found by review after, including one *"spec bug faithfully
  implemented — the review caught what the spec review didn't"*
- `cmd/evie/docs/done/web-search.decisions.md` — same loop, same day
- `docs/decisions.md` 2026-07-28 — "reads wide, writes gated," the standing
  preference for general capability plus an explicit guard; the `Role` allowlist
  above is that principle applied to sub-agents
