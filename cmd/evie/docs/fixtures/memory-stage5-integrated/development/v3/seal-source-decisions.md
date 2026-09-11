# Bind release sealing to the verified source commit

Before development v3 is frozen, `seal_release` now verifies the source bytes
behind the #167 commit marker. Previously, a commit with `#167` in its subject
could be recorded as the pilot source commit even when one of the inputs used
to build the frozen executable had changed or disappeared from that commit.

After verifying the development freeze and independently recomputed passing
development report, sealing resolves `HEAD` once. It checks the issue subject
on that immutable commit, then loads each path in the already hash-verified
`compiled-source-manifest.json` through `git show <commit>:<path>`. Every byte
hash must match the frozen manifest. Missing or changed Go, module, or embedded
inputs fail before the release output directory or prepared inputs are created.
An empty source manifest also fails. Sealing records that same verified commit
ID; a later movement of `HEAD` cannot substitute an unchecked commit ID.

Documentation and artifact additions outside the compiled manifest remain
permitted. The check reads committed blobs, so unrelated uncommitted work is
preserved and cannot stand in for committed source. Existing freeze hashes,
development report verification, issue-commit requirement, modes, schemas,
evaluation configuration and gates remain unchanged.

## Protocol verification

From the implementation worktree:

```sh
python3 -B cmd/evie/docs/fixtures/memory-stage5-integrated/development/v3/seal_source_regressions.py
```

The meaningful RED used a real temporary Git repository with changed compiled
source committed under a #167 subject: old sealing succeeded, so the expected
rejection failed (`RuntimeError not raised`, one test, 0.100 seconds). After
the source check, the same test passed (0.103 seconds).

The complete source-chain suite passed **8 tests in 0.943 seconds**. It covers
changed Go source, missing embedded input, changed module/embedded bytes,
documentation-only additions, uncommitted worktree changes, later `HEAD`
movement, empty manifests, and the existing issue-subject requirement. The
existing development-report sealing controls also passed **8 tests in 0.015
seconds** with the new runner.

These are synthetic protocol controls, not measurements. The source boundary
uses actual Git commits and raw committed bytes. Prior expensive freeze/report
validation and preparation are explicit fixture stubs. No model, embedding,
evaluation question, reader answer, or claimed passing quality result is
created. The fixture output is discarded with its temporary directory. An
initial fixture path mismatch was corrected before the meaningful RED above.

`seal-source-red-green-record.json` retains the meaningful RED, first GREEN,
complete source-suite GREEN, and existing report-suite GREEN transcripts. No
frozen v1/v2 files or historical results were edited.
