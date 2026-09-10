Use $implement from /Users/davidboktor/.agents/skills/implement/SKILL.md to implement Memory Stage 5 in https://github.com/davidadel66/evie.

Read parent specification #154 and all child issues #156–#168, including comments and native blocking relationships. Also read AGENTS.md, CONTEXT.md, the Stage 5 retrieval spec/decisions, and applicable memory ADRs. Some approved design documents may still be uncommitted in /Users/davidboktor/code/evie; preserve and carry only those relevant documents into the implementation worktree.

Deliver ONE PR containing exactly ONE completed commit per child issue: 13 issue commits in the final history. Include the owning issue number in every commit subject. Keep implementation, tests, and relevant documentation for an issue together. Fold review fixes into the owning issue's commit before pushing the final branch; do not create unrelated cleanup commits.

Use a dedicated codex/ branch and isolated worktree, reusing an appropriate task worktree if already assigned. Preserve all unrelated working-tree changes. Work through the issue dependency graph. A blocker is satisfied when its implementation is committed and verified on this branch; do not wait for GitHub issue closure or merge between tickets. Keep the parent and child issues open during implementation.

Follow the implement skill: use $tdd at the agreed seam—complete agent turns with real SQLite and a scripted provider, plus focused HTTP/UI tests. Run focused checks as each issue develops. Use $code-review, resolve all in-scope findings, and run ./scripts/verify-change.sh before handoff. Verify meaningful acceptance behavior rather than merely matching implementation details.

Complete the real local retrieval experiments and evaluations required by #165, #167, and #168. Freeze settings before the held-out assessment. Never invent measurements, weaken gates to obtain a pass, or mark a blocked experiment complete. If required hardware, credentials, dependency approval, or a behavioral contract is missing, identify the concrete blocker and continue independent authorized work.

Continue across the full issue set. Once all acceptance criteria and required checks pass, push the branch and open ONE PR. Include a commit-to-issue mapping, exact verification results, remaining limitations, and closing references for the 13 child issues. Do not close the parent specification or merge the PR.
