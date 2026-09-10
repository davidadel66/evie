## Parent

https://github.com/davidadel66/evie/issues/154

## What to build

Ordinary requests receive a small relevant selection of accepted memory and conversation evidence automatically.

## Acceptance criteria

- [ ] Invoke the shared retrieval boundary at the start of each new user message without a remember command, using a bounded representation of current and relevant earlier conversation/summary context rather than a hardcoded latest-message-only query.
- [ ] Select accepted memory and attributed Conversation Excerpts through the existing lexical/exact baseline; do not hardcode a list of personal preferences or require extraction to produce a Claim first.
- [ ] Inject bounded EVIE_MEMORY_DATA at the specified current-request position through the context composer, account for the complete serialized request, and retain the source references supplied for each request.
- [ ] Keep stable instructions and tool definitions consistent, preserve existing durable conversation events, and do not persist synthetic recall blocks as new independent episodes.
- [ ] Revalidate source/state eligibility and egress before every provider dispatch; a continuation must remain correct even before the later optimization for evidence reuse is added.
- [ ] Show compact activity without narrating every ordinary lookup. Continue with Memory unavailable when current context suffices, or explain when missing memory prevents an answer; never present failure as successful absence.
- [ ] Demonstrate a saved preference and relevant uncompiled conversation fact in fresh chats, no useful match, distractors, inaccessible matches, opt-in off, failed lookup, and compaction continuity using real turns and separate model-backed quality cases. Run required verification.

## Blocked by

- https://github.com/davidadel66/evie/issues/157
