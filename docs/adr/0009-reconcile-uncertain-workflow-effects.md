# Reconcile uncertain workflow effects before continuing

Every external workflow effect records its accepted input, pinned definition
and authority, node attempt, stable Effect Intent, provider response or receipt,
normalized output, and next checkpoint in durable state. Because an external
provider and Evie's SQLite cannot commit atomically, an intent without provable
terminal evidence becomes Outcome Unknown. Evie first reconciles it through the
provider's idempotency or lookup facilities and otherwise pauses for the owner;
it never blindly retries an uncertain write or payment.
