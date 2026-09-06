# Root v5 malformed-envelope regression

On the working ticket144 implementation after143 commit d61b2e7f29a751113e2eb0d713ad59f73caca740:

`go test -overlay .scratch/memory-stage-4/144-malformed-compound-overlay.json ./internal/eviedb -run '^TestRootMalformedCompoundFailsClosed$' -count=1`

FAIL (0.240s): malformed compound panicked instead of failing closed: runtime error: index out of range [0] with length 0.

The temporary overlay introduces only a test file; no repository file is overwritten. It constructs a v5 preview with two members and a declared dependency but empty member Claim arrays. validateReviewCompoundEncoding calls bindReviewDependencies before checking each member Claim count. Owner144 was asked to validate structure first and cover malformed persisted previews/accepted replay, including missing support arrays. Required behavior is a safe validation error/quarantine, not a process panic. Final resolution and checks will be recorded with ticket144.

Root repeated the exact overlay after the structural-validation fix: PASS (0.246s). The owner also added permanent malformed stored-preview and accepted-operation replay regression coverage.
