# Use focused capability-provider interfaces

Every plugin satisfies one small common lifecycle interface for identity,
version, dependencies, startup, and cleanup. It contributes behavior only
through the focused Capability Provider interfaces it actually supports, such
as tools initially and models, sandboxes, subagents, workflows, or UI later.
Evie will not define one universal interface containing every extension hook;
new provider families can therefore be added without changing or breaking
unrelated plugins.
