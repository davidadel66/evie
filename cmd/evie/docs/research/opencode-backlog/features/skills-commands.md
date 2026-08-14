# F18 - Skills and prompt commands

Status: deferred candidate, unapproved.

## Purpose

Package repeatable local workflows and domain guidance without inflating the
stable prompt or turning every workflow into Go code.

## How OpenCode does it

OpenCode discovers skills, advertises permission-filtered descriptions, and
loads a selected skill body plus nearby files on demand. Its command catalog
combines built-ins, configured Markdown templates, MCP prompts, and skills, with
argument placeholders. Custom agents can also be described through config.

Sources:

- [`skill/index.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/skill/index.ts)
- [`tool/skill.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/tool/skill.ts)
- [`command/index.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/command/index.ts)

## Proposed EVIE adaptation

Keep two concepts distinct:

- **Command:** user-invoked prompt template such as `/grill <plan>`.
- **Skill:** trusted instructions/resources loaded on demand and optionally made
  available to profiles.

Start local only:

```text
~/.evie/commands/*.md
~/.evie/skills/<name>/SKILL.md
<trusted-project>/.evie/commands|skills
```

Each entry declares name, description, allowed profiles, and any required
capabilities. Loading a skill never grants a tool permission; profile/session
policy remains authoritative. Project skills are instructions only after the
project is trusted. No remote marketplace, executable hooks, or self-editing
skills in the first version.

## Candidate first workflows

- `/grill`: adversarially review a plan/spec.
- `/verify`: run and report the project verification contract.
- `/review`: launch the read-only reviewer profile.
- `/wrap`: produce the project's approved close-out artifact.

## Acceptance evidence

- Loading a skill does not widen tool permissions.
- Skill source and content hash appear in the context snapshot.
- Invalid metadata fails clearly.
- Commands expand arguments without shell interpolation.
- Untrusted remote/project text cannot silently install a skill.

## Open decisions

1. Should skills be globally trusted by filesystem location or individually
   approved?
2. Can a skill invoke a fixed workflow directly, or only add instructions?
