# Memory Stage 5 final code review

The requested `$code-review` ran Standards and Spec in parallel against the
user-confirmed fixed point `f27546d5872c3c175dc45fca0ce814cbb5eb5afb`.
The reviewed head was `f1714138d11318ec9eeab2f66fffa2483b2a4891`, containing
exactly one commit per child issue #156–#168. The captured comparison was
`git diff f27546d...HEAD`; the base resolved and the diff was nonempty.
These are agent reviews, not human approval or release authorization.

A preliminary review found a #162 cross-scope conflict-refresh defect. Its
regression and fix were folded into that issue's commit before the passing
v4 development run and unchanged held-out evaluation. The final review found
no additional fix to fold. This report and its handoff link were subsequently
amended into #168; no evaluated implementation, test, corpus or scoring input
changed as a result of the final review.

## Standards

No actionable documented-standard violations or material baseline smells found across the 13 commits from `f27546d` to `f171413`.

The changes follow `AGENTS.md`’s consumer-owned interfaces and deterministic safety boundaries: retrieval authority remains in the Kernel; scope, source eligibility, retirement, egress, and cumulative budgets are enforced in code. Index maintenance uses transactional progress and revalidates computed documents before persistence. Original request references remain durable, and UI inspection checks the selected session and current source access.

The approved one-PR/13-commit breakdown overrides the usual change-splitting guidance. Historical preset compatibility and source-preservation copies are intentional, so they are not duplication findings. The documented failed release gates are not a Standards violation or a readiness claim.

Read-only review; no additional tests or model calls. The reported passing full verification remains the deterministic check, separate from this review.

Standards: **0 findings; worst severity: none.**

## Spec

No additional actionable Spec findings in `f27546d5872c3c175dc45fca0ce814cbb5eb5afb...f1714138d11318ec9eeab2f66fffa2483b2a4891`. Reviewed all thirteen child issues (#156–#168), parent #154, the binding memory decisions, and applicable scope, lifecycle, provenance and evaluation contracts. No additional missing requirement, scope expansion, or incorrectly implemented contract was substantiated.

Release readiness remains **false**: the retained held-out result has 23 failed and three incomplete gates. This is consistent with #168's explicit instruction to “publish the precise failures and follow-up work without changing thresholds to manufacture a pass.” The report preserves the rejected assessments, preparation failure, protocol deviation and historical reproduction gaps; this review does not convert them into passing acceptance evidence.

Independent checks passed:

- `git diff --check f27546d5872c3c175dc45fca0ce814cbb5eb5afb...HEAD` — exit 0.
- `go test ./internal/agent ./internal/web -run '^(TestMemoryInvestigation(RefreshesRestoredConflictsAcrossAuthorizedScopes|IgnoresRestoredConflictsOutsideAuthorizedScopes)|TestMemoryReceipt|TestHistorical(Memory|Conversation)Search|TestConversationExpansionTurn(RejectsForgedAndOutOfScopeAnchors|SubtractsOverlappingUTF8EvidenceAndDuplicateRequests|ExcludesCurrentRootFromEarlierCurrentSessionAnchor|OmitsRetiredNeighborRangeButKeepsUnrelatedUTF8Passage|OptOutWithholdsSourceReferencesFromCompleteRequest))' -count=1` — exit 0; agent 3.331 s, web 1.124 s.

The full repository verification and model-backed results were inspected as retained evidence rather than rerun in this read-only review. Model review does not replace deterministic checks.

Spec: **0 additional findings; worst severity: none.**

Standards: 0 findings, worst none. Spec: 0 additional findings, worst none.
