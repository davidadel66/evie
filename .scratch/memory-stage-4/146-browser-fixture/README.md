# Advanced review browser demonstration fixture

Root-owned disposable harness for ticket146. It uses a real temporary SQLite database and public Store compilation/review APIs with scripted extraction. No live model or real owner pilot observation is involved.

Scenarios: editable simple owner preference, two unresolved Maya/project suggestions sharing a proposed person and Predicate only after explicit dependency binding, an error correction against the seeded tea Claim, an uncompleted future plan, contracted local clock support with no timezone/effective instant, and a separate session-scope candidate. The source session closes before HTTP serves the inbox. The harness prints scenario candidate IDs and the exact temporary database path.

Final setup will be rebuilt from the verified isolated146 source, with these two Go files copied beneath that snapshot so internal-package imports and embedded frontend match the exact reviewed tree. Run `go run ./.scratch/memory-stage-4/146-browser-fixture` there, inspect through CUA, then send SIGINT. Shutdown closes the server/database and removes the temporary data directory. Do not use or replace the owner's database.

Initial setup validation: final `go build` and fixture startup PASS on working144/146 APIs. Earlier setup attempts used nonexistent constant names and then the wrong generation policy for unresolved identities; these fixture errors were corrected. Identity proposals use identity-review-v2; temporal proposals use temporal-review-v3; clock uses owner-clock-observations-v2. All five advanced candidates compiled successfully. Initial server was stopped cleanly and its temporary database removed. Actual browser acceptance/edit/batch checks await the frozen146 frontend and final144 kernel.
