# Ticket149 public startup contention diagnosis

Root applied diagnosing-bugs workflow to the concrete144 observation. This is an isolated prototype contribution, not yet production code or a completed149 story.

## Reproduction

Public OpenDBAtContext, exact initial146 tree fae9bdd86e9e0f0f2f42bac18d0cbc063684412c, real temporary SQLite, synchronized independent child processes, no preopened connections.

- 12 waves x4processes, fresh then existing:96starts,3fresh failures: `create schema: database is locked (5) (SQLITE_BUSY)` in0–1ms. See149-startup-probe-results.json and149-startup-probe.go.
- Minimized to2processes x40fresh waves: public80starts/14failures; sql.Open+Ping using identicalDSN with no application schema80starts/12failures. See149-startup-minimal-results.json/149-startup-minimal.go. Thus application DDL/migrations are not necessary for this bug.

## Ranked hypotheses and discriminating probes

1. Concurrent transition toWAL encounters lock-upgrade contention bypassing ordinary busy waiting. Prediction: omitWAL or initializeWAL before competing opens removesfailure.
2. Busy timeout applied afterWAL. Prediction: actualdriverordersPRAGMAs incorrectly. Pinned modernc.org/sqlite v1.53.0 applyQueryParams explicitly sorts busy_timeout first, ruling thisout.
3. Concurrent empty-filecreation. Prediction: precreateemptyfile removesfailure. Itdidnot.

40 two-process waves each: precreatedempty80starts/1failure; omitWAL80/0; uncontendedpreinitializedWAL80/0. All are disposabletmpfiles. See149-startup-hypothesis-results.json/149-startup-hypothesis.go. These isolate connection initialization, not a migration retry.

## Proposed fix and green original loop

149-startup-proposed-helper.go adds connectSQLiteStartup(ctx,db), a five-second total bounded loop over db.PingContext before any application statement. Only a typedSQLiteprimaryBUSY(code5) retries; all other errors propagate. Cancellation terminates between connection attempts and is retained withthelastcodederror. Ten-millisecond delay, boundedbyremainingdeadline; no schema/appoperation reexecution. Theexisting driverper-connectionpragmas stayunchanged.

149-startup-proposed-db.go is a prototype entirefile basedoninitial146, ONLY forreviewingthenarrowinsertion beforedb.ExecContext(schema). DoNOToverwrite live db.go:148nowownsnew diagnostics hook. Rootwillnarrowlyintegrate149whenownerready andpreserve148/anyuserbytes.

`python3 .scratch/memory-stage-4/149-startup-retry-recheck.py`:320actualpublicstarts(40waves x4processes, fresh+existing),0failures. See149-startup-retry-results.json. Modifiedonlytheoldroot-owned146prototypearchive at149-startup-probe-location.json, NOT live source orfinal146snapshot. Initial146fullverification doesnotdescribe thismodifiedprototype; final146is97e9768e elsewhere.

## Remaining149 implementation/verification

Fresh149owner should review/refine helper, addrealpublicmulti-processregression plus deterministic cancellation/nonBUSY/timeoutboundarycoverage, andrunfull149conformanceafterintegration. Preserve exactcodederroridentity; doNOTretry whole schema/migration/acceptedoperation afteranuncertainwrite. Publicread-only behavior/per-connectionsettings andexistingfilemode/cancellationtests must remaincorrect. Themodification doesnotprove every possible SQLite contention scenario. Root will own live db.go integration, frozenbefore/afterhashes andone149commit withthefullacceptancestory.
