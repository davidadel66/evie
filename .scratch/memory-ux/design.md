# Memory UI design

Brief: David should recognize his memories, open them, and review suggestions without reading instructions. Keep the existing Evie shell and remove redundant explanation. No scope merging or accepted-state changes.

Tokens: app #0e1113; surface #151b1d; selected #222a2c; text #e2e6e3; secondary #8b9491; action #4fb8a5. Retain IBM Plex Sans, 13–14px body, 18px section titles, 24px detail titles. Reserve monospace for expanded identifiers. Left align content; maximum readable detail width 72 characters.

Layout:

Memory   [Memories] [Review]                 [More]
[General v]                    [List | Graph] [Refresh]
----------------------------------------------------
Keep answers concise; avoid data dumps.           >
David · Prefers concise responses
----------------------------------------------------
Think about readability and style…                >
David · Prefers readable, well-styled responses

A clicked row or graph value opens a full detail view with Back, readable value, person, workspace, and exact evidence. Technical history is collapsed. The graph remains an alternate relationship view. Global and named Workspaces are primary; conversation/project scopes remain explicitly accessible through advanced controls. Review retains exact effects, sources, error warnings and final approval; debugging and batch setup are secondary.

Critique: a graph surrounded by nested tabs and configuration would repeat the current problem. Revised the default to a compact full-sentence list and moved graph/records mechanics behind familiar controls. Retained Evie's existing palette because continuity, not rebranding, is the request. No decorative cards or explanatory subtitles. Existing canonical owner identity remains unchanged; configured display name David is presentation only.

Verified defects: literal graph nodes have no summary and are disabled; the global owner entity is incorrectly inspected using the workspace scope and returns 422. Each object must open using its own authorized scope; literal nodes must inspect their owning Claim.
