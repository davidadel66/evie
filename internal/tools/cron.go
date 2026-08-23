package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"fmt"
	"github.com/davidadel66/evie/internal/openrouter"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
)

// maxCalendarDicts caps the StartCalendarInterval cross-product. launchd
// takes an array of dicts, one per firing combination; an expression like
// "*/2 */2 * * *" expands to 360 of them, which is a plist nobody meant
// to write. Past the cap the parser errors and tells the model to
// simplify the expression instead.
const maxCalendarDicts = 100

// calendarDict is one StartCalendarInterval entry. Nil means the field
// was "*" — omitted from the plist, which launchd treats as a wildcard
// ("Missing arguments are considered to be wildcard", launchd.plist(5)).
type calendarDict struct {
	Minute, Hour, Day, Month, Weekday *int
}

// cronField describes one of the five expression fields: its launchd
// name, its bounds, and any special-case rejection.
type cronField struct {
	name     string
	min, max int
}

var cronFields = [5]cronField{
	{"minute", 0, 59},
	{"hour", 0, 23},
	{"day-of-month", 1, 31},
	{"month", 1, 12},
	{"weekday", 0, 6},
}

// parseSchedule turns a 5-field cron expression into the launchd
// calendar dicts it translates to. The grammar is deliberately a subset
// of vixie cron — every unsupported shape errors with instructions
// rather than approximating, because a schedule that fires at the wrong
// time is worse than one that refuses to parse:
//
//   - per field: "*", a single number, a comma list of single numbers,
//     a range "a-b", or a step "*/n" (steps on ranges or numbers are
//     rejected; names like "mon" are rejected)
//   - restricting BOTH day-of-month and weekday is rejected: vixie ORs
//     them, launchd's dict ANDs them, and silently changing semantics
//     is worse than refusing
//   - weekday 7 is rejected with "use 0 for Sunday" — both conventions
//     exist in the wild and guessing invites an off-by-one-day bug
func parseSchedule(expr string) ([]calendarDict, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return nil, fmt.Errorf("expected 5 fields (minute hour day-of-month month weekday), got %d in %q", len(fields), expr)
	}

	sets := make([][]int, 5)
	for i, raw := range fields {
		vals, err := parseCronField(raw, cronFields[i])
		if err != nil {
			return nil, err
		}
		sets[i] = vals
	}

	// dom and dow: refuse when both are restricted.
	if sets[2] != nil && sets[4] != nil {
		return nil, fmt.Errorf("restricting both day-of-month and weekday is not supported: cron fires on either day, launchd only on days matching both — pick one field")
	}

	total := 1
	for _, s := range sets {
		if s != nil {
			total *= len(s)
		}
	}
	if total > maxCalendarDicts {
		return nil, fmt.Errorf("this schedule expands to %d launchd calendar entries (limit %d) — simplify the expression", total, maxCalendarDicts)
	}

	// Cross-product of the restricted fields; "*" fields stay nil in
	// every dict and are omitted from the plist.
	dicts := []calendarDict{{}}
	for fieldIdx, s := range sets {
		if s == nil {
			continue
		}
		next := make([]calendarDict, 0, len(dicts)*len(s))
		for _, d := range dicts {
			for _, v := range s {
				v := v
				nd := d
				switch fieldIdx {
				case 0:
					nd.Minute = &v
				case 1:
					nd.Hour = &v
				case 2:
					nd.Day = &v
				case 3:
					nd.Month = &v
				case 4:
					nd.Weekday = &v
				}
				next = append(next, nd)
			}
		}
		dicts = next
	}

	return dicts, nil
}

