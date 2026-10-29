# System explorer decisions

Spec: [system-explorer.spec.md](system-explorer.spec.md).

- **2026-10-28 — Diagrams are hand-built SVG in vanilla JavaScript, with no
  diagram library.** The brief proposed vendoring Mermaid 11.17.2. David
  answered "i dont love mermaid. if you can use js or three.js to make things
  better and more intuitive that would be great. or the hand drawn svg."
  Claude chose two-dimensional SVG drawn by the page over three.js, because
  three-dimensional graphs make architecture harder to read and three.js
  would be a new dependency, and chose purpose-built layouts per view (lanes,
  layered group views, a three-column focus view, stepped sequence diagrams)
  over a general graph-layout engine such as dagre, because they read more
  intuitively at Evie's size and add no dependency.
- **2026-10-28 — The drift check and the viewer's Node tests become required
  steps of `./scripts/verify-change.sh`.** David approved this in the brief
  ("everything else looks good"), accepting that any change adding or
  removing a package, import edge, command, route, table, glossary term, or
  ADR fails verification until `go run ./scripts/explorer extract` is rerun,
  and that a new package needs a node with at least a one-line cited
  summary. `AGENTS.md`'s Verification section is updated to match.
- **2026-10-28 — The explorer data format is the contract `/flow` will write
  against.** David approved it in the brief: strict JSON inside a fixed
  one-line classic-script wrapper, one file per owner (generated facts, one
  written-content file per subsystem, one file per design proposal), all
  listed in a manifest, versioned, with recursive `children` reserved for
  the later depth pass.
- **2026-10-28 — Two-way doors from the brief, approved as a set.** David
  approved the brief's decided items ("everything else looks good"):
  generated facts kept apart from written content; tables read from each
  database's real migrations on a throwaway file rather than parsed from
  SQL strings; import edges as the union across build constraints so macOS
  and Linux extract the same facts; routes from `Handle`/`HandleFunc` calls
  with range-over-literal expansion and loud failure otherwise; commands from
  argument comparisons with a golden test; glossary definitions and ADR
  titles copied into the facts so the page shows them from `file://`;
  references without line numbers, with Go symbols and headings checked;
  "statement" rather than "claim" and "design proposal" rather than bare
  "proposal" to keep clear of glossary terms, with no glossary entries added;
  a three-step components view (group overview, group, focus); a no-build,
  classic-script page; relative code links into the checkout; the tool under
  `scripts/explorer`; this spec under `cmd/evie/docs/active/`; the component
  grouping listed in the spec; the original memory diagrams left untouched
  and linked.
- **2026-10-28 — The ticket split from the brief stands.** David approved
  nine tickets: a tracer bullet, command/route/table extraction, the full
  viewer, and six content tickets by subsystem. Replacing Mermaid with
  hand-built SVG changes what the tracer bullet and viewer tickets render,
  not the split or its blocking edges.
- **2026-10-28 — Out of scope: changing `/flow` or `/implement`, and depth
  beyond one level.** David's request; both are done separately or later.
