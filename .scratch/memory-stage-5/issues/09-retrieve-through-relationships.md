## Parent

https://github.com/davidadel66/evie/issues/154

## What to build

Evie finds relevant accepted facts connected through known people and relationships.

## Acceptance criteria

- [ ] Add bounded deterministic one/two-hop graph candidate discovery to the existing search path and combine eligible exact, lexical, temporal, and relationship matches through Reciprocal Rank Fusion and transparent reranking.
- [ ] Reapply scope, source eligibility, lifecycle, temporal, and authority constraints at every hop. A Global anchor or a scoped relationship cannot act as a bridge into unrelated memory.
- [ ] Return source-bearing path support and retrieval reasons in the existing evidence results, receipts, and compact inspection UI without converting graph proximity into acceptance authority.
- [ ] Select a focused diverse result under the request budget rather than fill space with weak relationships; document measured graph/result bounds.
- [ ] Demonstrate a relationship question with a supported two-hop answer, direct-match competition, cycles, duplicate paths, contradictory candidates, retired nodes/Claims, and a tempting out-of-scope edge.
- [ ] Verify through the shared search/turn seam and scope matrix. Do not introduce learned graph scores or Stage 6 in-process acceleration. Run required verification.

## Blocked by

- https://github.com/davidadel66/evie/issues/156
