# Use durable workflow notifications

Every Background Run completion, failure, and needs-attention transition creates
a Durable Notification inside Evie, deduplicated by run and transition.
Notification plugins may later mirror notices to external channels, but the
external message omits sensitive detail by default and is never the only record
of the event.
