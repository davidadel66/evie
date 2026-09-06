# Version global Predicate definitions

Each canonical Predicate has one append-only, versioned definition across all
memory scopes, including its human label, allowed object kind or type, and
expected cardinality. Claims remain scoped and retain the Predicate definition
version under which they were accepted. A narrower scope is context-specific,
not an implicit override: exact queries preserve allowed session, Workspace or
project, and global Claims together with scope labels, while later presentation
may order the current Context Scope first.
