# A03 - Explore agent

Status: candidate, unapproved. Kind: foreground subagent first.

## Purpose

Search a codebase quickly and return a compact factual map while keeping noisy
discovery work out of the parent agent's context.

## How OpenCode does it

OpenCode's `explore` profile starts from deny-all and then allows glob, grep,
list, read, web search/fetch, Bash, and approved external directories. Its
prompt says never to modify files and asks the caller to specify `quick`,
`medium`, or `very thorough` exploration.

The same limitation as plan mode applies: Bash makes the no-mutation rule a
prompt promise rather than a hard boundary.

Sources:

- [`Agent` registry, `explore`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/agent/agent.ts#L196-L218)
- [`explore.txt`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/agent/prompt/explore.txt)

## Proposed EVIE adaptation

Make `explore` the first shipped child profile, with depth zero and a
mechanically read-only toolset:

- project-scoped file list, glob, grep, paginated read, and Git read operations;
- optional web search/fetch, clearly separated from trusted project evidence;
- no unrestricted Bash, edit, write, database mutation, cron, or subagent tool;
- one immutable project root inherited from the parent;
- a caller-selected thoroughness level that affects search breadth, not
  permissions;
- a compact final response with exact paths, symbols, and unresolved gaps.

Foreground execution should return only the final findings to the parent. The
child's complete transcript remains inspectable in durable history.

## Output contract

- Answer the assigned question, not a general repository tour.
- Cite exact project-relative paths and key symbols.
- Separate observed facts from inference.
- Report what was not inspected when scope or budget ran out.
- Do not recommend edits unless the caller asked for design analysis.

## Acceptance evidence

- No exposed tool can mutate project or external state.
- The child cannot leave the parent's project scope.
- Parent cancellation cancels the child.
- Large search results stay in the child transcript; the parent receives a
  bounded findings summary.
- Resuming by task ID continues the same child history.

## Open decisions

1. Does EVIE reverse the prior decision against typed glob/grep for this
   profile? Recommendation: yes, because mechanical read-only isolation and
   context savings are new concrete needs.
2. What token/result budget corresponds to each thoroughness level?
