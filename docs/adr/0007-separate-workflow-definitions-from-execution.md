# Separate workflow definitions from durable execution

Evie will implement its own Go-native workflow runtime rather than depend on
LangGraph. Procedural memory owns Git-backed, reviewed Workflow Definitions,
while the workflow runtime owns Workflow Runs, checkpoints, interruptions,
leases, retries, and external-effect receipts in SQLite. The runtime receives
its own specification and implementation stage because durable execution and
side-effect recovery are independently testable, high-risk behavior rather than
an incidental extension of Markdown procedural memory. Runs own their lifetime
and lease independently of sessions: a session may attach or detach a live view,
but disconnection does not cancel a run.
