# Use bounded node-specific retries and safe cancellation

Each workflow node declares a bounded Retry Policy appropriate to its behavior:
pure deterministic work may retry, reads use limited backoff, AI nodes may retry
invalid typed output, and external effects retry only with connector-proven
idempotency. Outcome Unknown never retries automatically. Cancellation prevents
future nodes but cannot undo completed effects; an in-flight effect reconciles
before cancellation finishes, while revoked Standing Authority blocks new runs
and pauses active runs before their next covered effect.
