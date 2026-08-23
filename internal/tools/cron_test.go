package tools

// Acceptance tests for the cron feature, written from
// cmd/evie/docs/active/cron.spec.md before any implementation exists
// (red -> green). Compile-time contract these tests pin — the spec's
// Interfaces section, plus two seams the spec leaves unnamed (choices
// flagged in the test-writing report):
//
//	func parseSchedule(expr string) ([]calendarDict, error)
//	    // name chosen here: the spec mandates a parser/translator but
//	    // names no entry point. Tests never assume calendarDict's shape —
//	    // dict contents are asserted through plistFor's rendering.
//	func plistFor(label string, id int64, binPath string, dicts []calendarDict) []byte
//	var installJob func(_ context.Context, label, plistPath string, plist []byte) error
//	var uninstallJob func(_ context.Context, label, plistPath string) error
//	var openCronDB = eviedb.OpenDB
//	    // db seam, type func(_ context.Context) (*sql.DB, error): the spec gives eviedb
//	    // an openDBAt temp-path seam but names none for the tools layer;
//	    // this follows the fetchTimeout/braveSearchURL/installJob var
//	    // pattern. Tests replace it with an opener for a temp file.
//	func RunScheduled(command string, timeout time.Duration) (output []byte, exitCode int)
//
// The tool funcs themselves are driven through the registry by name
// (cron_add / cron_list / cron_remove), so their Go names stay the
// implementer's choice.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"

	_ "modernc.org/sqlite"
)

func TestCronAddCancellationAfterMutationRollsBackRow(t *testing.T) {
	db := newCronDB(t)
	originalInstall := installJob
	originalUninstall := uninstallJob
	t.Cleanup(func() {
		installJob = originalInstall
		uninstallJob = originalUninstall
	})
	ctx, cancel := context.WithCancel(context.Background())
	installStarted := make(chan struct{})
	installJob = func(ctx context.Context, _, _ string, _ []byte) error {
		close(installStarted)
		<-ctx.Done()
		return ctx.Err()
	}
	uninstalled := make(chan struct{})
	uninstallJob = func(ctx context.Context, _, _ string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		close(uninstalled)
		return nil
	}
	result := make(chan error, 1)
	go func() {
		_, err := cronAdd(ctx, `{"name":"cancel-add","schedule":"0 9 * * *","command":"echo hi"}`)
		result <- err
	}()
	<-installStarted
	cancel()
	err := <-result
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cronAdd error = %v, want context.Canceled", err)
	}
	select {
	case <-uninstalled:
	default:
		t.Fatal("cronAdd did not reconcile launchd after effect-started cancellation")
	}
	if got := jobCount(t, db); got != 0 {
		t.Fatalf("jobs after cancellation cleanup = %d, want 0", got)
	}
}

func TestCronAddCancellationPreservesRowWhenLaunchdReconciliationFails(t *testing.T) {
	db := newCronDB(t)
	originalInstall := installJob
	originalUninstall := uninstallJob
	originalRun := runCronLaunchctl
	originalRemove := removeCronPlist
	t.Cleanup(func() {
		installJob = originalInstall
		uninstallJob = originalUninstall
		runCronLaunchctl = originalRun
		removeCronPlist = originalRemove
	})
	ctx, cancel := context.WithCancel(context.Background())
	installStarted := make(chan struct{})
	installJob = func(ctx context.Context, _, _ string, _ []byte) error {
		close(installStarted)
		<-ctx.Done()
		return ctx.Err()
	}
	reconcileErr := errors.New("launchd reconciliation failed")
	uninstallJob = reconcileCronJob
	runCronLaunchctl = func(context.Context, ...string) ([]byte, error) {
		return []byte("launchd state unavailable"), reconcileErr
	}
	plistRemoved := make(chan struct{}, 1)
	removeCronPlist = func(string) error {
		plistRemoved <- struct{}{}
		return nil
	}
	result := make(chan error, 1)
	go func() {
		_, err := cronAdd(ctx, `{"name":"uncertain-add","schedule":"0 9 * * *","command":"echo hi"}`)
		result <- err
	}()
	<-installStarted
	cancel()
	err := <-result
	if !errors.Is(err, context.Canceled) || !errors.Is(err, reconcileErr) {
		t.Fatalf("cronAdd error = %v, want cancellation joined with reconciliation failure", err)
	}
	if got := jobCount(t, db); got != 1 {
		t.Fatalf("jobs after uncertain launchd state = %d, want durable correlation preserved", got)
	}
	select {
	case <-plistRemoved:
		t.Fatal("plist removal ran without a successful bootout")
	default:
	}
}

func TestCronRemoveCancellationAfterMutationCompletesConsistencyCleanup(t *testing.T) {
	db := newCronDB(t)
	if _, err := db.Exec(`INSERT INTO jobs (name, schedule, command, created_at) VALUES ('cancel-remove', '0 9 * * *', 'echo hi', 'now')`); err != nil {
		t.Fatal(err)
	}
	original := uninstallJob
	t.Cleanup(func() { uninstallJob = original })
	ctx, cancel := context.WithCancel(context.Background())
	uninstallJob = func(context.Context, string, string) error { cancel(); return nil }
	_, err := cronRemove(ctx, `{"name":"cancel-remove"}`)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cronRemove error = %v, want context.Canceled", err)
	}
	if got := jobCount(t, db); got != 0 {
		t.Fatalf("jobs after cancellation cleanup = %d, want 0", got)
	}
}

func TestCronRemoveCancellationPreservesRowWhenUninstallFails(t *testing.T) {
	db := newCronDB(t)
	if _, err := db.Exec(`INSERT INTO jobs (name, schedule, command, created_at) VALUES ('cancel-uncertain-remove', '0 9 * * *', 'echo hi', 'now')`); err != nil {
		t.Fatal(err)
	}
	originalUninstall := uninstallJob
	originalRun := runCronLaunchctl
	t.Cleanup(func() {
		uninstallJob = originalUninstall
		runCronLaunchctl = originalRun
	})
	ctx, cancel := context.WithCancel(context.Background())
	uninstallErr := errors.New("launchd reconciliation failed")
	uninstallJob = reconcileCronJob
	runCronLaunchctl = func(context.Context, ...string) ([]byte, error) {
		cancel()
		return []byte("launchd state unavailable"), uninstallErr
	}

	_, err := cronRemove(ctx, `{"name":"cancel-uncertain-remove"}`)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, uninstallErr) {
		t.Fatalf("cronRemove error = %v, want cancellation joined with uninstall failure", err)
	}
	if got := jobCount(t, db); got != 1 {
		t.Fatalf("jobs after cancellation cleanup failure = %d, want durable correlation preserved", got)
	}
}

