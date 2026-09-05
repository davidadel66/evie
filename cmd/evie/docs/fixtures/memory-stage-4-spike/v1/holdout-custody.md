# Final holdout custody protocol

Status: protocol only; no final-holdout contents exist in this spike directory and none were authored or inspected by the tuning agent.

A separate curator assigned by David must author complete synthetic histories and variants in a location unavailable to the tuning task. David reviews their source evidence, labels, uncertainty, and completeness. The custodian records corpus/file hashes, reviewer identity and timestamp, narrative lineage assignments, and every access/exposure in a custody log. Development and pilot families N01–N12 are prohibited from final holdout; simple renaming or paraphrasing does not create a new lineage.

The tuning task receives only readiness, counts, hashes, and coverage metadata. It does not receive cases, gold labels, future questions, outputs, or feedback from final scoring. Before a single final exposure, freeze runtime/model artifacts, prompt, schema, decoding, evidence policy, scoring/adjudication rules, and numerical release gates established by the later integrated pilot. This standalone spike cannot invent those gates.

The custodian runs the frozen configuration once under the recorded repetition protocol, logs exposure before execution, and preserves raw results for human adjudication. Failed or interrupted attempts remain in the exposure log; do not tune and call a rerun the same holdout. Any accidental exposure retires the affected narrative family and requires fresh independently curated final data. No final run is authorized by creating this protocol.
