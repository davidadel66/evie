---
status: superseded by ADR-0006
---

# Load enabled plugins automatically and select capabilities per request

Evie's Plugin Manager may automatically load an enabled plugin without prompting
the owner. For each model request, Evie selects only the capabilities relevant
to the current work; loading a plugin does not automatically place all of its
tool schemas and instructions in the model's context. Account connection remains
a separate setup concern, and existing action-approval policy continues to
govern consequential operations. This keeps ordinary conversation fluid and
context small without treating loaded code as permission to perform every action
that code can support.
