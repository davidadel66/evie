# Version user Agent Presets in procedural Git

Evie ships immutable built-in Agent Presets such as `standard` and stores
user-authored presets as reviewed assets in the Git-backed procedural repository.
Workflow-style proposal and approval records activate an exact preset content
hash, and each new session durably pins that version; later edits affect only
new sessions. Built-ins cannot be shadowed, and preset membership selects
capabilities without granting Standing Authority. This improves on storing only
a mutable preset name, which cannot reproduce the composition a resumed session
originally used.
