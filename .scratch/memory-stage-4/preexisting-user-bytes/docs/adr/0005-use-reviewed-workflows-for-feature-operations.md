# Use reviewed workflows for feature operations

Feature Plugins expose high-level operations backed by Procedural Workflows
instead of asking the model to reconstruct a sequence of raw connector calls on
every run. A workflow is proposed and reviewed in an Evie session, then versioned
through procedural memory before it becomes active. Cairo's Kitchen tip closeout
will therefore coordinate Square and Google Sheets through a reviewed workflow,
while those Connector Plugins remain independently usable for ad hoc work.
