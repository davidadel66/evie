# Development decision before held-out evaluation

The completed development run used `freeze-v3.json`: no numerical budget,
model, threshold, HNSW parameter, or selection gate changed after the first
freeze. Two earlier attempts aborted on experiment-broker integration errors,
recorded in the final decision record. No held-out comparison has run at the
time this record is written.

Both dense configurations recovered all 16 development paraphrase targets;
the shipped lexical/exact path recovered 6/16. Their lexical-control recall
was also 16/16. Full query-to-eligible-evidence p95 was 27.173084 ms for SQLite
dense and 25.226458 ms for HNSW dense; hybrid p95 was 30.271583 ms and 29.211 ms.

The provisional production candidate is all-minilm:22m with normalized
384-dimensional float32 vectors retained in SQLite and bounded brute-force
cosine scoring. HNSW did not clear the frozen requirement for a 20% reduction
in full-query p95, so its additional dependency is not justified at this
fixture scale. This decision remains conditional on the already frozen
held-out gates, current-access checks, and operational checks. The held-out
run will use the exact same binary and configuration without tuning.
