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

- **2026-11-03 — Stored formats.** David accepted the recommendation ("yes"):
  four immutable tables, `skill_imports`, `skill_proposals`,
  `skill_activations` and `skill_deactivations`, with no-update and
  no-delete triggers and a surface fixed to `cli`. Skill files live under
  `global/skills/<name>/` in the procedural repository. A chat's receipt pins
  `skill.global.<name>` with the version hash as an instruction reference.

- **2026-11-03 — An owner invocation is recorded on the turn's root message.**
  David accepted the recommendation ("yes"): `/name` stores
  `skill: {name, version}` in the root `user_message` payload, written
  atomically with the turn. A side table was rejected because it would be
  written after the event and could miss it on a crash.

- **2026-11-03 — Choosing a skill in the `/` menu fills the draft.** David
  accepted the recommendation ("yes"): picking a skill puts `/name ` in the
  draft so he can type the request, and Enter sends it. The built-in commands
  `/model` and `/compact` keep acting at once.

- **2026-11-03 — The composer UI waits for a canvas verdict.** David accepted
  the recommendation ("yes"): the `/` menu's skill list is shown on the design
  canvas, today's screenshot beside the proposal, before that UI is built. The
  server side of owner invocation and the other tickets don't wait.

- **2026-11-03 — Built as four tickets on `feat/skills`.** David approved the
  split ("yes"): propose, approve and deactivate from a folder; migrate Claude
  Code skills (blocked by the first); Evie loads approved skills in new chats
  (blocked by the first); invoke a skill with `/name` (blocked by the third).

- **2026-11-03 — Build choices (two-way doors).** Decided under the flow brief
  David approved ("yes"); each is cheap to reverse.
  - A version hash is the bare 64-character hex SHA-256 of a canonical
    manifest with a format tag, the form receipt references require.
  - `approve <name> --hash` resolves to the newest pending proposal with that
    name and hash. `--expect-version` is the integer active version, 0 when
    none, as for workflows. `deactivate <name>` deactivates the active
    version.
  - A new folder walker checks a skill with `Lstat` before anything is
    committed, because the procedural `Commit` drops the executable bit.
    Only a top-level skill folder that is a symlink is followed; symlinks
    inside a skill are refused.
  - The no-model-tool test asserts that the skills plugin exposes exactly
    `skill` with `name` and `path`, and that no tool proposes, approves,
    activates or deactivates.
  - Skill references are added in `cmd/evie` just before the chat's creation
    transaction: the write is in the transaction, the activation read just
    before it. A chat whose skills plugin is disabled gets no references, and
    a pre-receipt chat gets none when first bound.
  - Resume accepts the preset's instructions followed by `skill.` references
    only when the `skill` capability is pinned. `cmd/evie` checks each one
    against the activation records and fails visibly when one is missing.
  - The `skill` tool reads the receipt by session ID, then the commit and
    manifest from the activation record.
  - The catalog is frozen for the chat's life and placed right after the
    repository instructions. The system prompt's guidance line appears only
    with the tool.
  - On the active turn the order is memory evidence, skill body, owner
    message. The body is inserted while the request is composed, so the
    compaction transcript never sees it, and a turn removed by compaction or
    trimming takes its body with it.
  - A bare `/name` is stored as the message text, as the spec says.

- **2026-11-03 — Canvas verdict on the composer, and the skill label.** David
  approved the canvas mock ("yes that looks good"): typing `/` lists the
  commands, then the chat's skills with one-line descriptions, an argument
  hint in place of the description when a skill has one, and "you only" on
  owner-only skills; choosing a skill fills the draft with `/name`. He also
  accepted a label the mock added ("i like the label"): an owner message sent
  with a skill shows `/name` above its text, read from the turn's recorded
  `skill` field.

- **2026-11-03 — Plugin skills are read from the plugin's own list too.**
  David approved the fix ("yes") after the migration missed
  `mattpocock-skills`, which lists its skills in `.claude-plugin/plugin.json`
  under `skills/<group>/<name>`. With `--plugins`, the migration reads both
  `skills/<name>/` and the folders the plugin's `skills` field lists, once
  each. A listed path that leaves the plugin's folder, or holds no
  `SKILL.md`, is reported in the notes and not read. Checked against the
  real installed plugins with a throwaway home: 61 proposed, 4 refused, no
  notes.
