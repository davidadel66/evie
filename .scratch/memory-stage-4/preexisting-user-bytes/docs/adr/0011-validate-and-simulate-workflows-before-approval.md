# Validate and simulate workflows before approval

Evie offers Workflow Approval only after validating the graph and referenced
capabilities, rejecting unreachable nodes, unsafe cycles, missing limits, and
invalid authority, and completing a dry run with external effects suppressed.
Review presents a human-readable graph plus separate executable and authority
changes, expected inputs and outputs, and affected resources. One approval then
activates the exact reviewed definition and Standing Authority.
