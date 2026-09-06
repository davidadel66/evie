# Isolate Context Scope memory and require explicit promotion

A session may retrieve genuinely owner-wide global memory together with its
own Context Scope and session memory, but it cannot retrieve memory from
another Workspace or project. New durable memories default to the active
Context Scope. Promoting a memory to global scope is explicit so a local fact
cannot silently become available everywhere.
