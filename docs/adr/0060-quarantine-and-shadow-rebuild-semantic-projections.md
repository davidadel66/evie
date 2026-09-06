# Quarantine and shadow-rebuild semantic projections

A failed semantic verification quarantines only affected scopes rather than
serving divergent state or disabling unrelated Episodic Memory. Owner-only
verification and rebuild replay accepted operations into shadow tables, verify
canonical hashes and Scope Revisions, and atomically swap only a valid projection
under a fenced maintenance lock. Startup performs inexpensive schema, foreign-key,
revision, and operation-frontier checks instead of a full replay. An unknown
operation schema version fails closed at that operation and is never skipped;
replay cannot mutate the live projection until the complete shadow result passes.
