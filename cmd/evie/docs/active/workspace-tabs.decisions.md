# Workspace tabs decisions

Spec: [workspace-tabs.spec.md](workspace-tabs.spec.md).

- **2026-10-12 — Tabs are real pages, sealed by the browser sandbox.**
  David chose Evie-written HTML and JavaScript over a configuration of
  Evie's own charts, so Evie can build any view on request. This amends
  `plugins.spec.md` ("without accepting untrusted executable code"): such
  code runs only in the sandboxed, opaque-origin, no-network frame the spec
  describes, never in Evie's process.
- **2026-10-12 — Custom Workspace UI is allowed for tabs only.** An
  exception to `workspaces.spec.md` ("Plugins do not inject custom Workspace
  administration UI"). Tabs come from the Workspace's folder, not plugins.
  Settings, access and presets stay Kernel-owned.
- **2026-10-12 — Tabs need a Workspace folder.** A tab lives only in the
  Workspace's folder under `.evie/tabs/<tab-id>/`. A Workspace without a
  folder has no custom tabs; Finance gets a folder before its Reports tab.
- **2026-10-12 — Evie builds Finance Reports.** After the data channel and
  Evie's loop (stories 2 and 3), Evie writes a Reports tab in the Finance
  folder on the finance database. Data → Spending stays until the owner
  decides otherwise.
- **2026-10-12 — WebRTC is an accepted residual risk, with a partial block.**
  No page policy can stop a tab's code from signalling out over WebRTC
  (STUN/TURN over DNS or UDP). David accepted that risk for tabs that read
  data, relying on his approval of each tab's exact data list and on Evie's
  tab code being reviewable, and asked for a partial block as defence in
  depth: every tab page starts with a script that removes the WebRTC
  constructors and the ways to reach a fresh window before the tab's own
  code runs. It is not a seal; a determined page may find another way.
