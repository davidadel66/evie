# Ticket 146 HTTP conformance contribution

Owns only `internal/web/candidate_review_advanced_test.go` (previously absent).
Final bytes are frozen under `146-http-frozen-files`; manifest is
`146-http-frozen-manifest.json`. No production edits, dependency changes,
branch changes, staging or commits by this contributor.

The five focused tests use the existing real SQLite web fixture and scripted
compiler output. They cover every advanced route's origin/host/method/content
protections, exact typed envelopes, unknown/duplicate/null/array/trailing JSON,
UTF-8/nesting, exact inclusive request caps, and Kernel size/dependency mappings.
They exercise explicit shared Entity/Predicate choices and immutable revisions;
complete compound group disclosure and the prohibition on approving a batch
member alone; source closure; global/session scope isolation; edit parent and
original extraction; stale choices/previews; error versus changed correction;
real persisted accepted operations and replay; current source redaction of
historic edit/identity/temporal/batch inspection; complete stale batch rollback;
independent partial failure after a dependent group's first writes; and lost
commit response followed by the same immutable receipt after database reopen.
Successful and partly failing batch deliveries and a single edited acceptance
include a legal 4096-byte reason that JSON-escapes to 24576 bytes.

The inclusive edit-limit test identified that the original adapter's compiler
JSON validator imposed an unintended 128 KiB cap below the advertised 264 KiB
edit cap. The #146 implementation owner replaced it with a web-owned strict
JSON validator after the existing route body limit. The owner and root also
raised only simple/batch resolve to 32 KiB to carry every bounded escaped reason.
All other simple/choice bounds remain 8 KiB; edit is 264 KiB and batch preparation
is 64 KiB. Regression tests cover the final behavior.

Final checks:
- `go test ./internal/web -run '^TestCandidateAdvancedHTTP' -count=1`: PASS, 0.939s.
- `go test -race ./internal/web -run '^TestCandidateAdvancedHTTP' -count=1`: PASS, 14.745s.
- Changed file formatted with gofmt; no trailing whitespace and final newline.
- `git diff --no-index --check /dev/null internal/web/candidate_review_advanced_test.go`
  emitted no whitespace diagnostics (exit 1 denotes the new-file difference).
- Root retains ownership of independent reviews and the required full
  `./scripts/verify-change.sh` check on the complete isolated ticket tree.

The root's browser harness and #146 frontend tests cover manual rendering and
clock-specific presentation; this contribution does not claim browser or model
quality evaluation. No known remaining adapter defect was observed.
