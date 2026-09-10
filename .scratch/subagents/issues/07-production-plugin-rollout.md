## Parent

[Subagents: Preset-composed foreground research with Kernel supervision (#155)](https://github.com/davidadel66/evie/issues/155)

## What to build

Make the fully verified foreground capability available through normal first-party plugin composition. Register the compiled Subagents Plugin, select its optional capability in the new standard Agent Preset, and wire the tested supervisor into both CLI and web runtime lifecycles.

This is the first production exposure. Preserve existing conversation presentation and pinned sessions. Workspace success remains a separate follow-up on reviewed Workspace preset allowances tracked by issue #71; this release demonstrates eligible Global/project sessions and explicit Workspace refusal.

## Acceptance criteria

- [ ] The compiled Subagents Plugin participates in existing manifest/version, dependency, enablement, startup, health, and shutdown management with a canonical delegation Capability Contract.
- [ ] New eligible standard-preset sessions select delegation when the Plugin is enabled and available. Disabled/unavailable optional contribution produces the normal diagnostic and omission without blocking unrelated primary-session behavior.
- [ ] Existing primary receipts retain exactly their original composition. New research children pin the restricted preset, worker instructions, execution policy, and authorized scope without silently modifying Workspace revisions.
- [ ] Both CLI and web use the real supervisor and existing parent invocation/result presentation. A bounded Global/project research request returns sourced findings and a normal parent answer, with no child reasoning or streaming presented as the parent.
- [ ] Already-created parents holding a delegation capability must pass current Plugin/supervisor admission checks. Disabling or stopping Subagents immediately blocks new work and invokes the tested cancellation lifecycle for active assignments.
- [ ] Runtime startup reconciles abandoned executions through the tested recovery contract before allowing fresh admission; it preserves another live process's work. Runtime shutdown closes admission and performs bounded cleanup.
- [ ] Required research capability failures and relevant current access revocations become explicit execution outcomes with no fallback to a broader composition. Kernel outcome inspection and recovery remain available when Subagents is stopped.
- [ ] Workspace admission reports the unresolved reviewed-preset-allowance prerequisite and never changes scope or bypasses the allowlist. Global/project rollout has no blocking dependency on completing all of issue #71.
- [ ] Manual demonstrations cover newly composed eligible CLI/web sessions, one foreground research result, parent cancellation, disable from an already-pinned parent, restart with retained findings, an old receipt without delegation, and explicit Workspace refusal.
- [ ] All prerequisite contracts are implemented before production registration becomes usable. Run focused integrated conversation/lifecycle/recovery checks, applicable race checks, and the full repository verification; no standalone testing cleanup or new worker dashboard is introduced.

## Blocked by

- [#173: End child execution when parent authority ends](https://github.com/davidadel66/evie/issues/173)
- [#174: Recover interrupted assignments and replay retained findings](https://github.com/davidadel66/evie/issues/174)
