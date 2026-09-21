# When multi-agent delegation earns its overhead

**Research date:** 2026-09-11
**Status:** Primary-source evidence and analysis only. This note does not
authorize implementation or amend Evie's specifications, capability boundaries,
or execution policy. No Evie capacity or cost measurement was performed.

## Finding

Delegation is most promising when substantial work can be assigned independently
and returns verifiable findings or artifacts. Independent review can also help
after implementation, without running concurrently. Latency, coverage, and
total expenditure must be assessed separately. More agents alone establishes
none of these benefits. The evidence below concerns specific systems and
experiments; the final decision test is an inference for evaluating Evie.

## First-party production experience

Anthropic's June 2025 Research system gives workers separate contexts and
synthesizes condensed findings. Independent research suited this design;
shared context and tight dependencies were poorer fits. Explicit objectives,
boundaries, sources, and output formats reduced duplicated searches and gaps.
([Research system engineering](https://www.anthropic.com/engineering/multi-agent-research-system))

It reported up to 90% less time for complex queries after introducing both
parallel workers and parallel tool calls, so worker effects are not isolated.
Multi-agent usage was approximately 15 times ordinary chat tokens; agents used
roughly four times chat tokens. These workload categories do not establish a
controlled multiplier for replacing one agent with a team. Synchronous batches
waited for their slowest member. Artifact references reduced copying and
information loss.
([Research system engineering](https://www.anthropic.com/engineering/multi-agent-research-system))

## Controlled scaling evidence

Kim and colleagues' **version 3**, revised April 8, 2026, evaluates 260
configurations across six benchmarks, five architectures, and three model
families, describing matched budgets and tools. Centralized Finance-Agent
performance improved 80.8% relative to its single-agent baseline; PlanCraft
multi-agent variants declined 39.1–70.0%. These are benchmark-specific success
differences, not latency improvements or Evie forecasts. Older summaries cover
version 1's 180 configurations and four benchmarks.
([Paper, v3](https://arxiv.org/html/2512.08296v3),
[version history](https://arxiv.org/abs/2512.08296))

Traces contrast independent financial research with planning dependent on
preceding state: unnecessary decomposition consumed communication and reasoning
resources. Limitations include canonical architectures, preliminary team-size
exploration up to nine, and limited heterogeneity. Thresholds are not universal.
Cost tracking covered 180 configurations; the additional benchmark families
used different execution/cost structures.
([Results and limitations, v3](https://arxiv.org/html/2512.08296v3))

## Shared-state work needs a different argument

Anthropic's August 2026 experiments distinguish bounded agent invocations from
long-lived peers that must coordinate changing state. Vulnerability discovery
could partition exploration without one agent's missed finding invalidating
another's work. Larger implementation projects introduced changing
interdependencies. In its vulnerability experiment, a coordinated swarm found
more issues while using more tokens and searching additional directories;
restricting comparison to the same core directories made tokens per finding
appear comparable. That is evidence for distinguishing search coverage from
efficiency, not proof that a coordinated swarm is universally cheaper or better.
([Patterns and problems in emerging multiagent systems](https://www.anthropic.com/research/multiagent-systems))

## Delegation and parallelism are separate choices

Anthropic's architecture guidance separates parallel sectioning, repeated
attempts for additional confidence, sequential prompt chains, and dynamic
orchestrator/worker decomposition. Parallel sectioning requires separable work;
sequential chains explicitly consume preceding outputs. An orchestrator can
discover subtasks dynamically without every subtask being safe or useful to
run concurrently. The guidance recommends adding complexity when evaluation
shows a benefit. Its page identifies the original December 2024 publication
as foundational guidance and links newer material for current product design.
([Building effective agents](https://www.anthropic.com/engineering/building-effective-agents))

## Model guidance and product behavior

Assuming the user's reference to Fable means **Claude Fable 5**, Anthropic says
it delegates more readily than prior models. Its guidance favors independent
assignments, asynchronous communication while the parent continues useful
work, and retained worker contexts that can benefit from cache reads. These
patterns require corresponding harness support; the model alone does not
create a persistent worker lifecycle.
([Fable 5: parallel subagents](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/prompting-claude-fable-5#parallel-subagents))

OpenAI's GPT-6 Astra guidance says delegation may be less frequent than a
workflow needs. It recommends explicitly stating when and how much to
delegate, based on possible time or quality gains, and tuning instructions to
the harness. This is model guidance, not a description of every Codex client's
trigger policy.
([Astra: subagent delegation](https://developers.openai.com/api/docs/guides/latest-model#subagent-delegation))

Current local Codex documentation describes explicit user requests or applicable
project/skill instructions as delegation triggers. Suggested starting points
include exploration, tests, triage, and summarization; concurrent edits add
conflict and coordination costs. Workers return distilled results and inherit
the parent's model and reasoning settings unless configured otherwise.
Its examples separate read-only exploration, review, and documentation research;
another workflow gives a UI fixer the targeted implementation after browser
reproduction and code mapping. The same page separately describes proactive
delegation for ChatGPT Work's Ultra intelligence level, which should not be
conflated with local Codex.
([Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents))

**Current Evie boundary:** the shipped design uses bounded web research children
with the parent's resolved model/provider configuration. Parallel children run
as a foreground batch whose caller waits for results. Nested delegation and
long-lived reusable workers are outside the current feature. Async parent work
and retained workers from the Fable guidance would therefore require separate
product and runtime changes.
([Evie specification](../active/subagents.spec.md),
[supervisor](../../../../internal/subagents/supervisor.go))

## Coding and independent review in Claude Code

Claude Code's general-purpose subagents can modify code. Custom workers can
restrict tools and configure permission modes, subject to parent and managed
policy. Its examples include read-only reviewers, test runners returning
failure summaries, and reviewer-to-optimizer sequences. Ordinary subagents
start fresh; conversation forks inherit the parent's history. Consequently,
an additional agent does not necessarily start with independent context.
The documentation favors self-contained assignments and keeps quick changes
or phases with substantial shared context in the main conversation.
([Claude Code subagents](https://code.claude.com/docs/en/sub-agents))

The separate, experimental agent-team feature illustrates simultaneous
security, performance, and test-coverage reviews of one PR, followed by lead
synthesis. It recommends clear deliverables such as a function, test file,
or review, and distinct file ownership to prevent competing edits. The lead
monitors and redirects work. These examples support both implementation and
review delegation; they do not establish that either needs a large team.
([Claude Code agent teams](https://code.claude.com/docs/en/agent-teams))

Claude Code can give writing subagents separate Git worktrees through
`isolation: worktree`. Its example delegates a refactor, tests, and a result
report. Worktrees separate files and branches but share Git metadata and
saved permission approvals. The documented default base is the repository's
default branch; `worktree.baseRef: head` selects the current committed state.
Choosing the correct base matters when a worker must inspect an in-progress
change.
([Claude Code worktrees](https://code.claude.com/docs/en/worktrees))

**Inference for a future Evie policy:** use one implementation owner plus an
independent reviewer as a starting arrangement. Give the reviewer requirements
and the actual change to check; a fresh context alone does not guarantee
independent judgment. Split writes when ownership or isolation makes the
boundaries clear. The parent should assess findings, integrate changes, and
verify the combined result. Review may follow implementation sequentially.
Evie's current web-only child cannot perform these repository workflows;
coding or review presets would need explicit tools, permissions, workspace
handling, and verification contracts. This note does not authorize that change.

## Decision test for an Evie experiment

**Inference, not an approved product policy:** before delegating, identify the
bounded result that another worker can produce without waiting for a sibling
or continually sharing mutable state. Estimate whether producing that result
is materially more work than specifying, checking, and incorporating it.

| Candidate work | Expected reason to try or avoid parallel delegation |
| --- | --- |
| Compare several providers' official documentation | Separate source sets and a common comparison format |
| Investigate independent explanations of a broad question | Distinct evidence streams; orchestrator must reconcile contradictions |
| Answer one readily searchable fact | Setup and synthesis can exceed the useful work |
| Follow a failure whose next step depends on the latest result | The dependency chain limits useful overlap |
| Concurrently update the same Task or shared artifact | Coordination and reconciliation can dominate; independent research does not require shared mutation |

For independent workers with sufficient capacity, a rough latency model is
`planning + slowest worker + synthesis`, compared with the sum of serial worker
times. Queueing, provider contention, retries, and unequal work sizes weaken
that ideal. Total tokens still include every worker's context and output plus
orchestration; keeping the primary context small does not establish lower
aggregate cost. These are accounting observations, not measured speedups.

Evaluate the same representative assignments with one agent and bounded
parallel settings. Record answer correctness and source coverage, elapsed
time, total model/tool usage, duplicate work, and synthesis/rework. Compare
cost per successful useful answer; report unknown usage as unknown. Retain
separate results for independent research and sequential/shared-state tasks
instead of hiding their differences in one average.
