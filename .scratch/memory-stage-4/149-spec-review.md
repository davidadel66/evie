# #149 independent Spec review

Verdict: no remaining actionable Spec findings.

Reviewed the combined tree `ce6ead508ffd65691258abae858ca34d839100e9`
against base `9d697bf57c7835d7522da391c44163e15f416189`, then the single
test-only isolation refinement frozen in
`149-owner-refined-frozen-files/scripts/memory_stage4_conformance_test.py`
(SHA256 `ed461ab89dd27785682faf6a3ee5fe967df280c9c15ecc886f8f627112aa5661`).
All other owner/root frozen files match that combined tree byte for byte.
Binding sources are published ticket #149 and the Stage 4 acceptance,
scope, persistence, replay, and verification requirements.

Both original findings are resolved:

- The browser helper binds actual returned operation IDs and preview/effect
  hashes to owner inspection. Active observers inspect accepted global,
  Workspace and project memory. Exact closed-session destinations use owner
  operation inspection, temporary projection assertions and canonical replay.
  The original sources remain closed; the helper no longer depends on an
  active closed-source context or primary-candidate recurrence resolution.
- Generic-storage conformance uses the actual query tool with an isolated
  read-only fixture. Two allowed controls succeed; 308 direct, nested, quoted
  and qualified queries across 77 protected tables/views require exact policy
  denials and zero database opens. Unrelated failures cannot count as
  containment. The trusted-reader constructor retains the default reader and
  unchanged SQL policy. The targeted mutation confirms the enhanced test
  detects the regression missed by the original.

The final isolation repair is appropriate: a scoped `patch.dict` binds
`GIT_DIR`, `GIT_COMMON_DIR`, `GIT_WORK_TREE`, and `GIT_INDEX_FILE` to the fixture's
temporary repository before every Git call, then restores the inherited
environment. Source-fingerprint assertions and the production runner remain
unchanged. Independently ran the frozen six-test script normally and with all
four inherited paths targeting a separate sentinel directory: both passed,
and the external destinations remained untouched.

Focused normal/race evidence previously inspected contains eleven scenarios
per run and zero failures. Final source-bound browser/full verification is
root-owned and must pass before conformance completion; this review does not
substitute for those checks or establish model quality or pilot readiness.
