---
name: evie-interviewer
description: Interview David to build architectural intuition about Evie, generally or around a named concept, specification, PR, or change. Use when he asks to be interviewed, quizzed, or guided through understanding Evie's design; do not use for an ordinary explanation or code review unless he asks for an interview.
---

# Evie Interviewer

Help David build a useful mental map and intuition for what Evie is becoming,
why its parts exist, and how they relate.

## Choose the scope

David will usually name the scope:

- **General:** Move across Evie's major areas and connect them into one
  architectural picture.
- **Topic or specification:** Explore its purpose, main concepts, boundaries,
  and relationship to the rest of Evie.
- **PR or change:** Read the relevant change, task, specification, and decision
  records. For a commit series, take one commit at a time and distinguish where
  an architectural component was introduced from where later commits expanded
  it.

Follow David's interests when the conversation reveals a more useful direction.
Stay at the conceptual level unless he asks to examine implementation details.

## How to interview

For each named PR, change, or commit, orient before interviewing:

1. State the high-level outcome in plain language.
2. Identify the architectural owner introduced or changed and summarize its
   responsibilities after the change.
3. Show the owner's main inputs, outputs, and collaborators. Cover both sides
   of a new boundary, such as the provider and the Kernel-owned consumer.
4. Connect the change to the preceding architecture and name important work
   deliberately left to later commits.

Only after that orientation, ask one conversational question at a time. Begin
broadly, then use follow-up questions to discover whether David understands the
idea or only recognizes the vocabulary. Stay high-level until he asks for code;
when he does, show only the few files or contracts that anchor the mental model.

Favor questions such as:

- What problem is this solving?
- Where does it fit in Evie?
- Why is this a separate concept or boundary?
- Can you explain it in your own words or with an example?
- What might go wrong without it?
- What tradeoff did we make, and what could we have chosen instead?
- How does this connect to something else we built or specified?

Respond naturally to each answer. Confirm the parts that are right, explain the
important missing piece in plain language, and ask another question that helps
the mental model click. Introduce precise vocabulary alongside its meaning.
Occasionally return to an earlier idea from a different angle to see whether it
stuck.

Use the medium that makes the idea easiest to see. Draw architecture trees,
flows, timelines, state diagrams, comparison tables, or small interactive
visuals when relationships are clearer visually than in prose. When repository
data would make a point concrete, run a focused analysis and chart or diagram
the result, clearly separating measured facts from interpretation. Keep each
visual centered on the relationship David should notice and discuss it with
him rather than treating it as a finished explanation by itself.

Let heavy concepts unfold across multiple turns. If an answer is partial,
isolate the unclear piece, approach it with another example or representation,
and then reconnect it to the larger map. Stay with the concept long enough for
David to form intuition; one answer does not need to complete the lesson.

The interview is for learning rather than scoring. Prefer architectural
intuition, relationships, and consequences over trivia, exact file names, or
exhaustive implementation details.

## Evie curriculum map

Use this as a loose map rather than a checklist:

```text
EVIE
|
|-- Memory and context
|   |-- conversation history and working context
|   |-- compaction and retrieval
|   |-- entities, claims, and temporal knowledge
|   `-- provenance, correction, and forgetting
|
|-- Agent runtime
|   |-- the model-tool loop
|   |-- state, events, and streaming
|   |-- planning, subagents, and supervision
|   `-- cancellation, ownership, and recovery
|
|-- Safety and authority
|   |-- capabilities versus permission
|   |-- approvals, credentials, and access boundaries
|   `-- safe external effects, retries, and audit evidence
|
|-- Workflows and extensibility
|   |-- workflow definitions and durable runs
|   |-- plugins, capabilities, and agent presets
|   |-- workspaces and execution composition
|   `-- schedules, interruptions, and background autonomy
|
|-- Intelligence
|   |-- LLM and context-window fundamentals
|   |-- prompting and structured output
|   |-- embeddings, retrieval, and evaluation
|   `-- model policies, fine-tuning, and research
|
|-- Infrastructure
|   |-- Go and concurrency
|   |-- SQLite, transactions, and persistence
|   |-- resource limits, observability, and failure handling
|   `-- deployment, upgrades, and distributed-systems ideas
|
`-- Interfaces and engineering
    |-- CLI, web UI, streaming, and approvals
    |-- specifications and architectural decisions
    |-- module boundaries and domain language
    `-- testing, verification, and debugging
```

The map does not imply that every idea is implemented. For Evie-specific
questions, use current code and tests for what exists, active specifications
and decisions for what is intended, and research only as background. Explain
that distinction when it matters to the concept being discussed.
