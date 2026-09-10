## Parent

https://github.com/davidadel66/evie/issues/154

## What to build

Evie reads neighboring messages when a short search hit needs context.

## Acceptance criteria

- [ ] Allow the model to request bounded neighboring messages anchored to an eligible retrieved source; validate the anchor and effective scope in the Kernel rather than trusting caller-supplied IDs.
- [ ] Apply the same source, authority, retirement, egress, and secret rules to every expanded passage. Expansion cannot bypass a suppressed interval or another conversation area's boundary.
- [ ] Account for cumulative expansion and serialized context costs, return explicit truncation/exhaustion information, and avoid returning whole conversations by default.
- [ ] Extend the existing evidence receipt and Conversation excerpt activity with the exact additional evidence supplied and inspectable source positions.
- [ ] Demonstrate a pronoun or tentative statement that becomes interpretable with neighboring messages while preserving attribution and without accepting memory.
- [ ] Verify forged/out-of-scope anchors, overlap and duplicate requests, UTF-8 locator boundaries, retired neighboring evidence, unrelated eligible passages, cancellation, restart inspection, and provider request bounds. Run required verification.

## Blocked by

- https://github.com/davidadel66/evie/issues/157
