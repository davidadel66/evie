# Skills decisions

Spec: [skills.spec.md](skills.spec.md).

- **2026-11-02 — Skills follow artifacts and are a separate feature.** David
  accepted building artifacts first ("go with your recommendations"). See
  [artifacts.spec.md](artifacts.spec.md).

- **2026-11-02 — Context Scope decides which skills a chat sees; the Agent
  Preset decides only whether the `skill` tool exists. Global scope first,
  Workspaces later.** David accepted the recommendation. This amends
  [plugins.spec.md](plugins.spec.md), where an Agent Preset carried "reviewed
  skill catalog entries", and settles the tension with
  [memory.spec.md](memory.spec.md), which puts skills under Workspace and
  project folders. Global skills are visible in Workspace chats and chats with
  no workspace alike, as global memory and Global workflows are.

- **2026-11-02 — Each skill is approved individually, at an exact version, and
  a chat sees only skills approved before it started.** David accepted the
  recommendation. No location is trusted wholesale, including
  `~/.agents/skills`. New approvals and deactivations reach new chats only,
  which keeps ADR 0006's pinning. Approving every skill of one import report in
  a single command is a convenience over the same per-skill records: each is
  activated at the hash the report showed, and a changed proposal is refused.

- **2026-11-02 — `/name` owner invocation is in the first slice.** David
  accepted the recommendation. Published evaluations show models often fail to
  invoke an available skill, and Evie usually runs DeepSeek. Owner invocation
  is the dependable path. Skills marked `disable-model-invocation` are
  owner-only.

- **2026-11-02 — No scripts in skills for now.** David accepted the
  recommendation after asking what scripts in skills are. Skills that bundle a
  `scripts/` folder or any executable are refused at import. A later stage may
  add a sandboxed runner that executes only an approved skill's scripts, with
  approval. Running them through the chat's existing shell was rejected,
  because a script would then run with the shell's full access.

- **2026-11-02 — Skill text reaches the model as framed user-role content, not
  a system-role block.** David accepted the recommendation. A model-loaded skill
  arrives as a framed tool result, and an owner-invoked skill as a framed
  user-role message before the owner's message, matching how repository
  instructions are sent today. This amends
  [memory.spec.md](memory.spec.md)'s "trusted supplemental system-role block"
  for skills.

- **2026-11-02 — David's Claude Code skills migrate through the same approval
  path.** David approved the spec's test seams and added: "we should have a
  migration of the skills we have in claude code into evie".
  - `evie skills import-claude` reads `~/.claude/skills`, the synced skills
    under it, and `~/.agents/skills`, plus installed plugin skills when asked.
  - It proposes each eligible skill once, by Claude Code's precedence, and
    reports what was refused and shadowed. Nothing activates on import.
  - Skills from third parties (for example the synced Anthropic skills and the
    Matt Pocock skills in `~/.agents/skills`) are allowed through the same
    per-skill review. The source and any license are recorded with the version.
  - Inventory on 2026-11-02:
    - **Personal:** 7 skills, all without scripts.
    - **`~/.agents/skills`:** 39 skills; 2 carry scripts (`diagnosing-bugs`,
      `git-guardrails-claude-code`) and will be refused.
    - **`~/.claude/skills/synced`:** 9 Anthropic skills, most of which carry
      scripts.

- **2026-11-02 — The catalog is a framed message, not the tool description;
  per-chat skill references are added to the Composition Receipt.** A design
  correction found while mapping the code, under David's "go with your
  recommendations".
  - Tool schemas are hashed and checked on resume, so a per-chat catalog in
    the `skill` tool's description would fail resume. The tool's description
    stays fixed instead, and the catalog travels as a framed user-role
    message.
  - Resume currently requires a receipt's instructions to equal the preset's.
    It is extended to accept `skill.` references when the `skill` capability
    is pinned, resolving each to its approved version.

- **2026-11-02 — The catalog has an 8 KiB budget.** Chosen under David's "go
  with your recommendations". With dozens of migrated skills, an unbounded
  catalog would sit in every request. Names are always listed, and
  descriptions are shortened evenly to fit. Two-way door.

- **2026-11-02 — Test seams approved.** David approved the three seams in the
  spec's Testing Decisions: the CLI approval harness, one composed chat test in
  `cmd/evie`, and the UI composer tests.
