# Stage 5 browser demonstration

The Codex root agent exercised eight scenarios through the real Evie browser UI on September 11, 2026. This was an agent-operated manual walkthrough, not a human review. The retained record contains **11 complete accessibility observations and eight screenshots**. It is separate from the held-out reader evaluation: the provider returned explicitly scripted answers, and no actual model requests or new reader-quality samples were made.

The fixture used disposable SQLite, public approved Memory operations, real `Session.Send`, the real web handler and built UI assets. Seven scenarios had prepared conversation history. For the fresh-chat scenario, the root agent typed and sent the ordinary question once through the composer. The actual first request supplied accepted preference evidence; [closed metadata](live-closed.json) confirms one saved answer and one saved request snapshot with status `success`. The fixture itself explicitly does **not** attest manual checks passed. The [browser observation record](manual/observations.json) identifies the root agent as the UI operator and preserves the actual accessibility states.

| Scenario | UI interaction and observed state | Screenshot |
| --- | --- | --- |
| Fresh-chat preference | Sent “Suggest dinner matching my saved diet.” in the empty reader, then inspected the saved dietary Claim and original owner source. | [01](manual/scenario-01.png) |
| Uncompiled original | Opened the fern answer's sources; the original observation appeared as an attributed Conversation excerpt, with its event and exact locator. | [02](manual/scenario-02.png) |
| Cross-topic reference | Opened the birthday answer after the parser detour and inspected Maya's accepted preference. | [03](manual/scenario-03.png) |
| Bounded investigation | Compared request 2's original match with request 3's neighboring original context. The possible visit remained tentative. | [04](manual/scenario-04.png) |
| Historical conflict | Inspected the conflicting accepted residence entries, the newer owner statement, and explicitly historical retired workplace evidence with its fact-valid interval. | [05](manual/scenario-05.png) |
| Retirement suppression | Compared the before-retirement original receipt with a fresh after-retirement answer: the retired instruction was omitted while the unrelated notebook statement remained available. | [06](manual/scenario-06.png) |
| Original sources after correction and restart | Opened the original answer after public correction/source retraction and actual SQLite close/reopen. The earlier azurefolio version remained inspectable with current superseded status; the restricted embermanifest source was unavailable. | [07](manual/scenario-07.png) |
| Memory unavailable | Inspected both persisted opt-out turns: the self-contained rewrite continued; the personal-memory answer reported unavailability. Both receipts retained `unavailable` with no supplied references. | [08](manual/scenario-08.png) |

These observations demonstrate presentation, persisted provenance and current-access behavior with scripted text. They do not establish model interpretation quality, change any held-out gate or repair any failing release criterion. The larger scripted CLI context profile is also distinct from the frozen evaluation profile. The root agent stopped the fixture with `SIGUSR1`; it exited 0. The live test's 1576.97 seconds include the operator's browser wait and are **not a latency measurement**.

## Preserved evidence

- [session-artifacts.tar.gz](session-artifacts.tar.gz): byte-exact files from the three setup attempts and the closed live run, including four SQLite databases, all ready/closed metadata and **98 captured provider requests** (13 failed-setup, 28 second-setup, 28 final-setup, 29 live). The live run includes the fresh browser request. [archive-members.json](archive-members.json) records each original path, size and SHA256.
- [manual/observations.json](manual/observations.json) and eight PNGs: root agent's actual UI accessibility observations and screenshots, copied unchanged.
- [logs](logs): complete setup/live logs. [preflight-history.json](preflight-history.json) retains the fixture-only lifecycle failure and its successful correction. The initial missing-import compiler diagnostic is described honestly as a tool-result summary; no separate raw compiler log or exact earlier helper source snapshot was retained.
- [live-ready.json](live-ready.json), [live-closed.json](live-closed.json): convenient unchanged copies of the archived live metadata. Ready-session title fields reflect their creation time; the browser read current titles from persisted original user messages.
- [source](source): exact final fixture source and its three referenced existing cmd test-helper files, with `.txt` suffixes to keep preserved copies outside the Go package discovery graph. The binary is excluded; its measured SHA256 was `71d97e3382655f32941f851c26e72fececdc8b68b4c07284c05a276bd71e4dda`.
- [preservation-record.json](preservation-record.json), [pre-run-source-proof.json](pre-run-source-proof.json): helper identity, actual counts and source qualifications. All **394 frozen compiled inputs** still match [their unchanged manifest](frozen-compiled-source-manifest.json). The new cmd fixture is outside that manifest. A complete cmd pre-build manifest was not recorded, so this record does not claim a byte-identical executable rebuild.

## Reproduce the disposable fixture

Run from the reviewed repository root, with the recorded fixture installed at `cmd/evie/memory_stage5_browser_test.go`. Its final SHA256 is `fd82bd86759860734c725b136e401431c13c8c0939b0d1d361397373a41cbe9a`. The existing UI build must be present because the actual web handler embeds it. No production configuration switch, new dependency or ordinary `evie serve` invocation is required.

```sh
go test -c -o /tmp/evie-stage5-browser.test ./cmd/evie
env EVIE_STAGE5_BROWSER_FIXTURE=1 \
  EVIE_STAGE5_BROWSER_OUTPUT=/absolute/new/disposable-directory \
  /tmp/evie-stage5-browser.test \
  -test.run '^TestStage5BrowserFixture$' -test.v -test.timeout=65m
```

The output directory must be new. The fixture prints `STAGE5_BROWSER_READY` with its ephemeral loopback URL and exact session identifiers, selecting the empty fresh-chat reader. Send `Suggest dinner matching my saved diet.` once; unexpected or repeated scripted requests return an error and cannot invoke a real model. Browse the other seven existing reader sessions through their labeled Workspaces and inspect each answer's Worked/source card. Source histories show the actual original messages and approved lifecycle operations. The unavailable scenario records turns created under `EVIE_REMOTE_MEMORY=off`; it does not imply a UI opt-out toggle. Signal the owned test process with `SIGUSR1` when finished; signal/timeout termination never asserts manual success.

For a setup-only check, add `EVIE_STAGE5_BROWSER_VALIDATE_ONLY=1`. The final recorded setup command and exact outputs are retained in [preparation-notes.md](preparation-notes.md). Normal `go test` skips the fixture unless explicitly enabled. The root agent owns the final `./scripts/verify-change.sh` result after this new cmd test.

Verify preservation without running the app or any model:

```sh
python3 cmd/evie/docs/fixtures/memory-stage5-integrated/heldout/v2/browser-demonstration/verify_artifacts.py --source-root .
```

The verifier checks every retained direct file and archived member, reads all four databases with SQLite's immutable read-only mode, validates the closed fresh-turn metadata, and optionally checks the 394 repository input hashes. Archive extraction is unnecessary for inspection of the manifest; standard `tar -xzf session-artifacts.tar.gz -C /new/directory` recovers each exact original SQLite/request file when desired.