// parseCronField expands one field to its sorted, deduped value set, or
// nil for "*".
func parseCronField(raw string, f cronField) ([]int, error) {
	if raw == "*" {
		return nil, nil
	}

	var vals []int
	switch {
	case strings.HasPrefix(raw, "*/"):
		step, err := strconv.Atoi(raw[2:])
		if err != nil || step < 1 {
			return nil, fmt.Errorf("%s: bad step %q — */n needs a positive number", f.name, raw)
		}
		for v := f.min; v <= f.max; v += step {
			vals = append(vals, v)
		}

	case strings.Contains(raw, "/"):
		return nil, fmt.Errorf("%s: %q — steps are only supported on wildcards (*/n), not on numbers or ranges", f.name, raw)

	case strings.Contains(raw, ","):
		for _, part := range strings.Split(raw, ",") {
			if strings.ContainsAny(part, "-/") {
				return nil, fmt.Errorf("%s: %q — list items must be single numbers (no ranges or steps inside a list)", f.name, raw)
			}
			v, err := parseCronValue(part, f)
			if err != nil {
				return nil, err
			}
			vals = append(vals, v)
		}

	case strings.Contains(raw, "-"):
		lo, hi, _ := strings.Cut(raw, "-")
		a, err := parseCronValue(lo, f)
		if err != nil {
			return nil, err
		}
		b, err := parseCronValue(hi, f)
		if err != nil {
			return nil, err
		}
		if a > b {
			return nil, fmt.Errorf("%s: range %q runs backwards", f.name, raw)
		}
		for v := a; v <= b; v++ {
			vals = append(vals, v)
		}

	default:
		v, err := parseCronValue(raw, f)
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}

	// Dedupe before anything counts against the cap.
	sort.Ints(vals)
	deduped := vals[:0]
	for i, v := range vals {
		if i == 0 || v != vals[i-1] {
			deduped = append(deduped, v)
		}
	}
	return deduped, nil
}

// parseCronValue parses and range-checks a single number in a field.
func parseCronValue(s string, f cronField) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a number (names like jan/mon are not supported)", f.name, s)
	}
	if f.name == "weekday" && v == 7 {
		return 0, fmt.Errorf("weekday: 7 is ambiguous — use 0 for Sunday")
	}
	if v < f.min || v > f.max {
		return 0, fmt.Errorf("%s: %d is out of range (%d-%d)", f.name, v, f.min, f.max)
	}
	return v, nil
}

// plistFor renders the LaunchAgent plist for one job. The binary path is
// a parameter rather than os.Executable() called here, so tests can pin
// a golden file against a fixed path; cron_add passes the real one.
// Every interpolated string goes through xmlEscape — the label and path
// are the only variable text, but escaping unconditionally costs
// nothing and never surprises.
func plistFor(label string, id int64, binPath string, dicts []calendarDict) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")

	fmt.Fprintf(&b, "\t<key>Label</key>\n\t<string>%s</string>\n", xmlEscape(label))

	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	fmt.Fprintf(&b, "\t\t<string>%s</string>\n", xmlEscape(binPath))
	b.WriteString("\t\t<string>cron-exec</string>\n")
	fmt.Fprintf(&b, "\t\t<string>%d</string>\n", id)
	b.WriteString("\t</array>\n")

	b.WriteString("\t<key>StartCalendarInterval</key>\n\t<array>\n")
	for _, d := range dicts {
		b.WriteString("\t\t<dict>\n")
		for _, kv := range []struct {
			key string
			val *int
		}{
			{"Minute", d.Minute}, {"Hour", d.Hour}, {"Day", d.Day},
			{"Month", d.Month}, {"Weekday", d.Weekday},
		} {
			if kv.val != nil {
				fmt.Fprintf(&b, "\t\t\t<key>%s</key>\n\t\t\t<integer>%d</integer>\n", kv.key, *kv.val)
			}
		}
		b.WriteString("\t\t</dict>\n")
	}
	b.WriteString("\t</array>\n")

	b.WriteString("</dict>\n</plist>\n")
	return []byte(b.String())
}

// xmlEscape covers the five XML special characters — plists are XML and
// a path or label containing & would otherwise corrupt the file.
var xmlEscaper = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
)

func xmlEscape(s string) string {
	return xmlEscaper.Replace(s)
}

// openCronDB is the tools-layer seam onto evie's own database — a var so
// tests point it at a temp path, same pattern as fetchTimeout.
var openCronDB = eviedb.OpenDBContext

