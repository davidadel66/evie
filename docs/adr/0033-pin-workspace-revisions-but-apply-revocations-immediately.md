# Pin Workspace Revisions but apply revocations immediately

Workspace configuration changes are reviewed and versioned. A session pins the
Workspace Revision from which it started, and default Agent Preset additions or
changes affect new sessions. Removing access is a safety revocation that takes
effect immediately despite the pin, preventing new covered actions and pausing
affected Workflow Runs before their next action.
