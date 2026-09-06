# Never silently migrate pinned Workflow Runs

After an Evie or plugin upgrade, a Workflow Run continues only when the new
runtime explicitly declares compatibility with its pinned Workflow Definition,
Execution Composition, and capability schemas. Otherwise Evie pauses the run
visibly and requires an explicit migration or rollback. It never substitutes
new behavior merely because an older implementation is unavailable.
