package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/google/uuid"
)

// Retention (Stage 14 sub-stage 12, 2026-10-04; ADR 0045:
// "Evie retains a complete local audit trail by default ... Workflow
// Definitions classify sensitive fields for redaction ... Deletion is an
// explicit, auditable operation"; workflow-runtime.spec.md: "Workflow
// field classifications and Workspace policy govern redaction, retention,
// and export"). Each retention classification a definition declares
// (ordinary, sensitive) has the operator's policy:
//
//   - mask: once a run ended, the owner's surfaces (inspection, run
//     lists, interruptions, notices; CLI and web) show its sensitive
//     values as "[sensitive]" (on by default). Storage is unchanged.
//   - redact: this long after a run (and every successor of it) ended, its
//     sensitive values are removed from storage ("[redacted]"), with a
//     tombstone. Never by default.
//   - notices: read notices of finished runs and of schedules, skipped and
//     failed schedule slots that are not their schedule's latest, and
//     schedule controls a later one replaced are deleted this long after
//     they were recorded, with a tombstone per class. Never by default.
//   - compact: this long after a run ended, its intermediate checkpoints
//     lose their stored state, which inspection rebuilds exactly from the
//     attempts that produced it (seven days by default; nothing is lost).
//
// Whatever the policy, a run's sensitive inputs never reach a chat
// (workflow_status) or a notice's detail. The sweep runs in `evie serve`
// and the REPL under one runner lease across processes (every hour, first
// at start), and on the owner's command (`evie workflows retention
// sweep`), which a dry run previews.

// RetentionPolicy is one classification's policy. A nil window never
// applies.
type RetentionPolicy struct {
	Mask    bool           `json:"mask"`
	Redact  *time.Duration `json:"redact,omitempty"`
	Notices *time.Duration `json:"notices,omitempty"`
	Compact *time.Duration `json:"compact,omitempty"`
}

// RetentionConfig is the operator's policy for each classification.
type RetentionConfig struct {
	Ordinary  RetentionPolicy `json:"ordinary"`
	Sensitive RetentionPolicy `json:"sensitive"`
}

// DefaultRetentionCompact is how long after a run ended its checkpoints
// are compacted by default.
const DefaultRetentionCompact = 7 * 24 * time.Hour

// DefaultRetention masks sensitive values once a run ended, compacts
// checkpoints after seven days, and redacts and deletes nothing: the
// complete local audit trail ADR 0045 keeps by default.
func DefaultRetention() RetentionConfig {
	ordinary, sensitive := DefaultRetentionCompact, DefaultRetentionCompact
	return RetentionConfig{Ordinary: RetentionPolicy{Mask: true, Compact: &ordinary}, Sensitive: RetentionPolicy{Mask: true, Compact: &sensitive}}
}

// For is the policy of a classification; an unknown one gets the
// sensitive policy.
func (c RetentionConfig) For(classification string) RetentionPolicy {
	if classification == workflow.ClassificationOrdinary {
		return c.Ordinary
	}
	return c.Sensitive
}

// maxRetentionWindow bounds a window (about 100 years).
const maxRetentionWindow = 100 * 366 * 24 * time.Hour

// ParseRetentionPolicy reads a policy written as comma-separated
// settings over base: mask=on|off, and redact, notices and compact each
// "never", a number of days ("30d") or a duration ("12h", "0").
func ParseRetentionPolicy(text string, base RetentionPolicy) (RetentionPolicy, error) {
	p := base
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		key, value = strings.TrimSpace(key), strings.ToLower(strings.TrimSpace(value))
		if !ok || value == "" {
			return base, fmt.Errorf("retention setting %q is not key=value", part)
		}
		switch key {
		case "mask":
			switch value {
			case "on":
				p.Mask = true
			case "off":
				p.Mask = false
			default:
				return base, fmt.Errorf("mask is on or off, not %q", value)
			}
		case "redact", "notices", "compact":
			window, err := parseWindow(value)
			if err != nil {
				return base, fmt.Errorf("%s: %w", key, err)
			}
			switch key {
			case "redact":
				p.Redact = window
			case "notices":
				p.Notices = window
			default:
				p.Compact = window
			}
		default:
			return base, fmt.Errorf("unknown retention setting %q (mask, redact, notices or compact)", key)
		}
	}
	return p, nil
}

