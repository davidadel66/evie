# Require explicit Context Scope selection

A session starts explicitly inside a Workspace, a filesystem project, or no
Context Scope. A Workspace supplies its default Agent Preset, while a session
outside any Context Scope uses `standard`. Evie may suggest opening a new
session in a relevant Workspace but never silently attaches an existing session
to memory, connections, or authority based on inferred intent.
