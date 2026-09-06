# Usage decisions

- **2026-09-06 — approve owner usage analytics and the `evie.` wordmark.**
  David approved the researched Data Usage proposal and requested the preview's
  lowercase wordmark with its period. usage.spec.md defines this delivery.
  This extends the 2026-08-25 diagnostic-only usage decision only by permitting
  a separate content-free owner aggregation/read surface. Existing immutable
  assistant payloads and their capture/continuation behavior remain unchanged.
  The existing exclusions for compaction, extraction and failed attempts are
  displayed as coverage limits until a dedicated capture story changes them.

- **2026-09-06 — account activity and detailed observed tokens retain separate coverage.**
  The installed Codex account API returns account daily activity and allowances.
  A checked task returned no task usage detail. Local history has detailed
  tokens without trustworthy historical account attribution. The UI therefore
  presents account totals and unassigned local detail separately. It never
  uses the current login to label historical usage or sums overlapping feeds.
  Additional existing Codex homes may be configured server-side; the feature
  does not switch or manage credentials. Collector caches are operational
  accelerators, not a second billing ledger; underlying source retention is
  visible as a coverage limit.
