## Parent

[Subagents: Preset-composed foreground research with Kernel supervision (#155)](https://github.com/davidadel66/evie/issues/155)

## What to build

Use the existing delegated-session shape to run a scripted child whose context contains only the allowed trusted instructions, explicit assignment data, and its own evolving transcript. Preserve the parent's role as the agent responsible for the owner's final answer.

This outcome is independently testable using existing child-session fixtures and supplied capability compositions. It does not depend on introducing the research preset, automatic retrieval features, or production delegation.

## Acceptance criteria

- [ ] Trusted runtime configuration supplies a pinned worker role that preserves foundational safety rules and makes the parent responsible for the owner's final answer. Ordinary primary-session instructions remain unchanged.
- [ ] The child receives the bounded assignment and explicitly selected supporting context as attributed data. It does not automatically fork the parent transcript or include sibling/unrelated session content.
- [ ] The child can continue its normal model/capability loop using its own durable history, without treating the parent assignment as direct owner authority.
- [ ] Automatic Recall, prior-conversation retrieval, and Global/Workspace/project memory injection are disabled by the delegated context policy independently of whether Memory capabilities are present.
- [ ] No Task Focus or automatic Task projection is loaded for the initial child policy. Explicit parent-selected Task facts remain bounded assignment data.
- [ ] Required trusted scoped instructions are still loaded under their existing contracts and fail visibly when required content is unavailable. Scope inheritance cannot widen resource access.
- [ ] Captured child requests demonstrate exclusions with sentinel parent history, sibling history, memory, and Task data. Explicitly supplied context and the child's own accepted history remain available.
- [ ] The instruction and context policy is stable for the child session and represented in its pinned reproducibility evidence. No mid-session model-selected policy changes are introduced.
- [ ] Run focused context/conversation/receipt checks and full repository verification. Use existing delegated-session and context-composition tests as prior art; do not add a second agent loop.

## Blocked by

None (can start immediately).
