# F06 - Shell and process runtime

Status: candidate, unapproved. Priority: first.

## Purpose

Run real project commands with correct cwd, cancellation, output handling, and
ownership while avoiding false sandbox claims.

## How OpenCode does it

OpenCode runs cancellable shell processes, tracks output, parses command/path
intent for permissions, and integrates the process with session/tool events.
Its shell still executes under the user's ordinary environment. Permission
parsing reduces accidental misuse but does not contain a hostile command.

Source: [`tool/shell.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/tool/shell.ts#L338-L414).

## EVIE today

EVIE's Bash tool has strong practical choices worth preserving: login-shell
environment, startup snapshot, persistent cwd, combined output, useful nonzero
exit results, timeout, `WaitDelay`, and output spill. However:

- cwd is package-global rather than session-owned;
- `context.Background()` prevents turn cancellation;
- output is buffered until process exit;
- spill names use the process ID and can collide across calls;
- the shell inherits loaded secrets;
- the tool is unrestricted and ungated by explicit prior decision.

## Proposed EVIE adaptation

- Move cwd into F01 project/session state.
- Pass the turn's `context.Context` through execution.
- Assign every process an execution ID and unique spill path.
- Kill the process group/tree on cancel or timeout, then record partial output.
- Keep nonzero exit as a normal result; distinguish timeout, cancellation, and
  start failure.
- Redact known secrets from model-visible and persisted output.
- Emit bounded live output events when the frontend is attached; retain the
  complete output under F04's user-only, unique, symlink-safe cleanup policy.
- Keep interactive stdin and PTY support out of the first version.

Restricted profiles must not receive this unrestricted tool. A future
read-oriented command runner would need an explicit command allowlist and still
would not be an OS sandbox.

## Acceptance evidence

- Cancelling a turn stops descendants and closes output pipes promptly.
- Two sessions maintain independent cwd and concurrent spill files.
- Partial timeout/cancel output remains visible and accurately labeled.
- A verbose command cannot overflow model context.
- Secrets known to EVIE are redacted before persistence and provider return.
- Login-shell startup and the captured aliases/functions/PATH remain available;
  capture failure still degrades to a plain login shell.
- `cd` persists only within its owning session, and an explicit cwd becomes that
  session's next cwd.
- Stdout/stderr stay interleaved, nonzero exits remain normal results with an
  exit status, and timeout remains distinct from command failure.
- Default/maximum timeouts and `WaitDelay` still prevent descendant-held pipes
  from defeating the deadline.
- Inline output remains capped and its complete `0600` spill is reachable
  through the reported scoped path.

## Open decisions

1. Revisit the standing ungated-Bash decision for project-development sessions,
   or preserve it only for the attended personal profile?
2. Is command streaming needed before long-running builds become a priority?
3. Should tools receive a reduced environment rather than every loaded secret?
