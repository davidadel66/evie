# Automatic recall development diagnostic: failed

The first growing-corpus run met only 13 of 30 exact-target-or-empty expectations.
Repeated prior user questions displaced the original greenhouse evidence and
turned later unsupported questions into irrelevant matching excerpts. No metric
or case was changed to relabel these results.

Command: `go test ./internal/agent -run '^TestAutomaticMemoryRecallDevelopmentMeasurements$' -count=1 -v`.

This was a development diagnosis on the live implementation worktree, not a
frozen benchmark or held-out evaluation. The exact compiled production binary
was not retained; the immutable original log, report and unchanged test fixture
are retained here. The fixture SHA256 is `7d6239625758ec0928ae62e3bd73022147b60c9a697aeabff2049c44415ddede`. A new version must record the next run after the implementation fix.
