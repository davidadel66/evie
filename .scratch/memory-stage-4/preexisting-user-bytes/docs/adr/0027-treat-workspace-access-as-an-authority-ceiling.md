# Treat Workspace Access as an authority ceiling

Workspace Access defines the outer set of connections, resources, and memory
that sessions and workflows in that Workspace may reference. Each Workflow
Definition must request a narrower, explicit subset through Standing Authority,
and an Agent Preset only exposes capabilities. Neither Workspace membership nor
preset selection grants authority to perform consequential actions.