func TestCronRemoveOrdinaryUninstallFailureStillDeletesRow(t *testing.T) {
	db := newCronDB(t)
	if _, err := db.Exec(`INSERT INTO jobs (name, schedule, command, created_at) VALUES ('uncertain-remove', '0 9 * * *', 'echo hi', 'now')`); err != nil {
		t.Fatal(err)
	}
	originalUninstall := uninstallJob
	originalRemove := removeCronPlist
	t.Cleanup(func() {
		uninstallJob = originalUninstall
		removeCronPlist = originalRemove
	})
	uninstallErr := errors.New("launchd still active")
	uninstallJob = func(context.Context, string, string) error { return uninstallErr }
	plistRemovalAttempted := false
	removeCronPlist = func(string) error { plistRemovalAttempted = true; return nil }

	_, err := cronRemove(context.Background(), `{"name":"uncertain-remove"}`)
	if err != nil {
		t.Fatalf("cronRemove ordinary uninstall error = %v, want ignored", err)
	}
	if got := jobCount(t, db); got != 0 {
		t.Fatalf("jobs after ordinary failed uninstall = %d, want row deleted", got)
	}
	if !plistRemovalAttempted {
		t.Fatal("ordinary remove did not attempt plist cleanup after bootout error")
	}
}

func TestCronCleanupContextIsIndependentAndBounded(t *testing.T) {
	type parentKey struct{}
	parent, cancelParent := context.WithCancel(context.WithValue(context.Background(), parentKey{}, "parent"))
	cancelParent()

	cleanupCtx, cancelCleanup := newCronCleanupContext(parent)
	defer cancelCleanup()
	deadline, ok := cleanupCtx.Deadline()
	if !ok {
		t.Fatal("cleanup context has no deadline")
	}
	if err := cleanupCtx.Err(); err != nil {
		t.Fatalf("cleanup context inherited parent cancellation: %v", err)
	}
	if got := cleanupCtx.Value(parentKey{}); got != nil {
		t.Fatalf("cleanup context inherited parent value %v", got)
	}
	remaining := time.Until(deadline)
	if remaining <= cronCleanupTimeout-time.Second || remaining > cronCleanupTimeout {
		t.Fatalf("cleanup deadline remaining = %s, want approximately %s", remaining, cronCleanupTimeout)
	}
}