func parseWindow(value string) (*time.Duration, error) {
	if value == "never" {
		return nil, nil
	}
	var d time.Duration
	if days, ok := strings.CutSuffix(value, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return nil, fmt.Errorf("%q is not never, a number of days or a duration", value)
		}
		d = time.Duration(n) * 24 * time.Hour
	} else {
		var err error
		if d, err = time.ParseDuration(value); err != nil {
			return nil, fmt.Errorf("%q is not never, a number of days or a duration", value)
		}
	}
	if d < 0 || d > maxRetentionWindow {
		return nil, fmt.Errorf("%q is outside 0 to 36600 days", value)
	}
	return &d, nil
}

func windowText(d *time.Duration) string {
	switch {
	case d == nil:
		return "never"
	case *d == 0:
		return "at once"
	case *d%(24*time.Hour) == 0 && *d == 24*time.Hour:
		return "1 day"
	case *d%(24*time.Hour) == 0:
		return fmt.Sprintf("%d days", *d/(24*time.Hour))
	}
	return d.String()
}

func (p RetentionPolicy) redactText() string {
	if p.Redact == nil {
		return "redact never"
	}
	return "redact " + windowText(p.Redact) + " after a run ended"
}

func (p RetentionPolicy) noticesText() string {
	if p.Notices == nil {
		return "delete notices never"
	}
	return "delete notices " + windowText(p.Notices) + " after they were recorded"
}

func (p RetentionPolicy) compactText() string {
	if p.Compact == nil {
		return "compact never"
	}
	return "compact " + windowText(p.Compact) + " after a run ended"
}

// String is the policy in one line.
func (p RetentionPolicy) String() string {
	mask := "mask off"
	if p.Mask {
		mask = "mask on"
	}
	return strings.Join([]string{mask, p.redactText(), p.noticesText(), p.compactText()}, ", ")
}

var (
	// ErrRetentionBusy refuses a sweep while another Evie runs one.
	ErrRetentionBusy = errors.New("another Evie is running the workflow retention sweep; try again once it finishes")
	// ErrRetentionOff refuses a sweep in an Evie without retention
	// policies.
	ErrRetentionOff = errors.New("this Evie has no workflow retention policy")
)

// retentionRunnerLease bounds one sweep's hold on the runner lease.
const retentionRunnerLease = 10 * time.Minute

// maxSweepActions bounds the run actions of one sweep; the next sweep
// continues.
const maxSweepActions = 500

// sensitivityOf is the sensitivity of a run's pinned version, read once
// per version from its approved commit without validating it again.
func (r *Runtime) sensitivityOf(ctx context.Context, ws memory.WorkspaceID, name string, version int64, commit, path string) (workflow.Sensitivity, error) {
	key := fmt.Sprintf("%s\x00%s\x00%d\x00%s", ws, name, version, commit)
	r.mu.Lock()
	s, ok := r.sensitivities[key]
	r.mu.Unlock()
	if ok {
		return s, nil
	}
	s, err := workflow.ParseSensitivity(r.repo.Source(ctx, commit, path))
	if err != nil {
		return workflow.Sensitivity{}, fmt.Errorf("version %d of %s cannot be read: %w", version, name, err)
	}
	r.mu.Lock()
	r.sensitivities[key] = s
	r.mu.Unlock()
	return s, nil
}

// runSensitivity is a run's sensitivity; one that cannot be read is
// treated as the sensitive classification, so nothing is shown that might
// be sensitive.
func (r *Runtime) runSensitivity(ctx context.Context, run eviedb.WorkflowRun) workflow.Sensitivity {
	s, err := r.sensitivityOf(ctx, run.WorkspaceID, run.Workflow, run.Version, run.Commit, run.Path)
	if err != nil {
		return workflow.Sensitivity{Classification: workflow.ClassificationSensitive}
	}
	return s
}

