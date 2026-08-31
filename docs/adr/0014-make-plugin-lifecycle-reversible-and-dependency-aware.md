# Make plugin lifecycle reversible and dependency-aware

Plugins declare required and optional dependencies, which the Plugin Manager
resolves in dependency order while rejecting cycles. Explicit disabled,
waiting, loading, ready, failed, stopping, and stopped states make health
inspectable. Failed initialization reverses partial registration and blocks
required dependents so a preset never starts half-composed; missing optional
dependencies remain visible warnings. Plugin Health and Connection Readiness
remain separate so missing account setup can be repaired from a working
session. Disabling a plugin blocks new dependent runs. An already-sent external
request must finish or enter reconciliation, after which its Workflow Run
pauses before the next operation requiring that plugin; disabling never erases
run state or treats an in-flight effect as cancelled. An optional plugin failure
does not prevent the Kernel or management surfaces from starting: Evie enters a
visible degraded state and blocks only affected presets, sessions, and runs.
Only a Kernel failure prevents Evie itself from starting.
