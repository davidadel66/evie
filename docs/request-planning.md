# Request planning workflow

Status: manual v1

## Purpose

Turn a product or engineering request into an approved sequence of small,
independently reviewable changes before implementation begins.

This workflow produces a delivery plan, not product behavior. Specifications
define what Evie should do; decision records explain binding choices; the plan
only divides that approved behavior into executable work.

## Classification

Classify the request by the work it actually requires:

- **Story:** one independently verifiable pull request.
- **Epic:** one coherent outcome that requires multiple stories.
- **Initiative:** a broad program containing multiple epics.
- **Research spike:** a bounded investigation needed before behavior can be
  specified or implementation can be estimated responsibly.

Do not force a small request into an epic. Do not disguise a broad program as
one story.

## Manual process

1. Capture the requested outcome and why it matters.
2. State explicit non-goals and constraints.
3. Find the applicable specifications, decisions, code paths, and prior work.
4. Identify unresolved questions. Create a research spike when evidence is
   required; otherwise surface product decisions for David.
5. Decide whether the request is a story, epic, or initiative.
6. Break large work into the smallest coherent stories that can be verified and
   reviewed independently.
7. Order stories by dependency and risk. Prefer proving uncertain foundations
   before building consumers on top of them.
8. Review the proposed breakdown with David and revise it.
9. Freeze the approved initiative and epic breakdown.
10. Select exactly one dependency-ready story for implementation.
11. Expand that selected story into its full execution contract and check it
    against the definition of ready.
12. Create or reuse its GitHub issue, then hand the issue to the implementation
    workflow.

Planning approval does not itself authorize code changes. Implementation begins
only when David selects a story.

## Story rules

Every story should:

- deliver one coherent outcome rather than a collection of files or layers;
- have explicit in-scope and out-of-scope boundaries;
- trace to an approved request or specification section;
- state observable acceptance criteria;
- name deterministic verification and any manual demonstration;
- identify dependencies, risks, and unresolved decisions;
- fit in one reviewable pull request;
- avoid implementing later stories opportunistically.

If a story cannot be verified independently, either split it differently or
explain why it is a necessary foundation and how that foundation will be
demonstrated.

## Definition of ready

A story is ready for implementation when:

- its outcome and non-goals are clear;
- no unresolved question would materially change its behavior, security,
  persistence model, or public interface;
- its acceptance criteria are observable;
- its verification commands or checks are known;
- its dependencies are complete or explicitly selected with it;
- its proposed pull-request boundary is reviewable;
- David has selected it as the next story.

## Planning artifacts

Use the smallest structure that fits the request:

- A one-story request normally needs only its GitHub issue.
- An ordinary multi-story epic may use one adjacent `<feature>.plan.md` file.
- A multi-epic initiative gets a feature directory with one file per epic.

Use this layout for a large initiative:

```text
cmd/<app>/docs/active/<feature>/
  README.md
  spec.md
  decisions.md
  epics/
    <epic-id>-<epic-name>.md
```

The files have distinct responsibilities:

- `spec.md` defines initiative-wide behavior, architecture, and invariants.
- `decisions.md` records binding initiative-wide choices.
- `README.md` indexes epics, dependencies, recommended order, approval state,
  and links. It does not duplicate their contents.
- Each epic file defines one coherent outcome, references the applicable spec
  sections, lists its stories, and states epic-level completion evidence.

Keep proposed stories as concise summaries in their epic file. Once David
selects a ready story for implementation, create a GitHub issue containing its
full execution contract. That issue owns the story's active discussion, status,
acceptance criteria, and pull-request link.

Create execution-contract issues one selected story at a time. Do not freeze
future story details or fill GitHub with issues for work whose dependencies,
implementation seams, or decisions may change before selection.

Do not create a nested Markdown file for every story by default. Create a
separate repository document only when a story needs a durable specification or
decision record that should outlive its GitHub issue.

## Initiative index template

```md
# <initiative>

Status: proposed

## Outcome

## Non-goals

## Sources

## Epics

| ID | Epic | Depends on | Status |
|---|---|---|---|

## Recommended order

## Open decisions and research spikes

## Approval record
```

## Epic template

```md
# <epic ID> - <epic title>

Status: proposed

## Outcome

## Specification references

## In scope

## Out of scope

## Stories

### <story ID> - <story title>

- Outcome:
- Depends on:
- Acceptance summary:
- Verification summary:
- Proposed PR boundary:

## Epic completion evidence

## Risks and open decisions

## Approval record
```

## GitHub story contract

An approved story issue contains:

- outcome and user or system value;
- epic and specification links;
- dependencies;
- in-scope and out-of-scope boundaries;
- observable acceptance criteria;
- deterministic verification;
- manual demonstration when applicable;
- risks, approved decisions, and known exclusions;
- the intended one-PR boundary.

Use this issue shape unless the repository defines a stricter template:

```md
# <story ID>: <story title>

## Outcome and value

## Sources

## Dependencies

## In scope

## Out of scope

## Acceptance criteria

## Verification

## Manual demonstration

## Risks and approved decisions

## One-PR boundary
```

Before creation, search open and closed issues for the story ID and outcome. If
an existing issue already owns the story, return it instead of creating a
duplicate. If the contract fails the definition of ready, report the missing
decision or evidence and do not create the issue.

Creating the issue authorizes planning state only. It does not authorize a
branch, worktree, product-code change, commit, push, pull request, or merge.
