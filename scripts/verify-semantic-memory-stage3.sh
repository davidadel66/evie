#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

go test -count=1 -v ./cmd/evie \
  -run '^TestSemanticMemoryStage3CrossSurfaceAcceptance$'

go test -count=1 -v ./internal/eviedb -run '^(TestSemanticScopeContainmentAcceptanceMatrix|TestSemanticMemoryTwoProcessRacesCommitOnlyValidOperations|TestSemanticClaimsUseExactAllowedScopeMatrix|TestArchivedSessionSemanticMemoryIsExplicitlyInspectableButNotShared|TestGraphLinkEndpointScopeMatrixAndProvenanceRedaction|TestSemanticMemoryIdempotencyAndStaleProposalChangeNothing|TestCorrectClaimRejectsInvalidAndStaleProposalsWithoutSemanticWrites|TestCorrectClaimConcurrentApplyAndCollidingTransactionTimesUseScopeRevision|TestPromotionRejectsStaleSourceAndDestinationWithoutWrites|TestPromotionTwoStoreRaceCommitsOneRevisionAndSurvivesReopen|TestExactSemanticPaginationFiltersTraversalTemporalLifecycleAndRestart|TestSemanticProjectionQuarantinesOnlyTheDivergentScope|TestSemanticProjectionVerifyQuarantineAndOwnerRebuild|TestSemanticEvaluationStage3Corpus)$'

go test -count=1 -v ./internal/plugins -run '^Test.*Memory.*$'
go test -count=1 -v ./internal/agent -run '^Test.*Memory.*$'
go test -count=1 -v ./internal/web -run '^TestSemanticMemoryHTTP.*$'
go test -count=1 -v ./internal/tools \
  -run '^(TestQueryDBEvieAllowsOnlyPublicTables|TestResolvePath|TestResolvePathRejectsSymlinksToMemoryStorage)$'

npm --prefix internal/web/ui exec -- vitest run --no-cache \
  src/api/memory.test.ts \
  src/memory/Memory.test.tsx \
  src/memory/latestRequest.test.ts

./scripts/verify-change.sh
