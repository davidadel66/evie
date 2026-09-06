# Ticket 146 history followup

The independent Spec P2 is fixed and frozen in the existing 18-file `146-engineering-checkpoint.json` / `146-frozen-files`. Eight paths changed; exact prior/final SHA-256 values are in `146-history-final-manifest.json`. Total frozen size is 187,908 bytes. Initial checkpoint, root verification record, handoff and all 18 frozen source files are preserved under `146-before-history-fix`. Original pre-ticket hashes are unchanged and still match committed #144. No staging, commits, dependencies, Kernel changes, root hooks or user-owned UI files were touched.

## Behavior and review entry points

- `AdvancedReviewForms.tsx` replaces the edit-only revision form with an explicit Owner edit / Identity choice / Correction choice selector and a bounded integer revision field. It explains the shared sequence. A loaded result is displayed only under the matching selected kind and number. Current recorded identity/correction choices also have readable disclosure.
- `controller.ts` uses the discriminated `InterpretationHistory` state and `loadHistoryRevision(kind, revision)`. Each kind calls its existing typed API endpoint. The response must match the selected candidate, exact revision/parent, recorded review revision and (for choices) scope/original option reference. Redacted candidates cannot request history. Existing request epochs fence delayed reads after scope navigation, candidate selection or refresh; source/freshness failures clear disclosure. Wrong-kind/missing reads do not change the candidate or erase owner intent.
- `ReviewDisclosure.tsx` renders immutable identity/correction history: parent and review revisions, audit, exact chosen identities/earlier claim, retained alternatives and Alias/context, recorded correction meaning, original unknown effective time and historical lifecycle. Complete canonical records remain expandable. Historical choices are explicitly interpretation rather than approval or new evidence.
- `candidate_review_advanced.go` provides a narrow history-route adapter for the three existing endpoints. It validates a nonempty candidate ID and positive revision after current scope authorization. Only `errors.Is(err, sql.ErrNoRows)` from these reads becomes `review_revision_not_found` (404). The error message explains matching kind/revision navigation. Invalid history input is 400. Unknown database errors, cancellation and failures with coincidentally similar text retain 503 `review_retryable`; source and authorization errors retain their existing meanings. There is no automatic scan across history kinds or generations.
- `advancedController.test.ts`, `advancedDisclosure.test.tsx`, `candidate_review_input_error_test.go` add mixed identity/edit/correction sequence coverage, exact binding refusal, bounded input, stale/scope/source/selection fencing, readable history and actual SQLite wrong-kind/missing-row behavior. The HTTP error fixture tests wrapped typed absence versus untyped similar text, infrastructure and cancellation.

## Verification

Final command records/logs: `146-history-verification.json`.

- `go test ./internal/web -run 'CandidateReview|CandidateAdvancedHTTP' -count=1`: PASS, package 2.006s.
- `go test -race ./internal/web -run 'CandidateReview|CandidateAdvancedHTTP' -count=1`: PASS, package 41.501s.
- UI `npx tsc -b`: PASS.
- UI `npx vitest run`: PASS, 27 files / 155 tests; includes pre-existing user UI tests.
- UI `npm run lint`: PASS; existing user-owned Icon FastRefresh warning.
- UI `npm run build`: PASS; existing Vite chunk-size warning.
- `git diff --check --` the eight changed paths: PASS. Three changed Go files formatted with `gofmt`; all 18 final files passed exact newline/trailing whitespace audit, frozen/live byte equality and original before-hash validation against current HEAD.

Regression evidence is retained in `146-history-ui-red.log` (five missing-history-method failures before implementation), `146-history-http-red.log` (actual missing/wrong-kind 503 before the fix), and `146-history-http-focused.log` (PASS 0.411s after the adapter fix). Focused controller/disclosure tests also passed 39 tests before the final full UI suite. `146-history-fix.diff` is the exact eight-file followup diff against the initial freeze.

Root owns the updated isolated `./scripts/verify-change.sh`, isolated UI tests, independent Standards/Spec rereviews and actual CUA demonstration before the single #146 commit. These required final gates were deliberately not duplicated in the mixed live working tree. The initial root full-check result belongs to the preserved older frozen tree and must not be reused as the final result.

## Browser demonstration addition

In root's synthetic browser fixture, after making an identity choice, open **Inspect interpretation history**, select **Identity choice**, enter its recorded interpretation revision and click **Load immutable history revision**. Confirm the recorded choices and both same-name alternatives. After editing that candidate, select **Owner edit** at the new revision; confirm before/after and parent. Select **Identity choice** at that edit's revision to demonstrate the safe wrong-kind message, then load the earlier identity revision again. On the correction candidate, record a correction choice and inspect that revision with **Correction choice**; confirm old claim/mode and unknown effective time. Navigating scope or refreshing must not leave the historical disclosure attached to the next selection. These agent-operated synthetic fixtures remain separate from David pilot/model-quality evidence.
