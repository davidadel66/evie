# Ticket #145 basic web review handoff

Implement only after #140 review/source fixes freeze. Read published-bodies/14-web-inbox-basic-review.md, owner review contract and implement skill. New HTTP adapter + frontend inbox over exact shared Kernel seam; no edits or compound effects until #146.

## Shared seam and authority

#140 defines opaque eviedb.OwnerReviewContext, minted by LocalOwnerReviewContext(ctx, exactScope) only in a trusted host. Existing CLI adapter cmd/evie/candidate_review.go lists/inspects/prepares/resolves and inspects accepted v6 operation. Scope is explicit; do not deserialize owner capability or effects from HTTP. The local web trust boundary is loopback listener plus guard and POST management route, JSON content type/origin/host checks. Preserve those checks, enforce bounded JSON, and keep request-specific preview/revision/digest/delivery key and explicit accept/reject. Scope switching must clear preview and stale async responses. Source-session lease is never required; do not rebind authority to active conversation as a workaround.

A preview prepares exactly one existing-identity CandidateRef, not all objects from an extraction job. Kernel rejects stale/mixed authority or source-policy changes and redacts ineligible content. Rejection may remain possible with redacted evidence. Never interpret a stale prepare/resolve response as permission to automatically refresh and accept. Kernel decides state; frontend presents it.

## User-owned changes and integration

User has major uncommitted web workspace/DataHub/memory-graph changes. Exact originals are in .scratch/memory-stage-4/preexisting-user-bytes/. Preserve them. Prefer new internal/web/candidate_review.go and tests, new UI candidateInbox directory/API/test files. Root will integrate narrow Server interface/route and main wiring hooks. No App/DataHub redesign or broad formatting. A narrow Memory component integration should work on both committed simple Memory view and user's new graph/records view; root builds isolated commit snapshots with only task-owned hunks. No new production dependency.

Current Server has semanticMemory and contextSessions, plus user-owned databaseInspector. ServeContextManaged user signature has extra databaseInspector argument; committed HEAD does not. Review should be an explicit consumer-owned interface constructor seam, not arbitrary reflection/type assertions. Root handles two exact hookup variants. Existing Memory scope-list HTTP path requires activeSession; new owner inbox must not use an active source session as authority. Choose a bounded applicable-scope listing seam or exact scope selection that remains usable when source closes, following existing owner registry semantics. Expose only appropriate registry identities.

## User journey and checks

Keep inbox separate from accepted Memory graph/query results. Show candidate meaning, support versus assistant context, original authority, exact scope, generation and state. Before acceptance show canonical reviewed effects and source projections. Accept/reject intentional buttons bound to preview; preserve durable idempotency key across retries of same resolution. Refresh resolved state; show actionable stale/error/already-resolved messages without discarding user intent. Safe disabled/empty/paginated states, no raw payload/implementation details needed for ordinary decision.

HTTP real-SQLite tests: closed source, preview/resolve/reopen, scope/origin/host/body/protected-source rejection, stale graph/review and idempotence, CLI/Kernel same persisted operation. Frontend tests: stale list/detail/prepare/resolve response after scope change/unmount, explicit approval, retry delivery, pagination. Browser demonstration when service dependencies permit; root coordinates final full verify + two-axis reviews and exact ticket tree. No installation of model config, no inference required.
