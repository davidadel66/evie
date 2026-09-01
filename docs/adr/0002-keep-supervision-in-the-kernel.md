# Keep supervision in the kernel

Evie's session history, Plugin Manager, credential mediation, safety rules,
action approvals, and durable workflow recovery remain in a non-removable
Kernel. Models, capability collections, external connectors, skills, sandbox
implementations, subagents, and interfaces may gain plugin seams when a concrete
use case requires variation; Evie will not convert every subsystem merely for
architectural symmetry. This keeps plugins replaceable without allowing them to
replace the mechanisms that supervise their authority or recover their work.
The plugin specification preserves a named second-half roadmap for model
providers, memory, coherent sandbox/execution providers, subagents, workflow
integration, and UI extensions. Those seams are implemented later through
independently reviewable outcomes rather than one universal conversion.
Presets and procedural assets may reference only opaque Connection IDs; the
Kernel mediates scoped credentials and never exposes them to the model, Git, or
Composition Receipts.
