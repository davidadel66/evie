# Ticket 146 advanced web review handoff

The complete adapter/UI implementation is frozen in `146-frozen-files`; the authoritative 18-file manifest is `146-engineering-checkpoint.json`. Root owns the final predecessor 144 tree, isolated full verification, independent Standards/Spec review, browser demonstration and one commit. No files were staged or committed by this owner; no production dependencies or root hooks were added.

## Review entry points

- `internal/web/candidate_review_advanced.go`: eleven guarded typed routes. The owner context is minted from the exact selected scope, never deserialized. The optional advanced interface preserves simple adapters; the real Store implements it.
- `internal/web/candidate_review.go` and `candidate_review_json.go`: strict one-object UTF-8/duplicate/nesting/unknown-field handling after explicit route-specific byte limits; typed safe error classification. Simple calls remain 8 KiB except single resolve 32 KiB; edits 264 KiB; batch prepare 64 KiB; batch resolve 32 KiB. Responses/canonical disclosure are never truncated. ErrReviewInvalidRequest is a predecessor 144 sentinel for proven pretransaction refusal; unknown infrastructure/commit outcomes remain retryable.
- `internal/web/ui/src/candidateInbox/controller.ts`, `pendingDecision.ts`, `previewSupport.ts`: exact displayed refs/options/preview/dependency/action binding, scope fencing, explicit mutation refresh, supported v1–v5 rendering capability, source/freshness invalidation, bounded metadata-only exact delivery recovery. The journal is capped at 32 KiB UTF-8 bytes, admitting every legal 4 KiB escaped reason and 20 maximum-length actions. Failed batch groups are never replayed automatically. Earlier winning resolutions are shown separately from current failed-group results.
- `AdvancedReviewForms.tsx`: explicit same-name alternatives, Entity/Alias/Predicate choices, error/changed correction choice, typed literal/polarity/validity/meaning edits with before/after and parent binding. New previews are mandatory after interpretation changes.
- `ReviewDisclosure.tsx`: complete identity creation/reuse, global Predicate definitions, Aliases, Claims, sources/context, original contracted clock authority, correction lifecycle/validity, typed edit/identity/correction history and recursive compound member/record/dependency disclosure. A complete canonical expansion is available in addition to readable summaries.
- `BatchReview.tsx`: explicit candidate revision selection, named atomic groups and provider/dependent field bindings. The entire preview and independent partial-failure semantics precede confirmation; durable outcomes and authorized operation inspection follow it.
- `candidate_review_advanced_test.go` (fresh HTTP contributor), `candidate_review_input_error_test.go`, and advanced frontend tests: real SQLite and focused controller/render regressions.

## Verification

Final checks and full logs are recorded in `146-focused-verification.json`:

- `go test ./internal/web -run 'CandidateReview|CandidateAdvancedHTTP' -count=1`: PASS, 2.006s.
- `go test -race ./internal/web -run 'CandidateReview|CandidateAdvancedHTTP' -count=1`: PASS, 41.501s.
- UI `npx tsc -b`: PASS.
- UI `npx vitest run`: PASS, 27 files / 155 tests, including the pre-existing user UI tests.
- UI `npm run lint`: PASS; existing user-owned Icon FastRefresh warning.
- UI `npm run build`: PASS; existing Vite chunk-size warning.
- Owned tracked `git diff --check`, Go formatting and exact all-file trailing-whitespace/newline audit: PASS.

Tests cover closed-session review/reopen, exact shared Entity/Predicate compound effects, savepoint-local rollback plus independent successful groups, postcommit response loss and exact replay after DB reopen, immutable edit/identity/correction inspection, source-policy redaction/staleness, scope isolation, all route input guards/inclusive caps, maximally escaped reasons, definitive pretransaction refusal, unknown server recovery, stale async UI reads and safe earlier-resolution display. The implementation never claims agent-driven fixtures are David pilot data or model-quality evidence.

## Remaining root gates

Run `./scripts/verify-change.sh` on the exact frozen ticket tree after final 144 refreeze; independent reviews have not been replaced by this owner's checks. Use root's `146-browser-fixture` plus `146-browser-steps.md` for actual CUA demonstration. Root owns the actual browser session/server and cleanup. Human evaluation/model-selection/pilot gates elsewhere in Stage 4 remain unchanged.

No changes to main/serve/db hooks, the user's App/Memory graph/DataHub/database/theme/shell, or user-owned `api/memory.ts` are included. Original bytes are retained in `146-originals` and the HTTP contributor's new-file manifest matches the aggregated frozen copy exactly.

## History review followup

The independent Spec review finding is fixed in the same 18-file frozen manifest. See `146-history-fix-handoff.md` and `146-history-final-manifest.json` for the exact eight changed paths, targeted regression evidence and final hashes. All initial files/checkpoints are retained in `146-before-history-fix`. The original pre-ticket hashes remain unchanged and still match committed ticket 144. Root must rebuild the final isolated tree before rerunning full verification, independent reviews and browser history interactions.
