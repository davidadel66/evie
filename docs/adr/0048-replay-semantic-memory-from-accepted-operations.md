# Replay Semantic Memory from accepted operations

Accepted Semantic Operations are the canonical history of Semantic Memory, and
the queryable temporal graph is their deterministic projection. Rebuilding the
graph replays only accepted operations and performs no model calls, extraction,
or external effects, so the same history produces the same scoped entities,
claims, provenance, temporal state, and revisions for recovery and evaluation.
Episodic Memory remains the canonical evidence cited by those operations rather
than becoming a second way to silently reconstruct accepted knowledge. The
read-only `/memory verify` command replays into a temporary shadow projection and
compares canonical per-scope hashes and revisions without mutating live tables.
