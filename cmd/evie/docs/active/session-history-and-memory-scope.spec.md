# Conversation history and recommended memory scope

David authorized this update on 2026-09-05. It supersedes the earlier UI deferral of transcript restoration and amends the default-only memory destination rule. Existing memory remains unchanged.

- Reopening a saved session and reloading its page restores persisted user/assistant messages and recorded tool outcomes. Historical approvals are records, never live actions. Failed/interrupted turns remain distinguishable; private reasoning and internal context snapshots are excluded.
- History reads bind to the selected session, reject stale IDs and busy turns, and return bounded pages. Loading or failed history must not silently present another session's messages. Older pages remain accessible. No model calls or tool executions occur during history reads.
- Evie recommends a memory's applicability independently of its subject and source: Everywhere for general enduring preferences, Workspace for that area's facts, This conversation for temporary conversational instructions. Honor explicit qualifiers; prefer narrow scope when uncertain.
- Scope is a closed choice resolved by the harness to Global, the active Context Scope, or current session. No arbitrary workspace ID may be proposed. Approval shows the exact destination and preserves the original source's scope. Scope changes are part of the approved effect and cannot be changed after approval.
- Preserve legacy proposals and existing accepted memory. No migration or automatic promotion of David's already accepted General memories. Prompt guidance is versioned in code and can be refined later.

Verification: deterministic history projection/HTTP scope and pagination tests; browser reopen/reload reproduction; scope preparation/application/forgery tests; prompt/tool examples and approval UI tests; full repository verification and UI tests. Human scope-quality evaluation uses explicit global/workspace/session, qualified, ambiguous and quoted statements; instruction quality is not a substitute for deterministic approval enforcement.

Background candidates use the explicit `memory-applicability-v1` compiler scope policy. A new generation must set `scope_policy` and add a required string `destination` property with exactly the `everywhere`, `workspace`, and `session` enum to the inline `properties.candidates.items` schema. The runtime rejects incompatible configuration before dispatch. Existing generation hashes, schemas, prompts and envelopes remain unchanged when the optional policy is absent. The scope-policy prompt is frozen under its version and checked by a digest regression; refinements require a new version. Configuring or activating a local extractor remains a separate operational step.

The review inbox stays in the source scope; its exact approved effect states applicability. Workspace evidence is not made Global by a Global claim. Mixed-applicability batches must be reviewed separately. Chat sends, as well as history reads, include the expected selected session ID; another tab selecting a different session makes the stale send fail before persistence.

Initial history pagination returns at most 100 display items per page, including terminal tool records. It uses the existing full-session event reader before projection; this bounds response item count, not database work or total bytes. Indexed event pagination is deferred until measured history size warrants that storage-interface change.

Manual scope-quality review, using a new disposable workspace and declining unwanted proposals:

| Statement | Expected recommendation |
| --- | --- |
| Remember I generally prefer concise answers. | Everywhere |
| I prefer readable answers with clear formatting. | Everywhere |
| In Finance, always show the calculations. | Workspace |
| General workspace notes should use short bullet points. | Workspace |
| For this conversation only, keep answers brief. | This conversation |
| For this debugging session, include detailed traces. | This conversation |
| I usually prefer concise answers, but include calculations for this workspace. | Two separately scoped statements |
| My coworker likes long explanations. | Preserve coworker attribution; do not infer David's preference |
| An example preference is “I like concise answers.” | Abstain without endorsement |
| What if I preferred very detailed answers? | Abstain |
| Keep this brief. | Narrow conversational scope; do not invent an enduring preference |
| I prefer concise answers everywhere, even when we discuss Finance. | Everywhere |

These cases assess model recommendations, distinct from deterministic tests that enforce destinations, approvals, source privacy and replay. No model-quality score is claimed until actual outputs are reviewed.

Memory provider implementation 1.1.0 retains exact 1.0.0 tool variants for pinned conversations through the existing compatibility-resolution mechanism. These variants preserve the old context-default destination and reject the new destination argument; receipts and accepted memories are never rewritten. Start a new conversation for direct scope recommendations. Frozen original schema hashes and a resume/new-composition regression verify this boundary. Failed session selections are visible in the main UI.
