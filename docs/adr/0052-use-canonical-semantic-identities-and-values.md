# Use canonical semantic identities and values

Stage 3 uses random stable IDs, a canonical registry for global, Workspace,
project, and session scopes, and monotonic Scope Revisions for ordering and
concurrency. Claims use validated canonical Predicates and either an Entity or
a closed Typed Literal drawn initially from text, integer, exact decimal,
boolean, calendar date, and UTC datetime. Proposition equality excludes
provenance: matching scope, subject, Predicate, object, polarity, and Valid Time
attach new evidence to the existing Claim rather than duplicating it.
Every mutation carries an idempotency key and the Scope Revision on which its
intent was based. An operation that reads one scope and writes another validates
the complete source/destination revision vector atomically before acceptance.
