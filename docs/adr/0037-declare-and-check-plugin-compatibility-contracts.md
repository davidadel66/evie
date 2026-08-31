# Declare and check plugin compatibility contracts

Each Plugin Manifest declares a stable plugin ID, implementation version,
supported Kernel API version, versioned Capability Contracts, and required or
optional dependency ranges. A resolved session or Workflow Run records exact
provider versions and contract hashes. New plugin code may continue pinned work
only when it explicitly declares compatibility with the required contracts and
schemas; implementation recency alone is not compatibility evidence.