func TestReconcileCronJobSurfacesBootoutAndPlistFailures(t *testing.T) {
	originalRun := runCronLaunchctl
	originalRemove := removeCronPlist
	t.Cleanup(func() {
		runCronLaunchctl = originalRun
		removeCronPlist = originalRemove
	})

	t.Run("bootout failure", func(t *testing.T) {
		bootoutErr := errors.New("launchctl unavailable")
		runCronLaunchctl = func(context.Context, ...string) ([]byte, error) {
			return []byte("permission denied"), bootoutErr
		}
		removed := false
		removeCronPlist = func(string) error { removed = true; return nil }

		err := reconcileCronJob(context.Background(), "com.evie.cron.test", "/tmp/test.plist")
		if !errors.Is(err, bootoutErr) || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("reconcileCronJob error = %v, want bootout failure and output", err)
		}
		if removed {
			t.Fatal("plist was removed before launchd absence was established")
		}
	})

	t.Run("plist removal failure", func(t *testing.T) {
		runCronLaunchctl = func(context.Context, ...string) ([]byte, error) { return nil, nil }
		removeErr := errors.New("read-only filesystem")
		removeCronPlist = func(string) error { return removeErr }

		err := reconcileCronJob(context.Background(), "com.evie.cron.test", "/tmp/test.plist")
		if !errors.Is(err, removeErr) {
			t.Fatalf("reconcileCronJob error = %v, want plist removal failure", err)
		}
	})

	t.Run("missing plist is already reconciled", func(t *testing.T) {
		runCronLaunchctl = func(context.Context, ...string) ([]byte, error) { return nil, nil }
		removeCronPlist = func(string) error { return os.ErrNotExist }
		if err := reconcileCronJob(context.Background(), "com.evie.cron.test", "/tmp/test.plist"); err != nil {
			t.Fatalf("reconcileCronJob missing plist: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newCronDB creates a temp evie-shaped database, points the openCronDB
// seam at it (fresh connection per call, since tool funcs close what they
// open), and returns a separate assertion connection.
//
// Both connections go through eviedb.OpenDBAt — the same function
// production uses — so the schema and pragmas under test are the real
// ones. An earlier version of this helper hand-copied the DDL and opened
// with its own pragma string; the copy kept a foreign key that production
// had dropped, and "run history survives cron_remove" passed only because
// the seam's connection happened to have FK enforcement off. A test db
// shaped by anything other than the production opener can pass while
// production breaks.
func newCronDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evie.db")

	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatalf("test setup: open temp db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	original := openCronDB
	openCronDB = func(_ context.Context) (*sql.DB, error) { return eviedb.OpenDBAt(path) }
	t.Cleanup(func() { openCronDB = original })
	return db
}

// installCall records one installJob invocation.
type installCall struct {
	label     string
	plistPath string
	plist     []byte
}

// captureInstalls replaces the installJob seam with a recorder that
// succeeds, restoring on cleanup — same save/restore pattern as
// pointBraveAt in websearch_test.go.
func captureInstalls(t *testing.T) *[]installCall {
	t.Helper()
	var calls []installCall
	original := installJob
	installJob = func(_ context.Context, label, plistPath string, plist []byte) error {
		calls = append(calls, installCall{label: label, plistPath: plistPath, plist: plist})
		return nil
	}
	t.Cleanup(func() { installJob = original })
	return &calls
}

// uninstallCall records one uninstallJob invocation.
type uninstallCall struct {
	label     string
	plistPath string
}

func captureUninstalls(t *testing.T) *[]uninstallCall {
	t.Helper()
	var calls []uninstallCall
	original := uninstallJob
	uninstallJob = func(_ context.Context, label, plistPath string) error {
		calls = append(calls, uninstallCall{label: label, plistPath: plistPath})
		return nil
	}
	t.Cleanup(func() { uninstallJob = original })
	return &calls
}

// runCronTool drives a cron tool the way the dispatcher does — through
// the registry by name with raw arguments JSON — so the Go function names
// stay unpinned.
func runCronTool(t *testing.T, name string, params map[string]any) (string, error) {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("test setup: marshal args: %v", err)
	}
	for _, tool := range all {
		if tool.Schema.Function.Name == name {
			return tool.Execute(context.Background(), string(raw))
		}
	}
	t.Fatalf("%s is not in the tool registry", name)
	return "", nil
}

func jobCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	return n
}

func runCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM job_runs`).Scan(&n); err != nil {
		t.Fatalf("count job_runs: %v", err)
	}
	return n
}

// insertRun seeds a job_runs row directly, with an explicit id so tests
// can control the id-vs-started_at ordering.
func insertRun(t *testing.T, db *sql.DB, id, jobID int64, startedAt string, exitCode int64) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO job_runs (id, job_id, started_at, finished_at, exit_code, output)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, jobID, startedAt, startedAt, exitCode, "run output",
	); err != nil {
		t.Fatalf("insert run %d: %v", id, err)
	}
}

// mustRender parses a schedule and renders it through plistFor with a
// fixed label/id/binPath, so dict contents can be asserted as XML without
// the tests assuming calendarDict's internal shape.
func mustRender(t *testing.T, expr string) string {
	t.Helper()
	dicts, err := parseSchedule(expr)
	if err != nil {
		t.Fatalf("parseSchedule(%q): %v", expr, err)
	}
	return string(plistFor("com.evie.cron.t", 1, "/usr/local/bin/evie", dicts))
}

// firstBytes and lastBytes make failure messages safe on short outputs.
func firstBytes(b []byte, n int) string {
	if len(b) < n {
		n = len(b)
	}
	return string(b[:n])
}

func lastBytes(b []byte, n int) string {
	if len(b) < n {
		n = len(b)
	}
	return string(b[len(b)-n:])
}

// normalizePlist trims per-line whitespace and drops blank lines: the
// golden comparison pins content and ordering, not indentation style.
func normalizePlist(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// ---------------------------------------------------------------------------
// schedule parser/translator
// ---------------------------------------------------------------------------

func TestParseScheduleDictCounts(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want int
	}{
		// A `*` field contributes nothing to the cross-product, so the
		// all-wildcard expression is exactly one (empty) dict.
		{name: "all wildcards is one dict", expr: "* * * * *", want: 1},
		{name: "single minute", expr: "30 * * * *", want: 1},
		{name: "minute and hour", expr: "0 9 * * *", want: 1},
		{name: "comma list of singles", expr: "0,15,30,45 * * * *", want: 4},
		{name: "range expands", expr: "0 9-17 * * *", want: 9},
		{name: "step on star", expr: "*/15 * * * *", want: 4},
		{name: "list values dedupe", expr: "5,5,5 * * * *", want: 1},
		{name: "cross-product of two fields", expr: "0,30 8,12,18 * * *", want: 6},
		{name: "day of month alone", expr: "0 0 1 * *", want: 1},
		{name: "boundary values accepted", expr: "59 23 31 12 *", want: 1},
		{name: "sunday is zero", expr: "0 9 * * 0", want: 1},
		{name: "exactly 100 dicts is allowed", expr: "0-9 0-9 * * *", want: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dicts, err := parseSchedule(tt.expr)
			if err != nil {
				t.Fatalf("parseSchedule(%q) returned error: %v", tt.expr, err)
			}
			if len(dicts) != tt.want {
				t.Errorf("parseSchedule(%q) produced %d dicts, want %d", tt.expr, len(dicts), tt.want)
			}
		})
	}
}

// Dedupe happens BEFORE the cap is counted: 120 listed minutes that
// collapse to 60 unique values must pass, not trip the 100-dict cap.
func TestParseScheduleDedupesBeforeCap(t *testing.T) {
	var parts []string
	for i := 0; i < 60; i++ {
		parts = append(parts, strconv.Itoa(i), strconv.Itoa(i))
	}
	expr := strings.Join(parts, ",") + " * * * *"

	dicts, err := parseSchedule(expr)
	if err != nil {
		t.Fatalf("parseSchedule with duplicate list values errored: %v — dedupe must run before the cap is counted", err)
	}
	if len(dicts) != 60 {
		t.Errorf("got %d dicts, want 60 unique minutes", len(dicts))
	}
}

// Each restricted field lands under its launchd key; every `*` field is
// OMITTED from the dict (launchd treats a missing key as a wildcard).
func TestParseScheduleFieldTranslation(t *testing.T) {
	launchdKeys := []string{"Minute", "Hour", "Day", "Month", "Weekday"}

	tests := []struct {
		name     string
		expr     string
		wantKey  string
		wantInts []string
	}{
		{name: "minute", expr: "30 * * * *", wantKey: "Minute", wantInts: []string{"30"}},
		{name: "hour", expr: "* 5 * * *", wantKey: "Hour", wantInts: []string{"5"}},
		{name: "day of month", expr: "* * 15 * *", wantKey: "Day", wantInts: []string{"15"}},
		{name: "month", expr: "* * * 6 *", wantKey: "Month", wantInts: []string{"6"}},
		{name: "weekday", expr: "* * * * 3", wantKey: "Weekday", wantInts: []string{"3"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustRender(t, tt.expr)
			if !strings.Contains(got, "<key>"+tt.wantKey+"</key>") {
				t.Errorf("plist for %q is missing <key>%s</key>:\n%s", tt.expr, tt.wantKey, got)
			}
			for _, v := range tt.wantInts {
				if !strings.Contains(got, "<integer>"+v+"</integer>") {
					t.Errorf("plist for %q is missing <integer>%s</integer>:\n%s", tt.expr, v, got)
				}
			}
			for _, key := range launchdKeys {
				if key == tt.wantKey {
					continue
				}
				if strings.Contains(got, "<key>"+key+"</key>") {
					t.Errorf("plist for %q carries <key>%s</key> for a * field — wildcards must be omitted:\n%s", tt.expr, key, got)
				}
			}
		})
	}

	t.Run("all wildcards renders an empty calendar dict", func(t *testing.T) {
		got := mustRender(t, "* * * * *")
		if !strings.Contains(got, "StartCalendarInterval") {
			t.Fatalf("plist is missing StartCalendarInterval:\n%s", got)
		}
		for _, key := range launchdKeys {
			if strings.Contains(got, "<key>"+key+"</key>") {
				t.Errorf("plist for * * * * * carries <key>%s</key>, want an empty dict:\n%s", key, got)
			}
		}
	})

	t.Run("cross-product carries every combination's values", func(t *testing.T) {
		got := mustRender(t, "0,30 8,18 * * *")
		for _, v := range []string{"0", "30", "8", "18"} {
			if !strings.Contains(got, "<integer>"+v+"</integer>") {
				t.Errorf("cross-product plist is missing value %s:\n%s", v, got)
			}
		}
		if n := strings.Count(got, "<key>Minute</key>"); n != 4 {
			t.Errorf("cross-product plist has %d Minute keys, want one per dict (4):\n%s", n, got)
		}
	})
}

func TestParseScheduleErrors(t *testing.T) {
	tests := []struct {
		name       string
		expr       string
		errMustSay string // lowercase substring; empty = any error
	}{
		// Joint list/range/step rules.
		{name: "step on a range is rejected", expr: "1-10/2 * * * *", errMustSay: "*/"},
		{name: "range embedded in a list is rejected", expr: "1-5,30 * * * *"},
		{name: "step on a single number is rejected", expr: "5/2 * * * *"},

		// Names are not supported — numbers only.
		{name: "weekday name is rejected", expr: "0 9 * * mon", errMustSay: "number"},
		{name: "month name is rejected", expr: "0 0 1 jan *", errMustSay: "number"},

		// Restricting both dom and dow: vixie ORs, launchd ANDs — refuse.
		{name: "both dom and dow is rejected", expr: "0 9 1 * 1", errMustSay: "day"},

		// Per-field range validation.
		{name: "minute 60 is out of range", expr: "60 * * * *"},
		{name: "hour 24 is out of range", expr: "* 24 * * *"},
		{name: "day of month 0 is out of range", expr: "* * 0 * *"},
		{name: "day of month 32 is out of range", expr: "* * 32 * *"},
		{name: "month 0 is out of range", expr: "* * * 0 *"},
		{name: "month 13 is out of range", expr: "* * * 13 *"},
		{name: "weekday 7 says use 0 for sunday", expr: "0 9 * * 7", errMustSay: "use 0 for sunday"},
		{name: "range endpoint out of range", expr: "0 9-25 * * *"},

		// The 100-dict cap tells the model to simplify.
		{name: "spec example over the cap", expr: "*/2 */2 * * *", errMustSay: "simplif"},
		{name: "101+ dicts is over the cap", expr: "0-9 0-10 * * *", errMustSay: "simplif"},

		// Malformed expressions.
		{name: "four fields", expr: "* * * *"},
		{name: "six fields", expr: "* * * * * *"},
		{name: "empty expression", expr: ""},
		{name: "whitespace only", expr: "   "},
		{name: "garbage tokens", expr: "a b c d e"},
		{name: "garbage text", expr: "not a cron"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dicts, err := parseSchedule(tt.expr)
			if err == nil {
				t.Fatalf("parseSchedule(%q) succeeded with %d dicts, want an error", tt.expr, len(dicts))
			}
			if tt.errMustSay != "" && !strings.Contains(strings.ToLower(err.Error()), tt.errMustSay) {
				t.Errorf("parseSchedule(%q) error = %v, want it to mention %q", tt.expr, err, tt.errMustSay)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// plist generation
// ---------------------------------------------------------------------------

// The generator takes the binary path as a parameter precisely so this
// golden comparison can use a fixed path instead of the test binary's
// temp build path. Representative job: two calendar dicts from a
// restricted-weekday schedule.
func TestPlistForGolden(t *testing.T) {
	dicts, err := parseSchedule("0 9 * * 1,4")
	if err != nil {
		t.Fatalf("parseSchedule: %v", err)
	}
	if len(dicts) != 2 {
		t.Fatalf("got %d dicts, want 2 for the golden job", len(dicts))
	}

	got := plistFor("com.evie.cron.finance-daily", 7, "/usr/local/bin/evie", dicts)

	want, err := os.ReadFile("testdata/cron-golden.plist")
	if err != nil {
		t.Fatalf("test setup: read golden file: %v", err)
	}

	if normalizePlist(string(got)) != normalizePlist(string(want)) {
		t.Errorf("plistFor output does not match testdata/cron-golden.plist\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// Every string interpolated into the XML goes through an escape helper —
// a binPath with & and a space is the probe. (Only label, path, and id
// enter the plist; the command never does.)
func TestPlistForEscapesBinPath(t *testing.T) {
	dicts, err := parseSchedule("* * * * *")
	if err != nil {
		t.Fatalf("parseSchedule: %v", err)
	}

	binPath := "/Users/david b/tools & bins/evie"
	got := string(plistFor("com.evie.cron.esc", 3, binPath, dicts))

	if !strings.Contains(got, "/Users/david b/tools &amp; bins/evie") {
		t.Errorf("plist does not carry the escaped binary path:\n%s", got)
	}
	if strings.Contains(got, "tools & bins") {
		t.Errorf("plist leaked a raw ampersand — XML this invalid never loads:\n%s", got)
	}
	if !strings.Contains(got, "<string>cron-exec</string>") {
		t.Errorf("plist is missing the cron-exec argument:\n%s", got)
	}
	if !strings.Contains(got, "<string>3</string>") {
		t.Errorf("plist is missing the job id argument:\n%s", got)
	}
	if !strings.Contains(got, "com.evie.cron.esc") {
		t.Errorf("plist is missing the label:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// cron_add
// ---------------------------------------------------------------------------

func TestCronAdd(t *testing.T) {
	t.Run("happy path inserts the row and installs the plist", func(t *testing.T) {
		db := newCronDB(t)
		installs := captureInstalls(t)

		out, err := runCronTool(t, "cron_add", map[string]any{
			"name":     "finance-daily",
			"schedule": "0 9 * * *",
			"command":  "finance sync && finance categorize",
		})
		if err != nil {
			t.Fatalf("cron_add returned error: %v", err)
		}

		var (
			id                           int64
			schedule, command, createdAt string
			enabled                      int64
		)
		if err := db.QueryRow(
			`SELECT id, schedule, command, created_at, enabled FROM jobs WHERE name = ?`,
			"finance-daily",
		).Scan(&id, &schedule, &command, &createdAt, &enabled); err != nil {
			t.Fatalf("jobs row missing after cron_add: %v", err)
		}
		if schedule != "0 9 * * *" {
			t.Errorf("schedule = %q, want the expression stored verbatim", schedule)
		}
		if command != "finance sync && finance categorize" {
			t.Errorf("command = %q stored wrong", command)
		}
		if _, err := time.Parse(time.RFC3339, createdAt); err != nil {
			t.Errorf("created_at %q is not RFC3339: %v", createdAt, err)
		}
		if enabled != 1 {
			t.Errorf("enabled = %d, want 1", enabled)
		}

		if len(*installs) != 1 {
			t.Fatalf("installJob called %d times, want 1", len(*installs))
		}
		call := (*installs)[0]
		if call.label != "com.evie.cron.finance-daily" {
			t.Errorf("installJob label = %q, want %q", call.label, "com.evie.cron.finance-daily")
		}
		wantSuffix := filepath.Join("Library", "LaunchAgents", "com.evie.cron.finance-daily.plist")
		if !strings.HasSuffix(call.plistPath, wantSuffix) {
			t.Errorf("installJob plistPath = %q, want it to end in %q", call.plistPath, wantSuffix)
		}
		plist := string(call.plist)
		if !strings.Contains(plist, "<string>cron-exec</string>") {
			t.Errorf("plist bytes missing the cron-exec argument:\n%s", plist)
		}
		if !strings.Contains(plist, "<string>"+strconv.FormatInt(id, 10)+"</string>") {
			t.Errorf("plist bytes missing the job id %d:\n%s", id, plist)
		}

		// Success result names the job and where the plist lives.
		if !strings.Contains(out, "finance-daily") {
			t.Errorf("result %q does not name the job", out)
		}
		if !strings.Contains(out, ".plist") {
			t.Errorf("result %q does not say where the plist lives", out)
		}
	})

	t.Run("invalid names are rejected before anything happens", func(t *testing.T) {
		db := newCronDB(t)
		installs := captureInstalls(t)

		for _, name := range []string{"", "Bad", "has space", "under_score", "dots.dots", "café"} {
			t.Run(strconv.Quote(name), func(t *testing.T) {
				out, err := runCronTool(t, "cron_add", map[string]any{
					"name": name, "schedule": "0 9 * * *", "command": "true",
				})
				if err == nil {
					t.Fatalf("cron_add accepted invalid name %q: %q", name, out)
				}
			})
		}
		if n := jobCount(t, db); n != 0 {
			t.Errorf("%d rows inserted for invalid names, want 0", n)
		}
		if len(*installs) != 0 {
			t.Errorf("installJob called %d times for invalid names, want 0", len(*installs))
		}
	})

	t.Run("a taken name errors and suggests cron_remove", func(t *testing.T) {
		db := newCronDB(t)
		installs := captureInstalls(t)

		if _, err := runCronTool(t, "cron_add", map[string]any{
			"name": "daily", "schedule": "0 9 * * *", "command": "true",
		}); err != nil {
			t.Fatalf("first cron_add returned error: %v", err)
		}

		out, err := runCronTool(t, "cron_add", map[string]any{
			"name": "daily", "schedule": "30 10 * * *", "command": "false",
		})
		if err == nil {
			t.Fatalf("cron_add silently accepted a taken name: %q", out)
		}
		if !strings.Contains(err.Error(), "cron_remove") {
			t.Errorf("error %v does not suggest cron_remove first", err)
		}
		if n := jobCount(t, db); n != 1 {
			t.Errorf("%d rows after duplicate add, want 1", n)
		}
		if len(*installs) != 1 {
			t.Errorf("installJob called %d times, want only the first add's call", len(*installs))
		}
	})

	t.Run("a bad schedule errors with no row and no install", func(t *testing.T) {
		db := newCronDB(t)
		installs := captureInstalls(t)

		for _, schedule := range []string{"not a cron", "0 9 1 * 1", "60 * * * *"} {
			t.Run(schedule, func(t *testing.T) {
				out, err := runCronTool(t, "cron_add", map[string]any{
					"name": "bad-sched", "schedule": schedule, "command": "true",
				})
				if err == nil {
					t.Fatalf("cron_add accepted schedule %q: %q", schedule, out)
				}
			})
		}
		if n := jobCount(t, db); n != 0 {
			t.Errorf("%d rows inserted for bad schedules, want 0", n)
		}
		if len(*installs) != 0 {
			t.Errorf("installJob called %d times for bad schedules, want 0", len(*installs))
		}
	})

	t.Run("an install failure rolls the row back", func(t *testing.T) {
		db := newCronDB(t)
		original := installJob
		originalUninstall := uninstallJob
		installJob = func(_ context.Context, label, plistPath string, plist []byte) error {
			return &installBootError{}
		}
		uninstallCalls := 0
		uninstallJob = func(context.Context, string, string) error {
			uninstallCalls++
			return nil
		}
		t.Cleanup(func() {
			installJob = original
			uninstallJob = originalUninstall
		})

		out, err := runCronTool(t, "cron_add", map[string]any{
			"name": "doomed", "schedule": "0 9 * * *", "command": "true",
		})
		if err == nil {
			t.Fatalf("cron_add succeeded despite installJob failing: %q", out)
		}
		if !strings.Contains(err.Error(), "bootstrap failed for test") {
			t.Errorf("error %v does not carry installJob's failure", err)
		}
		// No half-registered jobs: the row must be deleted again.
		if n := jobCount(t, db); n != 0 {
			t.Errorf("%d rows survived a failed install, want 0", n)
		}
		if uninstallCalls != 0 {
			t.Fatalf("ordinary install failure invoked reconciliation %d times", uninstallCalls)
		}
	})

	// Added after code review: the rollback DELETE's own error was
	// discarded, so a failed rollback left exactly the half-registered job
	// the spec forbids while reporting only the install failure. The db is
	// closed under cron_add's feet to make the DELETE fail; the error must
	// then name the job that needs manual cleanup.
	t.Run("a failed rollback is reported, not swallowed", func(t *testing.T) {
		newCronDB(t)

		// The seam hands cron_add a connection this test also holds, and
		// closing it turns the rollback DELETE into an error.
		var handedOut *sql.DB
		originalOpen := openCronDB
		openCronDB = func(_ context.Context) (*sql.DB, error) {
			db, err := originalOpen(context.Background())
			handedOut = db
			return db, err
		}
		t.Cleanup(func() { openCronDB = originalOpen })

		originalInstall := installJob
		installJob = func(_ context.Context, label, plistPath string, plist []byte) error {
			handedOut.Close()
			return &installBootError{}
		}
		t.Cleanup(func() { installJob = originalInstall })

		out, err := runCronTool(t, "cron_add", map[string]any{
			"name": "orphan", "schedule": "0 9 * * *", "command": "true",
		})
		if err == nil {
			t.Fatalf("cron_add succeeded despite install and rollback failing: %q", out)
		}
		msg := err.Error()
		if !strings.Contains(msg, "bootstrap failed for test") {
			t.Errorf("error %v dropped the original install failure", err)
		}
		if !strings.Contains(msg, "orphan") {
			t.Errorf("error %v does not name the job left in the database", err)
		}
	})
}

// installBootError is a distinct error type so the rollback test can
// confirm cron_add surfaces the seam's own failure text.
type installBootError struct{}

func (*installBootError) Error() string { return "bootstrap failed for test" }

// The timeout note is stored in job_runs.output and read by people, so a
// whole number of minutes must render "30m" — Duration.String() gives
// "30m0s". Added after code review found the rendered string had drifted
// from the one the spec and eviedb's round-trip test both pin.
func TestHumanDuration(t *testing.T) {
	for _, tc := range []struct {
		in   time.Duration
		want string
	}{
		{30 * time.Minute, "30m"},
		{time.Hour, "1h"},
		{90 * time.Minute, "1h30m"},
		{30 * time.Second, "30s"},
		{500 * time.Millisecond, "500ms"},
		{90 * time.Second, "1m30s"},
	} {
		if got := humanDuration(tc.in); got != tc.want {
			t.Errorf("humanDuration(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// cron_list
// ---------------------------------------------------------------------------

func TestCronList(t *testing.T) {
	t.Run("no jobs says so", func(t *testing.T) {
		newCronDB(t)

		out, err := runCronTool(t, "cron_list", map[string]any{})
		if err != nil {
			t.Fatalf("cron_list returned error: %v", err)
		}
		low := strings.ToLower(out)
		if !strings.Contains(low, "no ") || !strings.Contains(low, "job") {
			t.Errorf("result %q does not say there are no jobs", out)
		}
	})

	t.Run("renders one block per job with last run or never run", func(t *testing.T) {
		db := newCronDB(t)
		captureInstalls(t)

		for _, j := range []struct{ name, schedule, command string }{
			{"finance-daily", "0 9 * * *", "finance sync && finance categorize"},
			{"weekly-report", "30 8 * * 1", "echo report"},
		} {
			if _, err := runCronTool(t, "cron_add", map[string]any{
				"name": j.name, "schedule": j.schedule, "command": j.command,
			}); err != nil {
				t.Fatalf("cron_add %s: %v", j.name, err)
			}
		}

		var financeID int64
		if err := db.QueryRow(`SELECT id FROM jobs WHERE name = ?`, "finance-daily").Scan(&financeID); err != nil {
			t.Fatalf("select finance-daily id: %v", err)
		}
		insertRun(t, db, 1, financeID, "2026-08-04T09:00:02-04:00", 0)

		out, err := runCronTool(t, "cron_list", map[string]any{})
		if err != nil {
			t.Fatalf("cron_list returned error: %v", err)
		}

		for _, want := range []string{
			"finance-daily", "0 9 * * *", "finance sync && finance categorize",
			"weekly-report", "30 8 * * 1", "echo report",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("result is missing %q:\n%s", want, out)
			}
		}
		// The job that has run shows its run; the other says "never run".
		if !strings.Contains(out, "2026-08-04T09:00:02-04:00") {
			t.Errorf("result is missing finance-daily's last run time:\n%s", out)
		}
		if !strings.Contains(out, "never run") {
			t.Errorf("result does not say %q for the run-less job:\n%s", "never run", out)
		}
	})

	// Most recent run = highest job_runs.id, NOT max(started_at):
	// local-time RFC3339 breaks lexicographic ordering across the DST
	// fall-back, when a later run can carry an earlier-sorting timestamp.
	t.Run("latest run is by highest id not by started_at", func(t *testing.T) {
		db := newCronDB(t)
		captureInstalls(t)

		if _, err := runCronTool(t, "cron_add", map[string]any{
			"name": "dst-job", "schedule": "* * * * *", "command": "true",
		}); err != nil {
			t.Fatalf("cron_add: %v", err)
		}
		var jobID int64
		if err := db.QueryRow(`SELECT id FROM jobs WHERE name = ?`, "dst-job").Scan(&jobID); err != nil {
			t.Fatalf("select dst-job id: %v", err)
		}

		// Run 1 (older) sorts LATER as a string; run 2 (newer, higher id)
		// sorts earlier. Exit 42 belongs to the real latest run.
		insertRun(t, db, 1, jobID, "2026-11-01T01:30:00-04:00", 0)
		insertRun(t, db, 2, jobID, "2026-11-01T01:15:00-05:00", 42)

		out, err := runCronTool(t, "cron_list", map[string]any{})
		if err != nil {
			t.Fatalf("cron_list returned error: %v", err)
		}
		if !strings.Contains(out, "2026-11-01T01:15:00-05:00") {
			t.Errorf("result does not show the highest-id run's time:\n%s", out)
		}
		if !strings.Contains(out, "42") {
			t.Errorf("result does not show the highest-id run's exit code 42:\n%s", out)
		}
		if strings.Contains(out, "2026-11-01T01:30:00-04:00") {
			t.Errorf("result shows the stale run picked by max(started_at):\n%s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// cron_remove
// ---------------------------------------------------------------------------

func TestCronRemove(t *testing.T) {
	t.Run("happy path uninstalls, deletes the job, keeps the runs", func(t *testing.T) {
		db := newCronDB(t)
		captureInstalls(t)
		uninstalls := captureUninstalls(t)

		if _, err := runCronTool(t, "cron_add", map[string]any{
			"name": "doomed", "schedule": "0 9 * * *", "command": "true",
		}); err != nil {
			t.Fatalf("cron_add: %v", err)
		}
		var jobID int64
		if err := db.QueryRow(`SELECT id FROM jobs WHERE name = ?`, "doomed").Scan(&jobID); err != nil {
			t.Fatalf("select doomed id: %v", err)
		}
		insertRun(t, db, 1, jobID, "2026-08-04T09:00:00-04:00", 7)

		if _, err := runCronTool(t, "cron_remove", map[string]any{"name": "doomed"}); err != nil {
			t.Fatalf("cron_remove returned error: %v", err)
		}

		if len(*uninstalls) != 1 {
			t.Fatalf("uninstallJob called %d times, want 1", len(*uninstalls))
		}
		call := (*uninstalls)[0]
		if call.label != "com.evie.cron.doomed" {
			t.Errorf("uninstallJob label = %q, want %q", call.label, "com.evie.cron.doomed")
		}
		wantSuffix := filepath.Join("Library", "LaunchAgents", "com.evie.cron.doomed.plist")
		if !strings.HasSuffix(call.plistPath, wantSuffix) {
			t.Errorf("uninstallJob plistPath = %q, want it to end in %q", call.plistPath, wantSuffix)
		}

		if n := jobCount(t, db); n != 0 {
			t.Errorf("%d jobs rows after remove, want 0", n)
		}
		// History outlives the job — the whole point of job_runs.
		if n := runCount(t, db); n != 1 {
			t.Errorf("%d job_runs rows after remove, want the run kept", n)
		}
	})

	t.Run("an unknown name errors listing the existing jobs", func(t *testing.T) {
		newCronDB(t)
		captureInstalls(t)
		captureUninstalls(t)

		for _, name := range []string{"alpha", "beta"} {
			if _, err := runCronTool(t, "cron_add", map[string]any{
				"name": name, "schedule": "0 9 * * *", "command": "true",
			}); err != nil {
				t.Fatalf("cron_add %s: %v", name, err)
			}
		}

		out, err := runCronTool(t, "cron_remove", map[string]any{"name": "gamma"})
		if err == nil {
			t.Fatalf("cron_remove succeeded for an unknown name: %q", out)
		}
		for _, want := range []string{"alpha", "beta"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %v does not list existing job %q", err, want)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// RunScheduled — real shell, no mocking, same as bash_test.go
// ---------------------------------------------------------------------------

func TestRunScheduled(t *testing.T) {
	t.Run("output and exit 0 are returned verbatim", func(t *testing.T) {
		out, code := RunScheduled("echo hello-from-cron", time.Minute)
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
		if !strings.Contains(string(out), "hello-from-cron") {
			t.Errorf("output %q missing stdout", out)
		}
	})

	// The bash "result, not error" rule applied to scheduled runs: a
	// failing job is an exit code in the row, with stderr captured too.
	t.Run("a non-zero exit is returned with combined output", func(t *testing.T) {
		out, code := RunScheduled("echo oops >&2; exit 3", time.Minute)
		if code != 3 {
			t.Errorf("exit code = %d, want 3", code)
		}
		if !strings.Contains(string(out), "oops") {
			t.Errorf("output %q missing stderr — stdout and stderr must be combined", out)
		}
	})

	t.Run("output is capped at 64KB with a truncation note", func(t *testing.T) {
		out, code := RunScheduled("head -c 200000 /dev/zero | tr '\\0' a", time.Minute)
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
		if len(out) >= 200000 {
			t.Fatalf("output is %d bytes — the 64KB cap was not applied", len(out))
		}
		// Head kept plus a short note, nothing near the raw size.
		if len(out) > 64*1024+1024 {
			t.Errorf("output is %d bytes, want at most 64KB plus a note", len(out))
		}
		if !strings.HasPrefix(string(out), "aaaa") {
			t.Errorf("output does not keep the head of the stream (starts %q)", firstBytes(out, 40))
		}
		if !strings.Contains(strings.ToLower(string(out)), "truncat") {
			t.Errorf("capped output carries no truncation note (ends %q)", lastBytes(out, 200))
		}
	})

	t.Run("a timeout kills the command and returns -1 with a note", func(t *testing.T) {
		start := time.Now()
		out, code := RunScheduled("sleep 30", 500*time.Millisecond)
		elapsed := time.Since(start)

		if code != -1 {
			t.Errorf("exit code = %d, want -1 for a timed-out run", code)
		}
		low := strings.ToLower(string(out))
		if !strings.Contains(low, "killed") || !strings.Contains(low, "timed out") {
			t.Errorf("output %q missing the killed-on-timeout note", out)
		}
		// The note names the duration the way a person would write it.
		if !strings.Contains(string(out), "500ms") {
			t.Errorf("output %q does not name the timeout that killed it", out)
		}
		// snapshot() may cost up to ~10s; waiting out the full sleep may not.
		if elapsed > 20*time.Second {
			t.Errorf("RunScheduled took %s — the timeout did not kill the command", elapsed)
		}
	})

	t.Run("an unrunnable shell is -1 with the error text", func(t *testing.T) {
		t.Setenv("SHELL", filepath.Join(t.TempDir(), "no-such-shell"))

		out, code := RunScheduled("echo unreachable", time.Minute)
		if code != -1 {
			t.Errorf("exit code = %d, want -1 when the shell cannot start", code)
		}
		if len(out) == 0 {
			t.Error("output is empty, want the exec error text so the row explains itself")
		}
		if strings.Contains(string(out), "unreachable") {
			t.Errorf("output %q — the command ran despite the bogus shell", out)
		}
	})
}

// ---------------------------------------------------------------------------
// registry and db registration
// ---------------------------------------------------------------------------

// All three cron tools are ungated — David's call, recorded in the spec
// as the loudest security decision since ungated bash.
func TestCronToolsRegisteredUngated(t *testing.T) {
	find := func(name string) *Tool {
		for i := range all {
			if all[i].Schema.Function.Name == name {
				return &all[i]
			}
		}
		return nil
	}

	for _, name := range []string{"cron_add", "cron_list", "cron_remove"} {
		t.Run(name, func(t *testing.T) {
			tool := find(name)
			if tool == nil {
				t.Fatalf("%s is not in the tool registry", name)
			}
			if tool.NeedsApproval {
				t.Errorf("%s is gated, want it ungated", name)
			}
			if tool.Execute == nil {
				t.Fatalf("%s has no Execute function", name)
			}
		})
	}

	t.Run("cron_add parameters", func(t *testing.T) {
		tool := find("cron_add")
		if tool == nil {
			t.Fatal("cron_add is not in the tool registry")
		}
		req := tool.Schema.Function.Parameters.Required
		want := map[string]bool{"name": true, "schedule": true, "command": true}
		if len(req) != len(want) {
			t.Errorf("required parameters = %v, want name, schedule, command", req)
		}
		for _, r := range req {
			if !want[r] {
				t.Errorf("unexpected required parameter %q", r)
			}
		}
		for prop := range want {
			if _, ok := tool.Schema.Function.Parameters.Properties[prop]; !ok {
				t.Errorf("schema has no %s property", prop)
			}
		}
		// launchd evaluates in local time; the description must say so.
		desc := strings.ToLower(tool.Schema.Function.Description)
		for _, p := range tool.Schema.Function.Parameters.Properties {
			desc += " " + strings.ToLower(p.Description)
		}
		if !strings.Contains(desc, "local") {
			t.Errorf("cron_add's description does not say schedules run in local time")
		}
	})

	t.Run("cron_list takes no parameters", func(t *testing.T) {
		tool := find("cron_list")
		if tool == nil {
			t.Fatal("cron_list is not in the tool registry")
		}
		if req := tool.Schema.Function.Parameters.Required; len(req) != 0 {
			t.Errorf("required parameters = %v, want none", req)
		}
	})

	t.Run("cron_remove requires only the name", func(t *testing.T) {
		tool := find("cron_remove")
		if tool == nil {
			t.Fatal("cron_remove is not in the tool registry")
		}
		req := tool.Schema.Function.Parameters.Required
		if len(req) != 1 || req[0] != "name" {
			t.Errorf("required parameters = %v, want [name]", req)
		}
	})
}

// The evie db is registered read-side only: query_db gains it, edit_db
// does not — a hand-edited jobs row would silently diverge from its plist.
func TestEvieDBRegistration(t *testing.T) {
	find := func(name string) *Tool {
		for i := range all {
			if all[i].Schema.Function.Name == name {
				return &all[i]
			}
		}
		t.Fatalf("%s is not in the tool registry", name)
		return nil
	}

	t.Run("query_db enum gains evie and keeps finance", func(t *testing.T) {
		enum := find("query_db").Schema.Function.Parameters.Properties["db"].Enum
		got := map[string]bool{}
		for _, v := range enum {
			got[v] = true
		}
		if !got["evie"] {
			t.Errorf("query_db enum %v is missing %q", enum, "evie")
		}
		if !got["finance"] {
			t.Errorf("query_db enum %v lost %q", enum, "finance")
		}
	})

	t.Run("query_db description teaches the evie schema", func(t *testing.T) {
		desc := find("query_db").Schema.Function.Description
		for _, want := range []string{"evie", "jobs(", "job_runs(", "job_runs"} {
			if !strings.Contains(desc, want) {
				t.Errorf("query_db description does not mention %q", want)
			}
		}
	})

	t.Run("edit_db enum does NOT gain evie", func(t *testing.T) {
		enum := find("edit_db").Schema.Function.Parameters.Properties["db"].Enum
		for _, v := range enum {
			if v == "evie" {
				t.Errorf("edit_db enum %v offers evie — writes must go through the cron tools", enum)
			}
		}
	})

	t.Run("edit_db on evie points at the cron tools", func(t *testing.T) {
		out, err := editDB(context.Background(), `{"db":"evie","statement":"DELETE FROM jobs"}`)
		if err == nil {
			t.Fatalf("edit_db accepted a evie write: %q", out)
		}
		msg := err.Error()
		for _, want := range []string{"cron_add", "cron_remove", "read-only"} {
			if !strings.Contains(msg, want) {
				t.Errorf("error %q does not mention %q", msg, want)
			}
		}
	})

	t.Run("unknown-db errors list both databases", func(t *testing.T) {
		if _, err := editDB(context.Background(), `{"db":"bogus","statement":"DELETE FROM x"}`); err == nil {
			t.Fatal("edit_db accepted an unknown db")
		} else {
			for _, want := range []string{"finance", "evie"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("edit_db unknown-db error %v does not list %q", err, want)
				}
			}
		}

		if _, err := queryDB(context.Background(), `{"db":"bogus","query":"SELECT 1"}`); err == nil {
			t.Fatal("query_db accepted an unknown db")
		} else {
			for _, want := range []string{"finance", "evie"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("query_db unknown-db error %v does not list %q", err, want)
				}
			}
		}
	})
}
