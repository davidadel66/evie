# Namespace and pin Capability IDs

Every Capability has a canonical plugin-namespaced Capability ID such as
`square.list_payments`. The Plugin Manager rejects duplicate plugin identities
instead of selecting by load order. Presets may show friendlier labels, but
Workflow Definitions and execution records identify the canonical Capability,
provider version, and schema hash.