// installJob writes a job's plist and loads it into launchd. A var seam
// so tests capture calls instead of touching launchctl. It owns ALL
// plist file I/O: on bootstrap failure it removes the file it wrote, so
// cron_add's rollback only has the db row to worry about.
//
// Bootout before bootstrap, always — bootstrap on an already-loaded
// label fails with "Bootstrap failed: 5" — and bootout errors are
// ignored unconditionally: distinguishing "not found" from real
// failures means parsing launchctl stderr, and the bootstrap that
// follows surfaces anything real.
var installJob = func(ctx context.Context, label, plistPath string, plist []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// ~/Library/LaunchAgents can be absent on a fresh account.
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return fmt.Errorf("make LaunchAgents dir: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.WriteFile(plistPath, plist, 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	uid := strconv.Itoa(os.Getuid())
	if err := ctx.Err(); err != nil {
		_ = os.Remove(plistPath)
		return err
	}
	_ = exec.CommandContext(ctx, "launchctl", "bootout", "gui/"+uid+"/"+label).Run()

	if err := ctx.Err(); err != nil {
		_ = os.Remove(plistPath)
		return err
	}
	if out, err := exec.CommandContext(ctx, "launchctl", "bootstrap", "gui/"+uid, plistPath).CombinedOutput(); err != nil {
		os.Remove(plistPath)
		return fmt.Errorf("launchctl bootstrap: %w: %s", err, out)
	}
	return nil
}

// uninstallJob unloads a job from launchd and deletes its plist. All
// errors tolerated — removing something already gone is success.
var uninstallJob = func(ctx context.Context, label, plistPath string) error {
	uid := strconv.Itoa(os.Getuid())
	_ = exec.CommandContext(ctx, "launchctl", "bootout", "gui/"+uid+"/"+label).Run()
	if err := ctx.Err(); err != nil {
		return err
	}
	_ = os.Remove(plistPath)
	return nil
}

// jobNameRe fences names to plist-filename-safe identifiers: the name
// becomes the launchd label and the file on disk.
var jobNameRe = regexp.MustCompile(`^[a-z0-9-]+$`)

func cronLabel(name string) string { return "com.evie.cron." + name }

func cronPlistPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", cronLabel(name)+".plist"), nil
}

// cronAddTool describes cron_add to the model: create a scheduled job
// that fires via launchd whether or not evie is running.
var cronAddTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name: "cron_add",
		Description: `Schedule a recurring shell command. The job fires via macOS launchd even when evie is not running; if the Mac is asleep at fire time it runs on wake (missed fires while powered off are lost). Every run is recorded in the evie database (job_runs table, query it with query_db db=evie) — a failing job shows up there, nowhere else.

The schedule is a 5-field cron expression (minute hour day-of-month month weekday) evaluated in local time. Supported per field: *, a number, a comma list of numbers, a range a-b, or */n. Not supported (you get an error): names like mon/jan, steps on ranges, restricting both day-of-month and weekday at once, weekday 7 (use 0 for Sunday).

The command runs through the user's login shell with their full environment. Change a job by cron_remove then cron_add — there is no edit.

Each job's plist records the absolute path of the evie binary that created it. If evie is moved or reinstalled at a different path, existing jobs keep pointing at the old path and silently stop working — cron_remove and cron_add every job to re-point them.

To debug a job that is not firing, run "launchctl print gui/$(id -u)/com.evie.cron.<name>" with the bash tool: it shows whether launchd has the job loaded, its next fire time, and the last exit status.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"name", "schedule", "command"},
			Properties: map[string]openrouter.Property{
				"name": {
					Type:        "string",
					Description: "Unique job identifier, lowercase letters/digits/hyphens only (it becomes the launchd label).",
				},
				"schedule": {
					Type:        "string",
					Description: "5-field cron expression, local time. Example: \"0 9 * * *\" = daily at 09:00.",
				},
				"command": {
					Type:        "string",
					Description: "The shell command to run on each fire.",
				},
			},
		},
	},
}

func cronAdd(ctx context.Context, args string) (string, error) {
	var params struct {
		Name     string `json:"name"`
		Schedule string `json:"schedule"`
		Command  string `json:"command"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	if !jobNameRe.MatchString(params.Name) {
		return "", fmt.Errorf("invalid name %q — lowercase letters, digits, and hyphens only", params.Name)
	}
	if strings.TrimSpace(params.Command) == "" {
		return "", errors.New("command must not be empty")
	}

	dicts, err := parseSchedule(params.Schedule)
	if err != nil {
		return "", err
	}

	binPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate evie binary: %w", err)
	}
	plistPath, err := cronPlistPath(params.Name)
	if err != nil {
		return "", err
	}

	db, err := openCronDB(ctx)
	if err != nil {
		return "", fmt.Errorf("open evie db: %w", err)
	}
	defer db.Close()

	if err := ctx.Err(); err != nil {
		return "", err
	}
	res, err := db.ExecContext(ctx,
		`INSERT INTO jobs (name, schedule, command, created_at, enabled) VALUES (?, ?, ?, ?, 1)`,
		params.Name, params.Schedule, params.Command, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return "", fmt.Errorf("a job named %q already exists — cron_remove it first if you want to replace it", params.Name)
		}
		return "", fmt.Errorf("insert job: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return "", fmt.Errorf("job id: %w", err)
	}
	cleanupRow := func(primary error) error {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, delErr := db.ExecContext(cleanupCtx, `DELETE FROM jobs WHERE id = ?`, id); delErr != nil {
			return errors.Join(primary, fmt.Errorf(
				"could not roll back job row %d (%q) — it is in the database but NOT scheduled; retry cron_remove %s: %w",
				id, params.Name, params.Name, delErr))
		}
		return primary
	}
	if err := ctx.Err(); err != nil {
		return "", cleanupRow(err)
	}

	// No half-registered jobs: if launchd registration fails, the row
	// goes too (plist cleanup is installJob's own responsibility). If the
	// rollback ALSO fails, the db now holds a job that will never fire,
	// and the model is the only one who can tell David — so both errors
	// travel together rather than the delete failing silently.
	if err := installJob(ctx, cronLabel(params.Name), plistPath, plistFor(cronLabel(params.Name), id, binPath, dicts)); err != nil {
		if ctx.Err() != nil {
			return "", cleanupRow(ctx.Err())
		}
		err = fmt.Errorf("register with launchd: %w", err)
		return "", cleanupRow(err)
	}

	return fmt.Sprintf("Scheduled %q (%s): %s\nPlist: %s\n", params.Name, params.Schedule, params.Command, plistPath), nil
}