// masker is the redactor a finished run is shown through on the owner's
// surfaces, and whether its policy masks at all.
func (r *Runtime) masker(ctx context.Context, run eviedb.WorkflowRun) (eviedb.WorkflowRedactor, bool) {
	if r.retention == nil || !run.State.Terminal() {
		return eviedb.WorkflowRedactor{}, false
	}
	s := r.runSensitivity(ctx, run)
	if !r.retention.For(s.Classification).Mask {
		return eviedb.WorkflowRedactor{}, false
	}
	red := eviedb.NewWorkflowRedactor(s, run.Inputs, eviedb.WorkflowMaskMarker, false)
	return red, red.Active()
}

// present is a run's detail as an owner surface shows it.
func (r *Runtime) present(ctx context.Context, d eviedb.WorkflowRunDetail) eviedb.WorkflowRunDetail {
	if red, ok := r.masker(ctx, d.Run); ok {
		return red.Detail(d)
	}
	return d
}

// presentRun is a run as an owner surface lists it.
func (r *Runtime) presentRun(ctx context.Context, run eviedb.WorkflowRun) eviedb.WorkflowRun {
	if red, ok := r.masker(ctx, run); ok {
		return red.Run(run)
	}
	return run
}

// inputScrubber takes a run's sensitive inputs' values out of what a chat
// or a notice shows, whether or not the run ended and whatever the policy.
func (r *Runtime) inputScrubber(ctx context.Context, run eviedb.WorkflowRun) eviedb.WorkflowRedactor {
	s := r.runSensitivity(ctx, run)
	s.Classification = workflow.ClassificationOrdinary
	return eviedb.NewWorkflowRedactor(s, run.Inputs, eviedb.WorkflowMaskMarker, false)
}

// TabRuns lists a Workspace's latest runs for one of its tabs
// (workspace-tabs.spec.md story 2): as an owner surface shows them, then
// with every sensitive input's value taken out as a chat sees them, and
// without their inputs. A tab is Evie-written code, closer to a chat than to
// the owner's own screen.
func (r *Runtime) TabRuns(ctx context.Context, workspace memory.WorkspaceID, limit int) ([]eviedb.WorkflowRun, error) {
	runs, err := r.store.ListWorkflowRuns(ctx, eviedb.WorkflowRunFilter{WorkspaceID: workspace, Limit: limit})
	if err != nil {
		return nil, err
	}
	for i, run := range runs {
		shown := r.inputScrubber(ctx, run).Run(r.presentRun(ctx, run))
		shown.Inputs = nil
		runs[i] = shown
	}
	return runs, nil
}

// StartRun starts a run as startRun does; the run it returns is shown as
// an owner surface shows it.
func (r *Runtime) StartRun(ctx context.Context, req RunRequest) (RunResult, error) {
	res, err := r.startRun(ctx, req)
	res.Run = r.present(ctx, res.Run)
	return res, err
}

// Cancel records the owner's cancellation of an unfinished run and lets
// the runtime holding it, or the next to claim it, finish it: at once for
// a queued, paused or waiting run, after the node in flight for a running
// one, and once its effect is reconciled for a run with an effect whose
// outcome is unknown. Cancelling a cancelling or cancelled run returns it
// unchanged; a run that succeeded or failed is refused
// (ErrWorkflowRunFinished) and never changed. Only the owner's CLI and web
// surfaces cancel; no model-facing tool does.
func (r *Runtime) Cancel(ctx context.Context, req CancelRequest) (eviedb.WorkflowRunDetail, error) {
	d, err := r.cancelRun(ctx, req)
	return r.present(ctx, d), err
}

// Answer records the owner's answer to the interruption the run waits for
// at the step and lets the run continue. Only the owner's CLI and web
// surfaces answer; no model-facing tool does.
func (r *Runtime) Answer(ctx context.Context, req AnswerRequest) (eviedb.WorkflowRunDetail, error) {
	d, err := r.answerRun(ctx, req)
	return r.present(ctx, d), err
}

