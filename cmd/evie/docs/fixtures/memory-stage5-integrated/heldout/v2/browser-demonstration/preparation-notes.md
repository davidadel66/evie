# Disposable Stage 5 browser preparation

This is a scripted-provider UI/state demonstration, not a model quality or held-out measurement. It uses explicit disposable SQLite, public approved Memory operations, real `Session.Send`, real context snapshots/receipts, and the real web handler/assets. No user database, actual model endpoint, production source or frozen evaluation input was changed.

The only repository addition is `cmd/evie/memory_stage5_browser_test.go`. `heldout-v2-browser-source-proof.json` confirms all 394 frozen compiled inputs remain byte-identical and this cmd test lies outside that manifest. It records the exact helper hash and a zero-finding whitespace audit.

## Checks and retained artifacts

- Normal opt-in-disabled compile: `go test ./cmd/evie -run '^TestStage5BrowserFixture$' -count=1` passed 0.335s (fixture skipped by design).
- First opt-in setup (`heldout-v2-browser-preflight-v1.log`, retained DB and request captures) failed because the new fixture omitted the required lifecycle idempotency key. This was corrected only in the fixture; it was not a production behavior regression.
- Second setup passed: test 0.32s / package 0.662s.
- Final setup additionally asserts both persisted opt-out statuses are unavailable and have no evidence references. Exact command: `env EVIE_STAGE5_BROWSER_FIXTURE=1 EVIE_STAGE5_BROWSER_VALIDATE_ONLY=1 EVIE_STAGE5_BROWSER_OUTPUT=/tmp/evie-memory-stage5/heldout-v2-browser-preflight-v3 go test ./cmd/evie -run '^TestStage5BrowserFixture$' -count=1 -v`. Passed: test 0.33s / package 0.657s; full log retained.
- `go test -c -o /tmp/evie-memory-stage5/heldout-v2-browser.test ./cmd/evie` passed. Binary SHA256: `71d97e3382655f32941f851c26e72fececdc8b68b4c07284c05a276bd71e4dda`.
- `git diff --check -- cmd/evie/memory_stage5_browser_test.go` exited 0; because the new file was untracked, the separate direct whitespace audit provides the applicable new-file check.

## Live invocation

```sh
env EVIE_STAGE5_BROWSER_FIXTURE=1 \
  EVIE_STAGE5_BROWSER_OUTPUT=/tmp/evie-memory-stage5/heldout-v2-browser-live-v1 \
  /tmp/evie-memory-stage5/heldout-v2-browser.test \
  -test.run '^TestStage5BrowserFixture$' -test.v -test.timeout=65m
```

The exact live URL and all eight Workspace/source/reader IDs, answer IDs, snapshot IDs, original references and per-memory-request statuses are in `heldout-v2-browser-live-v1/ready.json`. Each directory must be NEW; existing outputs are not overwritten. The live log is `heldout-v2-browser-live-v1.log`. Captured `request-NNN.json` files preserve actual composed provider requests, their serialization byte count and hash.

The fresh-chat reader is deliberately empty. Send exactly `Suggest dinner matching my saved diet.` once in the browser composer. The script refuses unexpected/repeated requests and asserts the saved dietary Claim appears on this first actual request. Other reader histories are preplayed. Source-session histories show the actual original inputs and public lifecycle work. Select reader IDs from ready metadata where Workspace chat titles look similar.

Inspect each answer's Worked activity and source card; compare request tabs for the expansion and historical/conflict scenarios. Scenario 07 was recorded before approved correction/source retraction, and the SQLite database was actually closed/reopened before the server started. Public inspection already verifies the old azurefolio version stays available with current superseded status, while the restricted embermanifest reference has no source text. Scenario 08 persisted two real opt-out turns before the environment was restored for read-only browsing; no runtime UI toggle is implied.

The fixture uses the existing large scripted CLI context profile and Memory tools. It is not the frozen reader experiment profile. All answer text is explicitly labeled scripted demonstration text. Its assertions establish supplied evidence/state; the root agent separately records actual browser clicks and screenshots. `SIGUSR1` stops the server after inspection, retaining `closed.json`; that record never attests that manual checks passed. It distinguishes whether the fresh script returned from whether one actual fresh answer and snapshot persisted. SIGINT/SIGTERM or the one-hour limit also stop the server without claiming a successful manual walkthrough.

Full `./scripts/verify-change.sh` remains the root agent's required final check after this cmd test addition. No Git index, branch or commit mutation was performed by this subtask.
