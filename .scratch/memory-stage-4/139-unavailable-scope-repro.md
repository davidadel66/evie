# Root139 unavailable-source regression

Frozen tree96503100256f09cba28d5c6d122714db041cb402 passes existing full suite but failed an additional public-behavior probe. Select project history first, select independent global history second, archive the project before reconciliation, run12ticks. Each materialization attempt returns compiler Project unavailable; the later global request gets0jobs instead of1. Per-root priority updates rolled back with the source-authority error. Independent Standards reviewer confirmed same path.

Command in isolated snapshot: go test -overlay /Users/davidboktor/code/evie/.scratch/memory-stage-4/139-unavailable-overlay.json ./internal/eviedb -run '^TestHistoryUnavailableScopeDoesNotStarveOtherRequest$' -count=1 ->FAIL0.346s. Probe stored139-unavailable-scope-probe_test.go; overlay adds test without mutating snapshot. Initial command accidentally used live cwd and matched no tests; rerun in isolated cwd confirmed actual failure.

Owner139 fixing while preserving authorization, captured scope/cutoff and revisitable durable obligation. Recheck root rotations/resume fairness plus restore/reopen.