// presentInterruptions masks the requests and answers of finished runs'
// interruptions.
func (r *Runtime) presentInterruptions(ctx context.Context, list []eviedb.WorkflowInterruption) ([]eviedb.WorkflowInterruption, error) {
	if r.retention == nil {
		return list, nil
	}
	runs := map[string]eviedb.WorkflowRun{}
	out := make([]eviedb.WorkflowInterruption, len(list))
	for i, in := range list {
		run, ok := runs[in.RunID]
		if !ok {
			var err error
			if run, err = r.store.WorkflowRunRecord(ctx, in.RunID); err != nil {
				return nil, err
			}
			runs[in.RunID] = run
		}
		out[i] = in
		if red, mask := r.masker(ctx, run); mask {
			out[i] = red.Interruptions([]eviedb.WorkflowInterruption{in})[0]
		}
	}
	return out, nil
}

// SweepRequest is the owner's retention sweep from the CLI or the web, or
// a dry run of one, which records nothing.
type SweepRequest struct {
	DryRun  bool   `json:"dry_run,omitempty"`
	Surface string `json:"-"`
}

// SweepResult is what a sweep did (or, dry, would do): its tombstones, the
// runs it kept for now and why, and the versions whose classification it
// could not read.
type SweepResult struct {
	SweepID    string                              `json:"sweep_id"`
	DryRun     bool                                `json:"dry_run,omitempty"`
	At         time.Time                           `json:"at"`
	Tombstones []eviedb.WorkflowRetentionTombstone `json:"tombstones"`
	Skipped    []eviedb.WorkflowRetentionSkip      `json:"skipped,omitempty"`
	Problems   []string                            `json:"problems,omitempty"`
}

// Sweep runs the retention sweep now for the owner, under the runner
// lease another Evie's sweep holds (ErrRetentionBusy); a dry run needs no
// lease and records nothing.
func (r *Runtime) Sweep(ctx context.Context, req SweepRequest) (SweepResult, error) {
	if !ownerSurface(req.Surface) {
		return SweepResult{}, eviedb.ErrWorkflowApprover
	}
	if r.retention == nil {
		return SweepResult{}, ErrRetentionOff
	}
	if req.DryRun {
		return r.sweep(ctx, true, "owner", req.Surface)
	}
	writes := context.WithoutCancel(ctx)
	ok, err := r.store.AcquireWorkflowRetentionRunner(writes, r.holder, retentionRunnerLease)
	if err != nil {
		return SweepResult{}, err
	}
	if !ok {
		return SweepResult{}, ErrRetentionBusy
	}
	defer func() {
		if err := r.store.ReleaseWorkflowRetentionRunner(writes, r.holder); err != nil {
			r.reportError("retention", err)
		}
	}()
	res, err := r.sweep(writes, false, "owner", req.Surface)
	if err != nil {
		return res, err
	}
	return res, r.recordSweep(writes, res)
}

// tickRetention is the periodic sweep: it runs only while this runtime
// holds the runner lease, and gives it up afterwards.
func (r *Runtime) tickRetention(ctx context.Context) error {
	if r.retention == nil {
		return nil
	}
	writes := context.WithoutCancel(ctx)
	ok, err := r.store.AcquireWorkflowRetentionRunner(writes, r.holder, retentionRunnerLease)
	if err != nil || !ok {
		return err
	}
	defer func() {
		if err := r.store.ReleaseWorkflowRetentionRunner(writes, r.holder); err != nil {
			r.reportError("retention", err)
		}
	}()
	// A stopping runtime ends the sweep between actions.
	res, err := r.sweep(ctx, false, "runtime", "")
	if err != nil {
		return err
	}
	return r.recordSweep(writes, res)
}

func (r *Runtime) recordSweep(ctx context.Context, res SweepResult) error {
	summary := struct {
		SweepID    string         `json:"sweep_id"`
		Actions    map[string]int `json:"actions"`
		Skipped    int            `json:"skipped"`
		Problems   []string       `json:"problems,omitempty"`
		Tombstones []int64        `json:"tombstones,omitempty"`
	}{SweepID: res.SweepID, Actions: map[string]int{}, Skipped: len(res.Skipped), Problems: res.Problems}
	for _, t := range res.Tombstones {
		summary.Actions[t.Action+" "+t.DataClass] += t.Records
		summary.Tombstones = append(summary.Tombstones, t.ID)
	}
	data, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	return r.store.RecordWorkflowRetentionSweep(ctx, r.holder, data)
}

