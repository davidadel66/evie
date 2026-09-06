# Separate Workspace configuration, secrets, and state

Reviewed Workspace configuration and Procedural Workflows live in procedural
Git. The Kernel's secret storage owns credentials, while SQLite owns memory,
Workflow Run history, and changing operational state. A configuration value
that influences workflow behavior is pinned into the Workflow Definition and a
change creates a new version for review, so mutable Workspace configuration
cannot silently alter an approved run.
