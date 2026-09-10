# Subagents tickets

Parent specification: [#155](https://github.com/davidadel66/evie/issues/155). All seven tickets are published with the ready-for-agent label and verified native blocking links.

1. **[Compose and reopen the restricted research Agent Preset (#169)](https://github.com/davidadel66/evie/issues/169)**
   - **Blocked by:** None (can start immediately).
   - **What it delivers:** A research session receives exactly the Web capabilities selected by its preset, and reopening its receipt preserves those restrictions.

2. **[Run delegated sessions with isolated assignment context (#170)](https://github.com/davidadel66/evie/issues/170)**
   - **Blocked by:** None (can start immediately).
   - **What it delivers:** A child receives its worker role, selected assignment context, and its own history, with no automatic parent history, memory recall, or Task projection.

3. **[Exclude delegated assignments from owner-memory compilation (#171)](https://github.com/davidadel66/evie/issues/171)**
   - **Blocked by:** None (can start immediately).
   - **What it delivers:** Memory processing cannot mistake agent-authored assignments or child findings for owner assertions.

4. **[Complete one bounded durable foreground research assignment (#172)](https://github.com/davidadel66/evie/issues/172)**
   - **Blocked by:** [#169](https://github.com/davidadel66/evie/issues/169), [#170](https://github.com/davidadel66/evie/issues/170), [#171](https://github.com/davidadel66/evie/issues/171).
   - **What it delivers:** A parent delegates through the real composed runtime, one child researches, and persisted findings return to the parent without duplicate execution.

5. **[End child execution when parent authority ends (#173)](https://github.com/davidadel66/evie/issues/173)**
   - **Blocked by:** [#172](https://github.com/davidadel66/evie/issues/172).
   - **What it delivers:** Cancellation, lost ownership, revocation, and shutdown stop child activity while preserving whichever terminal outcome was durably accepted.

6. **[Recover interrupted assignments and replay retained findings (#174)](https://github.com/davidadel66/evie/issues/174)**
   - **Blocked by:** [#172](https://github.com/davidadel66/evie/issues/172).
   - **What it delivers:** Restart preserves completed findings, marks abandoned unfinished attempts interrupted, and never silently reruns a child.

7. **[Enable foreground research through the Subagents Plugin (#175)](https://github.com/davidadel66/evie/issues/175)**
   - **Blocked by:** [#173](https://github.com/davidadel66/evie/issues/173), [#174](https://github.com/davidadel66/evie/issues/174).
   - **What it delivers:** New eligible CLI/web conversations can delegate through the compiled Plugin; existing receipts stay unchanged and disable/shutdown reaches active workers.

## Execution order

Start #169, #170, and #171 independently. Then complete #172. Tickets #173 and #174 may proceed in parallel; #175 follows both.

Workspace success remains a separate follow-up on reviewed preset allowances from [#71](https://github.com/davidadel66/evie/issues/71). The Global/project batch is not blocked by the complete Workspace feature.

The parent issue body, title, labels, and state were not changed during ticket publication. Each child references the parent and its blockers; execution dependencies use native GitHub blocking links.
