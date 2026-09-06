## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Run the standalone local-model and evaluation spike required by the parent. Author the necessary corpus and runnable scoring/report artifacts as part of the experiment, then record a measured extractor choice. This establishes extraction behavior and standalone resource costs; compiler foreground overhead and real owner-review burden remain for the integrated pilot.

## Acceptance criteria

- [ ] Create synthetic Evie-shaped source windows and complete histories with human-reviewed labels and evidence. About 32 targeted windows plus 10–20 histories is a starting hypothesis; record the actual size and coverage rationale rather than treating those numbers as acceptance thresholds.
- [ ] Cover useful/no-memory cases, attribution and polarity, identity, change/conflict, time, evidence eligibility, scope/authority, and window continuity. Include optional and unwanted-but-true proposals, ambiguity, and supported equivalence.
- [ ] Freeze projected evidence, source events, accepted context, scope/authority, gold source locations, and uncertainty. Separate development, pilot/model-selection, and final holdout data by complete narrative and variant lineage. Define holdout curation, human annotation, custody and exposure records; keep final contents unavailable to model/prompt selection and pilot tuning until configuration and gates are frozen. Evaluator labels and future questions/answers never enter extractor input.
- [ ] Compare a bounded, recorded set of plausible local configurations on development data with repeated runs. Pin model artifact/digest and quantization, runtime/server, tokenizer/context, structured schema, prompt, decoding, evidence policy, corpus hashes, environment, and repetition protocol.
- [ ] Exercise malformed/truncated output, input/output bounds, timeout/cancellation, unavailable local endpoints, redirect/nonlocal rejection, and no remote fallback. Distinguish client return, simulated late-effect prevention, and actual server capacity release.
- [ ] Score raw proposals and proposals retained by the spike’s declared schema/source checks separately: supported useful precision, required-memory recall, identity and temporal errors, source attribution, typed meaning/polarity/Predicate agreement, unwanted suggestions, and omissions. This standalone validation result does not require the production compiler; integrated evaluation measures actual persisted reviewable candidates. Exact source matching is not entailment; record human meaning/usefulness adjudication.
- [ ] Produce a versioned report with counts, denominators, failures, uncertainty where meaningful, comparison baseline, and standalone latency/memory/resource results. Record limitations and keep final holdout unexposed.
- [ ] Record the selected local runtime/model/schema and why it is adequate to proceed, or explicitly report that no tested configuration is usable and keep production extractor selection blocked. Do not force a winner or invent quality thresholds.
- [ ] Keep private-history extraction local; any optional remote comparison uses synthetic data only. Add no production dependency without the repository-required approval. Verify reproducibility and run the checks applicable to any executable or documentation changes.
- [ ] Completion requires accessible local inference hardware/runtime and actual human-reviewed labels. Record missing access or annotation as incomplete work; scripted extraction, agent self-review or invented measurements cannot substitute for these observations.

## Blocked by

- [Freeze evidence, source-window, and closure rules](https://github.com/davidadel66/evie/issues/132)
