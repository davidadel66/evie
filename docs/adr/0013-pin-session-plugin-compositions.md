# Pin immutable plugin compositions per session

Evie resolves the enabled compiled-plugin set at process startup and does not
hot-reload plugin code initially. Every session persists a content-free
Composition Receipt with its exact preset version, Evie version, plugin and
capability identities, schema and instruction hashes, and non-secret
configuration references. Preset edits affect new sessions, while plugin code
changes require restart; a missing pinned version fails resume visibly instead
of silently substituting newer behavior.