// sensitiveValues reports whether a version has anything to redact.
func sensitiveValues(s workflow.Sensitivity) bool {
	return s.Classification == workflow.ClassificationSensitive || len(s.Inputs) > 0
}

// sweep applies every classification's policy once: redaction and
// compaction of finished runs past their windows (at most maxSweepActions
// of them), then deletion of read notices, superseded slots and replaced
// controls, and the redaction of the sensitive classification's slot
// details.
func (r *Runtime) sweep(stop context.Context, dryRun bool, actor, surface string) (SweepResult, error) {
	// Each action is one transaction that is never cancelled midway; stop
	// ending ends the sweep between actions.
	ctx := context.WithoutCancel(stop)
	now := r.now().UTC()
	res := SweepResult{SweepID: "sweep-" + uuid.NewString(), DryRun: dryRun, At: now, Tombstones: []eviedb.WorkflowRetentionTombstone{}}
	record := func(t *eviedb.WorkflowRetentionTombstone, err error) error {
		var skip *eviedb.WorkflowRetentionSkip
		switch {
		case errors.As(err, &skip):
			res.Skipped = append(res.Skipped, *skip)
		case err != nil:
			return err
		case t != nil:
			res.Tombstones = append(res.Tombstones, *t)
		}
		return nil
	}
	reason := func(classification string) string {
		return "the operator's retention policy for the " + classification + " classification"
	}
	actions := 0
	var after int64
	for actions < maxSweepActions {
		candidates, err := r.store.WorkflowRetentionCandidates(ctx, after, 200)
		if err != nil {
			return res, err
		}
		if len(candidates) == 0 {
			break
		}
		for _, c := range candidates {
			after = c.RowID
			if actions >= maxSweepActions || stop.Err() != nil {
				return res, nil
			}
			s, err := r.sensitivityOf(ctx, c.WorkspaceID, c.Workflow, c.Version, c.Commit, c.Path)
			if err != nil {
				res.Problems = append(res.Problems, fmt.Sprintf("run %s is kept: %v", c.RunID, err))
				continue
			}
			p := r.retention.For(s.Classification)
			base := eviedb.WorkflowRetentionRun{RunID: c.RunID, Sensitivity: s, SweepID: res.SweepID, Reason: reason(s.Classification),
				Actor: actor, Surface: surface, DryRun: dryRun}
			if p.Redact != nil && !c.Redacted && sensitiveValues(s) && !c.FinishedAt.After(now.Add(-*p.Redact)) {
				req := base
				req.FinishedBefore, req.Policy = now.Add(-*p.Redact), s.Classification+": "+p.redactText()
				actions++
				if err := record(r.store.RedactWorkflowRun(ctx, req)); err != nil {
					return res, err
				}
			}
			if p.Compact != nil && c.Compactable && !c.FinishedAt.After(now.Add(-*p.Compact)) {
				req := base
				req.FinishedBefore, req.Policy = now.Add(-*p.Compact), s.Classification+": "+p.compactText()
				actions++
				if err := record(r.store.CompactWorkflowRun(ctx, req)); err != nil {
					return res, err
				}
			}
		}
	}
	defs, err := r.store.ListWorkflowDefinitions(ctx, "")
	if err != nil {
		return res, err
	}
	for _, listed := range defs {
		if stop.Err() != nil {
			return res, nil
		}
		// The listing carries no version history; the definition does.
		d, err := r.store.WorkflowDefinition(ctx, listed.WorkspaceID, listed.Name)
		if err != nil {
			return res, err
		}
		versions := map[string][]int64{}
		for _, v := range d.Versions {
			s, err := r.sensitivityOf(ctx, d.WorkspaceID, d.Name, v.Version, v.Commit, v.Path)
			if err != nil {
				res.Problems = append(res.Problems, fmt.Sprintf("%s version %d's records are kept: %v", d.Name, v.Version, err))
				continue
			}
			versions[s.Classification] = append(versions[s.Classification], v.Version)
		}
		for _, classification := range []string{workflow.ClassificationOrdinary, workflow.ClassificationSensitive} {
			list := versions[classification]
			if len(list) == 0 {
				continue
			}
			p := r.retention.For(classification)
			base := eviedb.WorkflowRetentionDeletion{WorkspaceID: d.WorkspaceID, Workflow: d.Name, Versions: list, Classification: classification,
				SweepID: res.SweepID, Reason: reason(classification), Actor: actor, Surface: surface, DryRun: dryRun}
			if p.Notices != nil {
				req := base
				req.Before, req.Policy = now.Add(-*p.Notices), classification+": "+p.noticesText()
				deleted, err := r.store.DeleteWorkflowRecords(ctx, req)
				if err != nil {
					return res, err
				}
				res.Tombstones = append(res.Tombstones, deleted...)
			}
			if classification == workflow.ClassificationSensitive && p.Redact != nil {
				req := base
				req.Before, req.Policy = now.Add(-*p.Redact), classification+": "+p.redactText()
				if err := record(r.store.RedactWorkflowSlots(ctx, req)); err != nil {
					return res, err
				}
			}
		}
	}
	return res, nil
}