// cronListTool describes cron_list: the jobs ledger plus each job's most
// recent run.
var cronListTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "cron_list",
		Description: `List every scheduled job with its schedule, command, and most recent run (time and exit code), or "never run". For run history beyond the latest, query_db db=evie against the job_runs table.`,
		Parameters: openrouter.Parameter{
			Type:       "object",
			Properties: map[string]openrouter.Property{},
		},
	},
}

func cronList(ctx context.Context, _ string) (string, error) {
	db, err := openCronDB(ctx)
	if err != nil {
		return "", fmt.Errorf("open evie db: %w", err)
	}
	defer db.Close()

	// Latest run = highest job_runs.id, NOT max(started_at): timestamps
	// are local-time RFC3339, and lexicographic order breaks across the
	// DST fall-back.
	rows, err := db.QueryContext(ctx, `
		SELECT j.id, j.name, j.schedule, j.command, j.created_at, r.started_at, r.exit_code
		FROM jobs j
		LEFT JOIN job_runs r ON r.id = (SELECT MAX(id) FROM job_runs WHERE job_id = j.id)
		ORDER BY j.id`)
	if err != nil {
		return "", fmt.Errorf("query jobs: %w", err)
	}
	defer rows.Close()

	var b strings.Builder
	n := 0
	for rows.Next() {
		var (
			id                                 int64
			name, schedule, command, createdAt string
			lastStarted                        sql.NullString
			lastExit                           sql.NullInt64
		)
		if err := rows.Scan(&id, &name, &schedule, &command, &createdAt, &lastStarted, &lastExit); err != nil {
			return "", fmt.Errorf("scan job: %w", err)
		}
		n++
		fmt.Fprintf(&b, "[%d] %s\n    schedule: %s\n    command: %s\n    created: %s\n", id, name, schedule, command, createdAt)
		if lastStarted.Valid {
			fmt.Fprintf(&b, "    last run: %s (exit %d)\n", lastStarted.String, lastExit.Int64)
		} else {
			b.WriteString("    last run: never run\n")
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("read jobs: %w", err)
	}
	if n == 0 {
		return "no scheduled jobs\n", nil
	}
	return b.String(), nil
}

// cronRemoveTool describes cron_remove: unschedule and forget a job,
// keeping its run history.
var cronRemoveTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "cron_remove",
		Description: `Remove a scheduled job by name: unloads it from launchd, deletes its plist, and deletes the jobs row. Run history in job_runs is kept — it outlives the job.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"name"},
			Properties: map[string]openrouter.Property{
				"name": {
					Type:        "string",
					Description: "The job's name, as shown by cron_list.",
				},
			},
		},
	},
}

func cronRemove(ctx context.Context, args string) (string, error) {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	db, err := openCronDB(ctx)
	if err != nil {
		return "", fmt.Errorf("open evie db: %w", err)
	}
	defer db.Close()

	var id int64
	err = db.QueryRowContext(ctx, `SELECT id FROM jobs WHERE name = ?`, params.Name).Scan(&id)
	if err == sql.ErrNoRows {
		names, _ := jobNames(ctx, db)
		if len(names) == 0 {
			return "", fmt.Errorf("no job named %q — there are no scheduled jobs", params.Name)
		}
		return "", fmt.Errorf("no job named %q — existing jobs: %s", params.Name, strings.Join(names, ", "))
	}
	if err != nil {
		return "", fmt.Errorf("look up job: %w", err)
	}

	plistPath, err := cronPlistPath(params.Name)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Unloading and deleting are one existing consistency sequence. Once the
	// first mutation starts, finish both under the approved bounded cleanup
	// context even if the parent is cancelled in between.
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = uninstallJob(cleanupCtx, cronLabel(params.Name), plistPath)
	_, deleteErr := db.ExecContext(cleanupCtx, `DELETE FROM jobs WHERE id = ?`, id)
	if ctx.Err() != nil {
		if deleteErr != nil {
			return "", errors.Join(ctx.Err(), fmt.Errorf("delete job during cancellation cleanup: %w", deleteErr))
		}
		return "", ctx.Err()
	}
	if deleteErr != nil {
		return "", fmt.Errorf("delete job: %w", deleteErr)
	}

	// job_runs rows are deliberately kept: history outlives the job.
	return fmt.Sprintf("Removed %q. Its run history is kept in job_runs.\n", params.Name), nil
}

func jobNames(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM jobs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// maxRunOutput caps what one scheduled run stores in job_runs — the
// table is an audit trail, not a log archive.
const maxRunOutput = 64 * 1024

// RunScheduled executes one scheduled job's command the way the bash
// tool would — login shell, snapshot sourced so aliases and PATH edits
// exist, eval for the second parse — but with no sessionCwd read or
// write: a scheduled run is its own process and must not touch REPL
// state. Exported for cmd/evie's cron-exec subcommand, which is outside
// this package; everything it reuses (snapshot, shellQuote) stays
// unexported.
//
// There is no error return: job_runs is the only consumer, and every
// outcome is encoded as (output, exitCode). exitCode -1 means "did not
// complete normally" — the output text says whether that was a timeout
// kill or a failure to start. snapshot() is called synchronously; a
// one-shot process has no Warm window, and the capture cost is fine
// with nobody waiting.
func RunScheduled(command string, timeout time.Duration) ([]byte, int) {
	var script strings.Builder
	if snap := snapshot(context.Background()); snap != "" {
		fmt.Fprintf(&script, "source %s 2>/dev/null || true\n", shellQuote(snap))
		fmt.Fprintf(&script, "eval %s\n", shellQuote(command))
	} else {
		script.WriteString(command + "\n")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, shellPath(), "-l", "-c", script.String())
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	cmd.WaitDelay = 2 * time.Second

	out, err := cmd.CombinedOutput()
	out = capRunOutput(out)

	if ctx.Err() == context.DeadlineExceeded {
		return append(out, fmt.Sprintf("\n[killed: timed out after %s]", humanDuration(timeout))...), -1
	}

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return out, 0
	case errors.As(err, &exitErr):
		return out, exitErr.ExitCode()
	default:
		// Could not run at all — no shell, exec failure. The error text
		// is the only diagnosis there will ever be.
		return append(out, err.Error()...), -1
	}
}

// humanDuration trims the zero tail off Duration.String() so a whole
// number of minutes reads "30m", not "30m0s". This string is stored in
// job_runs.output and read by David and the model; the trailing "0s" is
// noise that suggests a precision the timeout does not have.
// The suffixes carry the preceding unit letter on purpose: trimming a
// bare "0s" would turn "30s" into "3".
func humanDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// capRunOutput keeps the head and notes the cut — the start of a failing
// job's output usually carries the diagnosis.
func capRunOutput(out []byte) []byte {
	if len(out) <= maxRunOutput {
		return out
	}
	dropped := len(out) - maxRunOutput
	return append(out[:maxRunOutput], fmt.Sprintf("\n[output truncated: %d bytes dropped]", dropped)...)
}
