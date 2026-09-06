# Ticket149 deterministic Stage4 conformance

Read published ticket18 and the parent complete acceptance scenario. Build a repeatable scripted end-to-end demonstration over real SQLite and actual foreground/worker/review surfaces, not a unit-test inventory or a fake model-quality result. Stall extraction while a foreground turn reaches terminal commit and response-finalization; restart/recover selected work; close source session; inspect/approve exact candidate; inspect accepted graph/provenance; quarantine/rebuild/replay without extractor/model/tool calls.

Prove Kernel/CLI/HTTP/web shared effect/source hashes and outcomes over global, multiple Workspace/project and exact session scopes. Preserve unaccepted isolation, generic storage containment and no implicit Promotion. Include explicit selected history, outside-selection ranges, failed earlier gap/later success, successful empty, distinct generations and preserved edits/rejections. Competing processes/expiry/replacement/cancellation/stale completion/duplicate publication and review must preserve durable attempts/capacity and exact accepted outcomes. Source-policy revision/visibility changes fail closed; batch group and outer transaction/crash rollback behave exactly as contract. Avoid duplicating existing narrowly tested implementation mechanics; add integrated seams where needed.

Publish versioned machine-readable conformance results with exact commands/test names/environment/tree/hash and warnings/skips. Any exact scope/authority/source/persistence/replay violation blocks conformance. Keep learned extraction/resource/active human-review claims absent. Run required full verification and record browser interactions with scripted temporary data. Retain all existing focused tests. No live model, output adjudication, owner pilot imitation, final-holdout creation/exposure or ongoing enablement. Root handles final two-axis review/checks/one commit; preserve dirty user work.


## Known startup observation to assess

During144 migration race tests, concurrent fresh lazy SQLite connections could fail at connection initialization with SQLITE_BUSY while enabling WAL, before reaching the schema migration. The migration test now preopens connections to isolate its owned boundary; genuine populated concurrent OpenDBAt still passes. Preserve this distinction. In149's actual multi-process conformance, include ordinary fresh/existing database startup appropriate to the deployment contract, not only preopened connections. If the earlier connection-initialization race reproduces on the intended public path, diagnose and fix that concrete conformance failure under149 rather than claiming universal startup safety or hiding it with test-only priming. See144-migration-frozen-handoff.md and test TestCorrectionSchemaConcurrentFreshBootstrap for exact prior limitation. Keep changes narrow and coordinate db.go root hooks/user bytes.


## Root reproduced and minimized public startup failure

See149-startup-diagnosis.md and149-startup-*-results.json. Fresh actual public starts reproducedSQLiteBUSY; onlyconnectioninitializationneeded. A narrowpre-schemaPingBUSYretryprototype passes320fresh/existingpublicstarts. No liveproductionfixyet. Review/foldthehelperandaddpublicregression; rootownsdb.gohookon148baseline. Do notoverwriteitwiththeoldprototypewholefile.