// RetentionStatus is the retention policies in force, the sweep's runner
// and latest summary, the totals of every recorded action, and what a
// sweep would do now.
type RetentionStatus struct {
	Policies map[string]string             `json:"policies"`
	State    eviedb.WorkflowRetentionState `json:"state"`
	Due      SweepResult                   `json:"due"`
}

// RetentionStatus reports the retention policies and the sweep's state,
// with a dry run of what is due now.
func (r *Runtime) RetentionStatus(ctx context.Context) (RetentionStatus, error) {
	if r.retention == nil {
		return RetentionStatus{}, ErrRetentionOff
	}
	out := RetentionStatus{Policies: map[string]string{workflow.ClassificationOrdinary: r.retention.Ordinary.String(), workflow.ClassificationSensitive: r.retention.Sensitive.String()}}
	var err error
	if out.State, err = r.store.WorkflowRetentionStatus(ctx); err != nil {
		return out, err
	}
	out.Due, err = r.sweep(ctx, true, "owner", "cli")
	return out, err
}

// retentionText renders what the retention sweep did to a run.
func retentionText(b *strings.Builder, d eviedb.WorkflowRunDetail) {
	for i := len(d.Retention) - 1; i >= 0; i-- {
		t := d.Retention[i]
		verb := map[string]string{eviedb.WorkflowRetentionRedact: "Redacted", eviedb.WorkflowRetentionCompact: "Compacted", eviedb.WorkflowRetentionDelete: "Deleted"}[t.Action]
		by := "the retention sweep"
		if t.Actor == "owner" {
			by = "the owner's sweep via " + t.Surface
		}
		fmt.Fprintf(b, "%s %s (%d records, %s bytes) at %s by %s: %s\n", verb, t.DataClass, t.Records, grouped(t.Bytes), t.RecordedAt.Format(timeLayout), by, t.Policy)
	}
}

// SweepText renders a sweep for the terminal.
func SweepText(res SweepResult) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	if res.DryRun {
		line("Dry run %s at %s: nothing was changed or recorded.", res.SweepID, res.At.Format(timeLayout))
	} else {
		line("Sweep %s at %s.", res.SweepID, res.At.Format(timeLayout))
	}
	if len(res.Tombstones) == 0 {
		line("Nothing is due.")
	}
	for _, t := range res.Tombstones {
		verb := map[string]string{eviedb.WorkflowRetentionRedact: "redact", eviedb.WorkflowRetentionCompact: "compact", eviedb.WorkflowRetentionDelete: "delete"}[t.Action]
		if !res.DryRun {
			verb += "ed"
			if t.Action == eviedb.WorkflowRetentionDelete {
				verb = "deleted"
			}
		}
		subject := t.Workflow + " in " + t.WorkspaceID
		if t.RunID != "" {
			subject = "run " + t.RunID + " (" + subject + ")"
		}
		line("  %s %s of %s: %d records, %s bytes (%s)", verb, t.DataClass, subject, t.Records, grouped(t.Bytes), t.Policy)
	}
	for _, s := range res.Skipped {
		line("  kept run %s for now: %s", s.RunID, s.Reason)
	}
	for _, p := range res.Problems {
		line("  %s", p)
	}
	return b.String()
}
