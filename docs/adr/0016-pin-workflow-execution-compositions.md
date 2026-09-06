# Pin Workflow Runs to their own execution compositions

A session must expose the capability that starts a workflow, but the resulting
Workflow Run does not inherit the session's Agent Preset as its execution
authority or lifetime. At start, the Kernel resolves the Workflow Definition's
required capabilities into an immutable Execution Composition containing exact
provider versions, Connection IDs, schemas, and non-secret configuration. This
lets foreground and background runs resume independently while preserving what
implementation actually produced their effects.
