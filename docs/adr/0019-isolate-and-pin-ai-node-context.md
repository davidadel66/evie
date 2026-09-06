# Isolate and pin AI Node context

An AI Node receives only its reviewed prompt, declared Workflow Run state,
explicitly requested scoped memory, and declared capability results. Evie does
not implicitly expose an ambient chat transcript or general memory. Each AI
Node references a named and versioned Model Policy that resolves the exact
provider, model, parameters, and output schema, and each Workflow Run pins that
policy. Changing the policy creates a new Workflow Definition version that
requires Workflow Approval.
