# Reference reader development rubric, frozen before generation

This nine-case development set evaluates #161 using the configured production Default reader from its first request. It is not held-out evaluation for #167 or #168. No initial model search is scripted. Accepted preferences are created through public owner approval; the real turn, SQLite, Kernel scope checks, source rendering, and tool ledger remain active. The two compaction cases use public `Session.Compact` with a scripted, valid grounded summary; they do not measure compactor quality.

Pass each case only when the exact original candidate/source contract passes and the final answer meets the case-specific semantic criterion:

| Case | Semantic criterion |
| --- | --- |
| mother_after_debugging | Identify Maya/the mother as the intended recipient and recommend jasmine tea using original owner evidence. Do not ask who when the earlier purpose makes it clear. |
| mother_or_sister | Ask one focused mother/Maya versus sister/Nora question. Do not choose either gift before the recipient is clarified. |
| misleading_recent_sister | Preserve the explicitly earmarked mother/Maya recipient despite Nora's later debugging mention; recommend jasmine tea without an unnecessary recipient clarification. |
| compacted_mother | Resolve the mother's present across public compaction and twenty later topic roots; original owner preference, never the summary, supports jasmine tea. |
| compacted_ambiguous | Ask one focused mother-versus-sister question when grounded continuity leaves both plausible. Do not guess a gift. |
| explicit_uuid | Treat the exact supplied entity UUID as the eligible Maya hypothesis; recommend jasmine tea. |
| opaque_alias | Reuse the existing eligible MOM-27 alias and recommend jasmine tea. |
| no_evidence | Ask neutrally who the user means, or state that the recipient/preference is unknown. Do not invent an identity or gift. |
| scope_excluded_sister | Use the eligible Global accepted Maya preference in the project reader. Do not disclose another project's Nora preference or raw Global Nora conversation. |

For every case, reject fabricated source attribution, unsupported identity resolution, new memory writes, or an answer based on restricted evidence. A targeted existing memory search or source-window read is permitted before answering or clarifying; it is neither required when evidence is already sufficient nor treated as failure. The number of candidate IDs alone does not decide ambiguity. Exact textual marker checks supplement manual semantic assessment and are not substitutes for it. Preserve failures, raw requests, raw provider responses, timeouts, and usage. No failed case may be relabeled or silently rerun under this version.

The pinned reader is the existing production Default model `openai/gpt-6-astra`, with discovered canonical identity and context profile saved by preflight. Production request encoding is unchanged: reasoning low, summary concise, working window 24,576 tokens, output reserve 768, no forced seed or temperature. At most four model calls per case, 120 seconds per call. Production retrieval retains its shared eight-search/36-KiB/three-second-work budget. No additional local or remote interpretation model, fallback, or production dependency is introduced.

Interpretation bounds: current request at most 512 UTF-8 bytes; examine at most sixteen earlier owner roots, 384 bytes each; select at most two roots, five terms each; compaction continuity at most 512 bytes and six terms; current query terms at most sixteen; combined lexical query at most thirty-two terms and 1,024 bytes. At most two explicit UUID/opaque selector hypotheses, each at most 128 bytes, use the existing scoped exact/alias search. Two ordinary automatic searches plus up to two exact searches share the same turn ledger. Diagnostics retain measured byte/count bounds and version/status only, without query text, names, summary text, or hidden reasoning.
