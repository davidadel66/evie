// Package workflowruntime is the Workflow Runtime's single entry point
// (workflow-runtime.spec.md). Stage 14 sub-stage 2 gives it activation and
// Standing Authority: propose a definition into procedural Git with its
// validation report and dry run, present the approval card (executable and
// authority diffs against the current version), approve one exact version
// for future runs, deactivate it, and list and show definitions. Sub-stage 3
// adds manual runs of deterministic and branch steps (runs.go); sub-stage 4
// adds fenced run leases, the background scheduler and restart recovery
// (scheduler.go); sub-stage 5 adds capability reads through pinned
// providers (capabilities.go); sub-stage 6 adds capability effects through
// the durable intent-call-receipt-checkpoint protocol and their
// reconciliation (effects.go); sub-stage 7 adds AI steps through pinned
// Model Policies (ai.go, models.go) and durable interruptions answered by
// the owner (interrupts.go); sub-stage 8 adds the owner's cancellation,
// revocation at every node boundary and the audited rerun override
// (cancel.go); sub-stage 9 adds agent steps whose workers run under the
// run's lease (agent.go) and bounded map fan-out with per-item attempts
// (mapstep.go); sub-stage 10 adds bundled templates to proposals, bounded
// agent outputs (runtime /6) and chat starts and status for the Workflows
// plugin (session.go); sub-stage 11 adds approved schedules started
// through StartRun with bounded catch-up (schedules.go), the operator's
// daily cap (caps.go) and durable notifications (notifications.go); later
// sub-stages extend runs through this same type.
//
// Validation runs against the bound Workspace: its current Workspace Access
// and the runtime's capability catalog (the workflow capabilities compiled
// plugins contribute, sub-stage 5) and the operator's worker ceiling. The
// store re-checks access, the Workspace's worker allowances and the
// reviewed version inside the approval transaction, so a change racing the
// approval refuses it rather than slipping past.
//
// Only owner surfaces call Propose, Approve and Deactivate: the CLI and the
// web management API. No model tool reaches them.
package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/google/uuid"
)

// Store is the durable activation state the runtime needs.
type Store interface {
	ListWorkspaces(ctx context.Context, includeArchived bool) ([]memory.Workspace, error)
	WorkspaceAccess(ctx context.Context, id memory.WorkspaceID) (eviedb.WorkspaceAccessRevision, error)
	RecordWorkflowProposal(ctx context.Context, p eviedb.WorkflowProposal) (eviedb.WorkflowProposal, error)
	WorkflowProposal(ctx context.Context, id string) (eviedb.WorkflowProposal, error)
	ActivateWorkflow(ctx context.Context, approval eviedb.WorkflowApproval) (eviedb.WorkflowActivation, error)
	DeactivateWorkflow(ctx context.Context, ws memory.WorkspaceID, name string, version int64, reason, surface string) (eviedb.WorkflowActivation, error)
	WorkflowDefinition(ctx context.Context, ws memory.WorkspaceID, name string) (eviedb.WorkflowDefinitionState, error)
	ListWorkflowDefinitions(ctx context.Context, ws memory.WorkspaceID) ([]eviedb.WorkflowDefinitionState, error)
	ActiveWorkflow(ctx context.Context, ws memory.WorkspaceID, name string) (eviedb.WorkflowActivation, error)

	// Built-in and Global workflows (2026-10-04): resolution by name for a
	// Workspace, built-in versions and the owner's controls, and Global
	// custom definitions.
	ResolveWorkflow(ctx context.Context, ws memory.WorkspaceID, name string) (eviedb.WorkflowResolution, error)
	RecordWorkflowBuiltinVersion(ctx context.Context, v eviedb.WorkflowBuiltinVersion) (eviedb.WorkflowBuiltinVersion, error)
	WorkflowBuiltinVersion(ctx context.Context, name string, version int64) (eviedb.WorkflowBuiltinVersion, error)
	WorkflowBuiltinState(ctx context.Context, name string) (eviedb.WorkflowBuiltinState, error)
	ControlWorkflowBuiltin(ctx context.Context, req eviedb.WorkflowBuiltinControlRequest) (eviedb.WorkflowBuiltinControl, error)
	WorkspaceWorkerBlocks(ctx context.Context, ws memory.WorkspaceID, workers []string) ([]string, error)
	RecordGlobalWorkflowProposal(ctx context.Context, p eviedb.WorkflowProposal) (eviedb.WorkflowProposal, error)
	GlobalWorkflowProposal(ctx context.Context, id string) (eviedb.WorkflowProposal, error)
	ActivateGlobalWorkflow(ctx context.Context, approval eviedb.WorkflowApproval) (eviedb.WorkflowActivation, error)
	DeactivateGlobalWorkflow(ctx context.Context, name string, version int64, reason, surface string) (eviedb.WorkflowActivation, error)
	GlobalWorkflowDefinition(ctx context.Context, name string) (eviedb.WorkflowDefinitionState, error)
	ListGlobalWorkflowDefinitions(ctx context.Context) ([]eviedb.WorkflowDefinitionState, error)

	StartWorkflowRun(ctx context.Context, req eviedb.WorkflowRunStart) (eviedb.WorkflowRun, bool, error)
	WorkflowRunProgress(ctx context.Context, id string) (eviedb.WorkflowRunProgress, error)
	WorkflowRun(ctx context.Context, id string) (eviedb.WorkflowRunDetail, error)
	ListWorkflowRuns(ctx context.Context, f eviedb.WorkflowRunFilter) ([]eviedb.WorkflowRun, error)

	// Leases, recovery and scheduling (sub-stage 4): every write below is
	// fenced by the run's current lease.
	AcquireWorkflowRunLease(ctx context.Context, runID, holder string, d time.Duration) (eviedb.WorkflowRunClaim, error)
	HeartbeatWorkflowRunLease(ctx context.Context, lease eviedb.WorkflowRunLease, d time.Duration) (eviedb.WorkflowRunLease, error)
	ReleaseWorkflowRunLease(ctx context.Context, lease eviedb.WorkflowRunLease, reason string) error
	AdmitWorkflowRun(ctx context.Context, lease eviedb.WorkflowRunLease) error
	ResumeWorkflowRun(ctx context.Context, lease eviedb.WorkflowRunLease) (eviedb.WorkflowRun, error)
	PauseWorkflowRun(ctx context.Context, lease eviedb.WorkflowRunLease, reason string, recheck time.Duration) (eviedb.WorkflowRun, error)
	BeginWorkflowAttempt(ctx context.Context, lease eviedb.WorkflowRunLease, a eviedb.WorkflowAttemptStart) (eviedb.WorkflowNodeAttempt, error)
	FinishWorkflowAttempt(ctx context.Context, lease eviedb.WorkflowRunLease, e eviedb.WorkflowAttemptEnd) (eviedb.WorkflowRun, error)
	FailWorkflowRun(ctx context.Context, lease eviedb.WorkflowRunLease, checkpoint int64, reason string) (eviedb.WorkflowRun, error)
	DueWorkflowRuns(ctx context.Context, limit int) ([]string, error)

	// Capability reads (sub-stage 5): per-node admission, the lease check
	// right before a provider is called, an attempt its holder interrupts
	// when it stops, and audited provider substitutions.
	AdmitWorkflowNode(ctx context.Context, lease eviedb.WorkflowRunLease, n eviedb.WorkflowNodeAdmission) error
	CheckWorkflowRunLease(ctx context.Context, lease eviedb.WorkflowRunLease) error
	InterruptWorkflowAttempt(ctx context.Context, lease eviedb.WorkflowRunLease, seq int64, reason string) error
	RecordWorkflowRunSubstitution(ctx context.Context, sub eviedb.WorkflowRunSubstitution) (eviedb.WorkflowRunSubstitution, error)
	WorkflowRunSubstitutions(ctx context.Context, runID string) ([]eviedb.WorkflowRunSubstitution, error)

	// Effects (sub-stage 6): the Effect Intent recorded with its attempt
	// after per-node admission, max_calls_per_run and Resource Conflict
	// Keys; the provider's answer as the Effect Receipt with the next
	// checkpoint; an unknown outcome; its reconciliation; and the intents
	// and held conflict keys read back.
	BeginWorkflowEffect(ctx context.Context, lease eviedb.WorkflowRunLease, e eviedb.WorkflowEffectStart) (eviedb.WorkflowEffectIntent, error)
	FinishWorkflowEffect(ctx context.Context, lease eviedb.WorkflowRunLease, e eviedb.WorkflowEffectEnd) (eviedb.WorkflowRun, error)
	MarkWorkflowEffectUnknown(ctx context.Context, lease eviedb.WorkflowRunLease, m eviedb.WorkflowEffectUnknownMark) (eviedb.WorkflowRun, error)
	ResolveWorkflowEffect(ctx context.Context, lease eviedb.WorkflowRunLease, r eviedb.WorkflowEffectResolution) (eviedb.WorkflowRun, error)
	DeferWorkflowReconciliation(ctx context.Context, lease eviedb.WorkflowRunLease, reason string, recheck time.Duration) (eviedb.WorkflowRun, error)
	WorkflowEffectIntents(ctx context.Context, runID string) ([]eviedb.WorkflowEffectIntent, error)
	WorkflowEffectConflict(ctx context.Context, runID string, keys []string) (*eviedb.WorkflowEffectConflictError, error)

	// AI steps and interruptions (sub-stage 7): an AI attempt recorded
	// with its model call and finished with its usage; a question asked
	// and answered under the lease; interruptions listed for a Workspace,
	// a session or a run.
	BeginWorkflowModelCall(ctx context.Context, lease eviedb.WorkflowRunLease, m eviedb.WorkflowModelCallStart) (eviedb.WorkflowNodeAttempt, error)
	FinishWorkflowModelCall(ctx context.Context, lease eviedb.WorkflowRunLease, m eviedb.WorkflowModelCallEnd) (eviedb.WorkflowRun, error)
	AskWorkflowQuestion(ctx context.Context, lease eviedb.WorkflowRunLease, q eviedb.WorkflowQuestion) (eviedb.WorkflowInterruption, error)
	AnswerWorkflowQuestion(ctx context.Context, lease eviedb.WorkflowRunLease, a eviedb.WorkflowAnswer) (eviedb.WorkflowRun, error)
	ListWorkflowInterruptions(ctx context.Context, f eviedb.WorkflowInterruptionFilter) ([]eviedb.WorkflowInterruption, error)

	// Cancellation (sub-stage 8): the owner's cancellation recorded without
	// the lease, finished under it, and the run's state read alone while a
	// node's call is in flight.
	RequestWorkflowRunCancellation(ctx context.Context, req eviedb.WorkflowRunCancellationRequest) (eviedb.WorkflowRunCancellation, error)
	FinishWorkflowCancellation(ctx context.Context, lease eviedb.WorkflowRunLease) (eviedb.WorkflowRun, error)
	WorkflowRunState(ctx context.Context, id string) (eviedb.WorkflowRunState, error)

	// Agent steps and map fan-out (sub-stage 9): an agent attempt recorded
	// with its worker call and finished with its outcome; map item
	// attempts begun in batches under the map's attempt and finished one
	// by one; what a step or item ran before at a checkpoint; a run-owned
	// worker execution with its report; the run's spend.
	BeginWorkflowWorkerCall(ctx context.Context, lease eviedb.WorkflowRunLease, w eviedb.WorkflowWorkerCallStart) (eviedb.WorkflowNodeAttempt, error)
	FinishWorkflowWorkerCall(ctx context.Context, lease eviedb.WorkflowRunLease, w eviedb.WorkflowWorkerCallEnd) (eviedb.WorkflowRun, error)
	InterruptWorkflowWorkerCall(ctx context.Context, lease eviedb.WorkflowRunLease, w eviedb.WorkflowWorkerCallEnd, reason string) error
	BeginWorkflowItemAttempts(ctx context.Context, lease eviedb.WorkflowRunLease, mapSeq int64, items []eviedb.WorkflowItemStart) ([]eviedb.WorkflowNodeAttempt, error)
	FinishWorkflowItemAttempt(ctx context.Context, lease eviedb.WorkflowRunLease, e eviedb.WorkflowItemEnd) error
	WorkflowItemAttempts(ctx context.Context, runID string, checkpoint int64, step string) ([]eviedb.WorkflowNodeAttempt, error)
	WorkflowStepWorkerCalls(ctx context.Context, runID string, checkpoint int64, step string, item *int) ([]eviedb.WorkflowWorkerCall, error)
	RunWorkerExecution(ctx context.Context, runID, id string) (delegation.Attempt, string, error)
	WorkflowRunSpend(ctx context.Context, runID string) (eviedb.WorkflowRunSpend, error)

	// Schedules, the daily cap and notifications (sub-stage 11): the
	// schedules of current versions with their cursors, slots recorded
	// without a run, the owner's pause and resume, the ticker's single
	// runner lease, today's spend across all runs, and durable notices.
	WorkflowSchedules(ctx context.Context, ws memory.WorkspaceID) ([]eviedb.WorkflowSchedule, error)
	RecordWorkflowScheduleSlot(ctx context.Context, slot eviedb.WorkflowScheduleSlot) error
	ControlWorkflowSchedule(ctx context.Context, req eviedb.WorkflowScheduleControlRequest) (eviedb.WorkflowScheduleControl, error)
	AcquireWorkflowScheduleRunner(ctx context.Context, holder string, ttl time.Duration) (bool, error)
	ReleaseWorkflowScheduleRunner(ctx context.Context, holder string) error
	WorkflowDailySpend(ctx context.Context, day string) (eviedb.WorkflowDailySpend, error)
	ListWorkflowNotifications(ctx context.Context, f eviedb.WorkflowNotificationFilter) ([]eviedb.WorkflowNotification, error)
	MarkWorkflowNotifications(ctx context.Context, ids []int64, read bool) (int, error)
	MarkAllWorkflowNotificationsRead(ctx context.Context, ws memory.WorkspaceID) (int, error)
	WorkflowNotificationCounts(ctx context.Context, ws memory.WorkspaceID) (eviedb.WorkflowNotificationCounts, error)

	// Migration and retention (sub-stage 12): the owner's migration of an
	// unfinished run to a Successor Run and the record of one; the
	// retention sweep's candidates, its redaction, compaction and deletion
	// with their tombstones, its single runner lease and its state.
	WorkflowRunRecord(ctx context.Context, id string) (eviedb.WorkflowRun, error)
	MigrateWorkflowRun(ctx context.Context, m eviedb.WorkflowMigration) (eviedb.WorkflowRunMigration, error)
	WorkflowRunMigration(ctx context.Context, successorID string) (eviedb.WorkflowRunMigration, bool, error)
	WorkflowRetentionCandidates(ctx context.Context, afterRowID int64, limit int) ([]eviedb.WorkflowRetentionCandidate, error)
	RedactWorkflowRun(ctx context.Context, req eviedb.WorkflowRetentionRun) (*eviedb.WorkflowRetentionTombstone, error)
	CompactWorkflowRun(ctx context.Context, req eviedb.WorkflowRetentionRun) (*eviedb.WorkflowRetentionTombstone, error)
	DeleteWorkflowRecords(ctx context.Context, req eviedb.WorkflowRetentionDeletion) ([]eviedb.WorkflowRetentionTombstone, error)
	RedactWorkflowSlots(ctx context.Context, req eviedb.WorkflowRetentionDeletion) (*eviedb.WorkflowRetentionTombstone, error)
	WorkflowRetentionStatus(ctx context.Context) (eviedb.WorkflowRetentionState, error)
	WorkflowRetentionTombstones(ctx context.Context, f eviedb.WorkflowRetentionTombstoneFilter) ([]eviedb.WorkflowRetentionTombstone, error)
	AcquireWorkflowRetentionRunner(ctx context.Context, holder string, ttl time.Duration) (bool, error)
	ReleaseWorkflowRetentionRunner(ctx context.Context, holder string) error
	RecordWorkflowRetentionSweep(ctx context.Context, holder string, summary json.RawMessage) error
}

// Config wires a Runtime.
type Config struct {
	Store Store
	Repo  *procedural.Repo
	// Capabilities is the workflow capability catalog and its providers:
	// the Plugin Manager in production. nil has no capabilities.
	Capabilities CapabilityProviders
	// Workers is the operator's subagent ceiling bounding agent budgets.
	Workers workflow.WorkerCeiling
	// Scheduler bounds and paces how this runtime executes runs; zero
	// fields take their defaults.
	Scheduler SchedulerConfig
	// Report receives scheduler errors, each distinct one once while it
	// persists; nil drops them.
	Report func(error)
	// Models is the model provider AI steps call (sub-stage 7): the
	// OpenRouter adapter when the owner allowed remote workflow models.
	// nil turns AI steps off; ModelsOff then says why and how to turn them
	// on.
	Models    Models
	ModelsOff string
	// Agents runs agent steps' workers (sub-stage 9): the Kernel subagent
	// supervisor when the owner allowed remote workflow models. nil turns
	// agent steps off; AgentsOff then says why and how to turn them on.
	Agents    Agents
	AgentsOff string
	// DailyCap is the operator's cap on all workflow runs' AI calls and
	// workers in one UTC day (sub-stage 11): every call and worker start
	// is refused, and its run paused until the next day, when it would
	// pass it. nil enforces none; every Evie process sets it from the
	// environment with conservative defaults (cmd/evie).
	DailyCap *eviedb.WorkflowDailyCap
	// Retention is the operator's retention policy for each
	// classification (sub-stage 12): masking on the owner's surfaces, and
	// the periodic sweep's redaction, compaction and deletion. nil masks
	// nothing and sweeps nothing (tests); every Evie process sets it from
	// the environment, with the defaults of DefaultRetention.
	Retention *RetentionConfig
}

// SchedulerConfig bounds and paces a runtime's execution of runs
// (sub-stage 4).
type SchedulerConfig struct {
	// Workers is how many runs this runtime's scheduler advances at once
	// (default 2, at most MaxSchedulerWorkers).
	Workers int
	// Lease is how long a claim on a run lasts without renewal (default
	// 30s); the holder renews it every third of that.
	Lease time.Duration
	// Poll is how often the scheduler looks for due runs (default 1s).
	Poll time.Duration
	// Recheck is how long a paused run waits before its cause is checked
	// again (default 1m).
	Recheck time.Duration
	// Interruptions is how many times one node may be interrupted at one
	// checkpoint (its runtime crashed, stopped or lost the lease before
	// recording the outcome) before the run fails visibly instead of
	// running it again (default 3, at most 10; sub-stage 5).
	Interruptions int
	// ReadTimeout bounds one capability read (default 30s, at most 10m).
	ReadTimeout time.Duration
	// EffectTimeout bounds one effect call or reconciliation call (default
	// 30s, at most 10m; sub-stage 6). A call without an answer by then
	// leaves its outcome unknown.
	EffectTimeout time.Duration
	// ModelTimeout bounds one model request of an AI step (default 2m, at
	// most 10m; sub-stage 7); ModelRetryGap is the first wait before a
	// request that provably generated nothing is sent again (default 2s,
	// doubling, at most two retries and 30s).
	ModelTimeout  time.Duration
	ModelRetryGap time.Duration
	// Schedules is how often the schedule ticker looks for due slots
	// (default 15s, at most 10m; sub-stage 11), and ScheduleGrace how late
	// a slot may first be seen and still count as on time for the skip
	// Catch-up Policy (default 5m, at most 1h).
	Schedules     time.Duration
	ScheduleGrace time.Duration
	// Retention is how often the retention sweep runs (default 1h, at most
	// 24h; sub-stage 12), first at start.
	Retention time.Duration
}

const (
	// DefaultSchedulerWorkers and MaxSchedulerWorkers bound
	// SchedulerConfig.Workers.
	DefaultSchedulerWorkers = 2
	MaxSchedulerWorkers     = 32
	defaultLease            = 30 * time.Second
	defaultPoll             = time.Second
	defaultRecheck          = time.Minute
	// DefaultInterruptions and MaxInterruptions bound
	// SchedulerConfig.Interruptions.
	DefaultInterruptions = 3
	MaxInterruptions     = 10
	defaultReadTimeout   = 30 * time.Second
	maxReadTimeout       = 10 * time.Minute
	defaultModelTimeout  = 2 * time.Minute
	defaultModelRetryGap = 2 * time.Second
)

func (c SchedulerConfig) resolve() (SchedulerConfig, error) {
	if c.Workers < 0 || c.Workers > MaxSchedulerWorkers || c.Lease < 0 || c.Poll < 0 || c.Recheck < 0 {
		return c, fmt.Errorf("workflow scheduler needs 1 to %d workers and positive durations", MaxSchedulerWorkers)
	}
	if c.Interruptions < 0 || c.Interruptions > MaxInterruptions || c.ReadTimeout < 0 || c.ReadTimeout > maxReadTimeout ||
		c.EffectTimeout < 0 || c.EffectTimeout > maxReadTimeout || c.ModelTimeout < 0 || c.ModelTimeout > maxReadTimeout ||
		c.ModelRetryGap < 0 || c.ModelRetryGap > maxModelRetryDelay {
		return c, fmt.Errorf("workflow scheduler needs an interruption bound of 1 to %d, read, effect and model timeouts up to %s and a model retry gap up to %s",
			MaxInterruptions, maxReadTimeout, maxModelRetryDelay)
	}
	if c.Schedules < 0 || c.Schedules > maxReadTimeout || c.ScheduleGrace < 0 || c.ScheduleGrace > time.Hour {
		return c, fmt.Errorf("workflow scheduler needs a schedule tick up to %s and a schedule grace up to 1h", maxReadTimeout)
	}
	if c.Retention < 0 || c.Retention > 24*time.Hour {
		return c, errors.New("workflow scheduler needs a retention sweep interval up to 24h")
	}
	if c.Retention == 0 {
		c.Retention = time.Hour
	}
	if c.Schedules == 0 {
		c.Schedules = defaultScheduleTick
	}
	if c.ScheduleGrace == 0 {
		c.ScheduleGrace = defaultScheduleGrace
	}
	if c.ModelTimeout == 0 {
		c.ModelTimeout = defaultModelTimeout
	}
	if c.ModelRetryGap == 0 {
		c.ModelRetryGap = defaultModelRetryGap
	}
	if c.Interruptions == 0 {
		c.Interruptions = DefaultInterruptions
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = defaultReadTimeout
	}
	if c.EffectTimeout == 0 {
		c.EffectTimeout = defaultReadTimeout
	}
	if c.Workers == 0 {
		c.Workers = DefaultSchedulerWorkers
	}
	if c.Lease == 0 {
		c.Lease = defaultLease
	}
	if c.Poll == 0 {
		c.Poll = defaultPoll
	}
	if c.Recheck == 0 {
		c.Recheck = defaultRecheck
	}
	return c, nil
}

// Runtime is the Workflow Runtime.
type Runtime struct {
	store     Store
	repo      *procedural.Repo
	providers CapabilityProviders
	// catalog is the providers' compiled contracts, for validation.
	catalog workflow.Catalog
	workers workflow.WorkerCeiling
	sched   SchedulerConfig
	report  func(error)
	// models is the model provider AI steps call; nil when they are off,
	// for the reason modelsOff gives (sub-stage 7).
	models    Models
	modelsOff string
	// agents runs agent steps' workers; nil when they are off, for the
	// reason agentsOff gives (sub-stage 9).
	agents    Agents
	agentsOff string
	// holder names this runtime instance on every lease it takes.
	holder string
	// compatible are the pinned runtimes this runtime continues (ADR 0025);
	// pin is the runtime a run it starts pins (RuntimeVersion; tests pin an
	// earlier one to stand for an earlier Evie).
	compatible []string
	pin        string
	// call runs one deterministic step's function; nil is the reviewed
	// function itself. Tests replace it to inject failures, crashes and
	// to observe durable state from inside a node.
	call  func(run string, v *workflow.Validated, step string, input map[string]any) (any, error)
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error

	mu sync.Mutex
	// scheduling is set while Start's scheduler runs in this process.
	scheduling bool
	// active are the runs this process is advancing now.
	active map[string]bool
	// failures are the last error of each run's latest pass here, for
	// waiters; reported are the errors already reported.
	failures map[string]error
	reported map[string]string
	// wakeup prompts the scheduler; changed is closed and replaced each
	// time a pass ends.
	wakeup  chan struct{}
	changed chan struct{}
	// signals reach the passes this process runs when the owner cancels
	// their runs here (sub-stage 8).
	signals map[string]*cancelSignal
	// cap is the operator's daily cap (sub-stage 11); nil enforces none.
	cap *eviedb.WorkflowDailyCap
	// scheduleYield is when this runtime may take the schedule runner
	// lease again after a slot it could not start (sub-stage 11).
	scheduleYield time.Time
	// retention is the operator's retention policy, nil for none, and
	// sensitivities the versions' sensitivity read so far (sub-stage 12).
	retention     *RetentionConfig
	sensitivities map[string]workflow.Sensitivity
	// builtins are this Evie's built-in workflows, validated once and
	// recorded before the first run pins them (2026-10-04).
	builtins   map[string]*builtinEntry
	builtinSet builtinCatalog
}

// New returns a Runtime. It touches neither Git nor the database.
func New(c Config) (*Runtime, error) {
	if c.Store == nil || c.Repo == nil {
		return nil, errors.New("workflow runtime needs a store and a procedural repository")
	}
	if c.Workers.Deadline <= 0 || c.Workers.Tokens <= 0 {
		return nil, errors.New("workflow runtime needs the operator's worker ceiling")
	}
	if c.Capabilities == nil {
		c.Capabilities = ContractsOnly(workflow.NewCatalog())
	}
	sched, err := c.Scheduler.resolve()
	if err != nil {
		return nil, err
	}
	if c.Models == nil && strings.TrimSpace(c.ModelsOff) == "" {
		c.ModelsOff = "AI steps are off in this Evie"
	}
	if c.Agents == nil && strings.TrimSpace(c.AgentsOff) == "" {
		c.AgentsOff = "agent steps are off in this Evie"
	}
	var limit *eviedb.WorkflowDailyCap
	if c.DailyCap != nil {
		if c.DailyCap.Tokens < 0 || c.DailyCap.MicroUSD < 0 {
			return nil, errors.New("the daily workflow cap is not negative")
		}
		copied := *c.DailyCap
		limit = &copied
	}
	var retention *RetentionConfig
	if c.Retention != nil {
		copied := *c.Retention
		retention = &copied
	}
	return &Runtime{store: c.Store, repo: c.Repo, providers: c.Capabilities, catalog: providerCatalog{c.Capabilities}, workers: c.Workers, sched: sched, report: c.Report,
		models: c.Models, modelsOff: c.ModelsOff, agents: c.Agents, agentsOff: c.AgentsOff, cap: limit, retention: retention, sensitivities: map[string]workflow.Sensitivity{},
		builtins: map[string]*builtinEntry{}, builtinSet: embeddedBuiltins(),
		holder: fmt.Sprintf("evie-%d-%s", os.Getpid(), uuid.NewString()), compatible: slices.Clone(compatibleRuntimes), pin: RuntimeVersion,
		now: time.Now, sleep: sleepContext,
		active: map[string]bool{}, failures: map[string]error{}, reported: map[string]string{}, signals: map[string]*cancelSignal{},
		wakeup: make(chan struct{}, 1), changed: make(chan struct{})}, nil
}

// Holder is the name this runtime instance takes leases under.
func (r *Runtime) Holder() string { return r.holder }

func ownerSurface(surface string) bool { return surface == "cli" || surface == "web" }

func (r *Runtime) environment(access workflow.WorkspaceAccess) workflow.Environment {
	return workflow.Environment{Capabilities: r.catalog, Access: access, Workers: r.workers}
}

// Proposal is the owner proposing the definition in Dir for a Workspace,
// or (Stage 14 sub-stage 10) the bundled template named Template.
// Fixtures are the dry run approval needs; a proposal without them can be
// reviewed, and needs none when it only changes documentation. A template
// brings its bundled fixtures when none are given.
type Proposal struct {
	WorkspaceID memory.WorkspaceID
	// Global proposes a Global custom definition (2026-10-04): no
	// Workspace, qualified like a built-in.
	Global   bool
	Dir      string
	Template string
	Fixtures *workflow.Fixtures
	Surface  string
}

// Propose validates the definition against the Workspace's current access,
// simulates it with the fixtures, commits exactly its reviewed files to
// procedural Git, re-validates the commit, records the proposal with its
// report and dry run, and returns its approval card. Nothing is activated.
func (r *Runtime) Propose(ctx context.Context, req Proposal) (Card, error) {
	if !ownerSurface(req.Surface) {
		return Card{}, eviedb.ErrWorkflowApprover
	}
	var src workflow.Source
	origin := ""
	switch dir, template := strings.TrimSpace(req.Dir), strings.TrimSpace(req.Template); {
	case dir != "" && template != "":
		return Card{}, errors.New("propose either a definition directory or a bundled template, not both")
	case template != "":
		bundled, ok := r.builtinSet.source(template)
		if !ok {
			return Card{}, fmt.Errorf("no bundled workflow template named %q (evie workflows builtin list lists the built-ins)", template)
		}
		if req.Fixtures == nil {
			f, err := r.builtinSet.fixtures(template)
			if err != nil {
				return Card{}, err
			}
			req.Fixtures = &f
		}
		src, origin = bundled, template
	case dir != "":
		src = workflow.DirSource{Dir: req.Dir}
	default:
		return Card{}, errors.New("propose needs a definition directory or a bundled template")
	}
	if req.Global {
		if req.WorkspaceID != "" {
			return Card{}, errors.New("a Global definition belongs to no Workspace")
		}
		return r.proposeGlobal(ctx, req, src, origin)
	}
	workspace, err := r.workspace(ctx, req.WorkspaceID)
	if err != nil {
		return Card{}, err
	}
	if workspace.State != memory.WorkspaceActive {
		return Card{}, eviedb.ErrWorkspaceInactive
	}
	access, err := r.store.WorkspaceAccess(ctx, req.WorkspaceID)
	if err != nil {
		return Card{}, err
	}
	env := r.environment(access.Access)
	local, err := workflow.Validate(src, env)
	if err != nil {
		return Card{}, err
	}
	if req.Fixtures != nil {
		// Fixture problems are reported before anything is committed.
		if _, err := local.Simulate(*req.Fixtures); err != nil {
			return Card{}, err
		}
	}
	path := eviedb.WorkflowDefinitionPath(req.WorkspaceID, local.Name())
	authority := workflow.RequirementsOf(local.Report()).AuthorityHash
	message := fmt.Sprintf("Propose workflow %s for Workspace %s\n\nExecutable-Hash: %s\nAuthority-Hash: %s\nProposed-By: owner (%s)\n",
		local.Name(), req.WorkspaceID, local.Hash(), authority, req.Surface)
	if origin != "" {
		message += "Template: " + origin + " (bundled with Evie)\n"
	}
	commit, _, err := r.repo.Commit(ctx, path, local.Files(), message)
	if err != nil {
		return Card{}, err
	}
	committed, err := workflow.Validate(r.repo.Source(ctx, commit, path), env)
	if err != nil {
		return Card{}, fmt.Errorf("the committed definition did not validate: %w", err)
	}
	if committed.Hash() != local.Hash() {
		return Card{}, fmt.Errorf("the committed definition hashes to %s, not %s", committed.Hash(), local.Hash())
	}
	p := eviedb.WorkflowProposal{
		WorkspaceID: req.WorkspaceID, Name: committed.Name(), Commit: commit, Path: path,
		ExecutableHash: committed.Hash(), AuthorityHash: authority, Report: committed.Report(),
		ProposedBy: "owner", Surface: req.Surface,
	}
	if req.Fixtures != nil {
		if p.Simulation, err = committed.Simulate(*req.Fixtures); err != nil {
			return Card{}, err
		}
	}
	recorded, err := r.store.RecordWorkflowProposal(ctx, p)
	if err != nil {
		return Card{}, err
	}
	card, err := r.card(ctx, recorded)
	card.Template = origin
	return card, err
}

// Card is the approval card: what approving a proposal would bind and
// change, and why it would be refused now.
type Card struct {
	Proposal        eviedb.WorkflowProposal    `json:"proposal"`
	Current         *eviedb.WorkflowActivation `json:"current,omitempty"`
	CurrentStatus   eviedb.WorkflowStatus      `json:"current_status,omitempty"`
	CurrentBlocked  []string                   `json:"current_blocked,omitempty"`
	ExpectedVersion int64                      `json:"expected_version"`
	NeedsApproval   bool                       `json:"needs_approval"`
	Diff            workflow.ReviewDiff        `json:"diff"`
	Requirements    workflow.Requirements      `json:"requirements"`
	Refusals        []string                   `json:"refusals,omitempty"`
	Patch           string                     `json:"patch"`
	ApproveCommand  string                     `json:"approve_command,omitempty"`
	// Template names the bundled template a proposal came from, on the card
	// the proposal itself returns (sub-stage 10).
	Template string `json:"template,omitempty"`
}

// Card builds the approval card for a recorded proposal.
func (r *Runtime) Card(ctx context.Context, proposalID string) (Card, error) {
	p, err := r.proposal(ctx, proposalID)
	if err != nil {
		return Card{}, err
	}
	return r.card(ctx, p)
}

func (r *Runtime) card(ctx context.Context, p eviedb.WorkflowProposal) (Card, error) {
	c := Card{Proposal: p, Requirements: workflow.RequirementsOf(p.Report)}
	global := p.Source == eviedb.WorkflowSourceGlobal
	var state eviedb.WorkflowDefinitionState
	var err error
	if global {
		state, err = r.store.GlobalWorkflowDefinition(ctx, p.Name)
	} else {
		state, err = r.store.WorkflowDefinition(ctx, p.WorkspaceID, p.Name)
	}
	if err != nil {
		return Card{}, err
	}
	var base *workflow.Report
	baseCommit := ""
	if cur := state.Current; cur != nil {
		c.Current, c.CurrentStatus, c.CurrentBlocked, c.ExpectedVersion = cur, state.Status, state.Blocked, cur.Version
		known, err := r.proposal(ctx, cur.ProposalID)
		if err != nil {
			return Card{}, err
		}
		base, baseCommit = &known.Report, cur.Commit
	}
	c.Diff = workflow.DiffReports(base, p.Report)
	c.NeedsApproval = !(state.Status == eviedb.WorkflowActive && c.Current.ExecutableHash == p.ExecutableHash && c.Current.AuthorityHash == p.AuthorityHash)
	if c.Patch, err = r.repo.Diff(ctx, baseCommit, p.Commit, p.Path); err != nil {
		return Card{}, err
	}
	if c.NeedsApproval {
		if c.Refusals, err = r.precheck(ctx, p, c.Requirements); err != nil {
			return Card{}, err
		}
		c.ApproveCommand = fmt.Sprintf("evie workflows approve %s --hash %s --authority %s --expect-version %d", p.ID, p.ExecutableHash, p.AuthorityHash, c.ExpectedVersion)
	}
	return c, nil
}

// precheck lists why approving would be refused now. The store's approval
// transaction is what enforces it; this only tells the reviewer first.
func (r *Runtime) precheck(ctx context.Context, p eviedb.WorkflowProposal, req workflow.Requirements) ([]string, error) {
	var out []string
	if p.Simulation == nil {
		out = append(out, "no dry run: propose it again with fixtures before approving")
	} else if p.Simulation.Status != workflow.SimulationSucceeded {
		out = append(out, fmt.Sprintf("the dry run ended %s: approval needs one that succeeded", p.Simulation.Status))
	}
	if p.Source == eviedb.WorkflowSourceGlobal {
		// A Global definition is qualified like a built-in; each
		// Workspace's worker allowance applies when a run starts there.
		out = append(out, workflow.QualifyBuiltin(p.Report)...)
		if _, err := r.revalidate(ctx, p, workflow.WorkspaceAccess{}); err != nil {
			out = append(out, err.Error())
		}
		return out, nil
	}
	w, err := r.workspace(ctx, p.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if w.State != memory.WorkspaceActive {
		return append(out, "the Workspace is archived"), nil
	}
	for _, worker := range req.Workers {
		allowed := false
		switch worker {
		case workflow.WorkerResearch:
			allowed = slices.Contains(w.AllowedPresetIDs, worker)
		case workflow.WorkerExplore:
			allowed = w.Explore.Allowed
		case workflow.WorkerBuild:
			allowed = w.Build.Allowed
		}
		if !allowed {
			out = append(out, fmt.Sprintf("agent steps run %s workers, which this Workspace does not allow; allow them in the Workspace's settings first", worker))
		}
	}
	access, err := r.store.WorkspaceAccess(ctx, p.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out = append(out, workflow.AuthorityOutside(req.Authority, access.Access)...)
	if _, err := r.revalidate(ctx, p, access.Access); err != nil {
		out = append(out, err.Error())
	}
	return out, nil
}

// workspace reads one Workspace with its current revision's settings.
func (r *Runtime) workspace(ctx context.Context, id memory.WorkspaceID) (memory.Workspace, error) {
	workspaces, err := r.store.ListWorkspaces(ctx, true)
	if err != nil {
		return memory.Workspace{}, err
	}
	i := slices.IndexFunc(workspaces, func(w memory.Workspace) bool { return w.ID == id })
	if i < 0 {
		return memory.Workspace{}, fmt.Errorf("%w: %s", eviedb.ErrWorkspaceNotFound, id)
	}
	return workspaces[i], nil
}

// revalidate reads the proposal's exact commit and validates it against the
// current catalog, access and worker ceiling; its hash must be unchanged.
func (r *Runtime) revalidate(ctx context.Context, p eviedb.WorkflowProposal, access workflow.WorkspaceAccess) (*workflow.Validated, error) {
	env := r.environment(access)
	if p.Source == eviedb.WorkflowSourceGlobal {
		env = r.standaloneEnvironment()
	}
	v, err := workflow.Validate(r.repo.Source(ctx, p.Commit, p.Path), env)
	if err != nil {
		return nil, fmt.Errorf("the proposal is no longer valid: %w", err)
	}
	if v.Hash() != p.ExecutableHash {
		return nil, fmt.Errorf("the proposal now hashes to %s, not %s; propose it again", v.Hash(), p.ExecutableHash)
	}
	return v, nil
}

// Approval is the owner approving exactly what a card showed.
type Approval struct {
	ProposalID      string `json:"proposal_id"`
	ExecutableHash  string `json:"executable_hash"`
	AuthorityHash   string `json:"authority_hash"`
	ExpectedVersion int64  `json:"expected_version"`
	Surface         string `json:"-"`
	Detail          string `json:"-"`
}

// Approve re-validates the proposal's exact commit against the Workspace's
// current access, the catalog and the worker ceiling, then records the
// owner's approval, which the store checks again in one transaction. It
// activates the definition, and any schedule it declares, for future runs
// and starts none.
func (r *Runtime) Approve(ctx context.Context, a Approval) (eviedb.WorkflowActivation, error) {
	if !ownerSurface(a.Surface) {
		return eviedb.WorkflowActivation{}, eviedb.ErrWorkflowApprover
	}
	p, err := r.proposal(ctx, a.ProposalID)
	if err != nil {
		return eviedb.WorkflowActivation{}, err
	}
	if a.ExecutableHash != p.ExecutableHash || a.AuthorityHash != p.AuthorityHash {
		return eviedb.WorkflowActivation{}, eviedb.ErrWorkflowHashMismatch
	}
	approval := eviedb.WorkflowApproval{
		ProposalID: a.ProposalID, ExecutableHash: a.ExecutableHash, AuthorityHash: a.AuthorityHash, ExpectedVersion: a.ExpectedVersion,
		Approver: eviedb.WorkflowApprover{Actor: "owner", Surface: a.Surface, Detail: a.Detail},
	}
	if p.Source == eviedb.WorkflowSourceGlobal {
		v, err := r.revalidate(ctx, p, workflow.WorkspaceAccess{})
		if err != nil {
			return eviedb.WorkflowActivation{}, err
		}
		if problems := workflow.QualifyBuiltin(v.Report()); len(problems) > 0 {
			return eviedb.WorkflowActivation{}, &eviedb.WorkflowRefusal{Reasons: problems}
		}
		return r.store.ActivateGlobalWorkflow(ctx, approval)
	}
	access, err := r.store.WorkspaceAccess(ctx, p.WorkspaceID)
	if err != nil {
		return eviedb.WorkflowActivation{}, err
	}
	if _, err := r.revalidate(ctx, p, access.Access); err != nil {
		return eviedb.WorkflowActivation{}, err
	}
	return r.store.ActivateWorkflow(ctx, approval)
}

// Deactivation is the owner revoking a definition's current version.
type Deactivation struct {
	WorkspaceID memory.WorkspaceID `json:"workspace_id"`
	// Global ends the Global custom definition's version instead
	// (2026-10-04); WorkspaceID is then empty.
	Global  bool   `json:"global,omitempty"`
	Name    string `json:"name"`
	Version int64  `json:"version"`
	Reason  string `json:"reason,omitempty"`
	Surface string `json:"-"`
}

// Deactivate ends the definition's current version: no new run starts.
func (r *Runtime) Deactivate(ctx context.Context, d Deactivation) (eviedb.WorkflowActivation, error) {
	if !ownerSurface(d.Surface) {
		return eviedb.WorkflowActivation{}, eviedb.ErrWorkflowApprover
	}
	if d.Global {
		if d.WorkspaceID != "" {
			return eviedb.WorkflowActivation{}, errors.New("a Global definition belongs to no Workspace")
		}
		return r.store.DeactivateGlobalWorkflow(ctx, d.Name, d.Version, d.Reason, d.Surface)
	}
	return r.store.DeactivateWorkflow(ctx, d.WorkspaceID, d.Name, d.Version, d.Reason, d.Surface)
}

// List lists definitions in a Workspace, or every Workspace when ws is "".
func (r *Runtime) List(ctx context.Context, ws memory.WorkspaceID) ([]eviedb.WorkflowDefinitionState, error) {
	return r.store.ListWorkflowDefinitions(ctx, ws)
}

// Show is one definition's status and history.
func (r *Runtime) Show(ctx context.Context, ws memory.WorkspaceID, name string) (eviedb.WorkflowDefinitionState, error) {
	return r.store.WorkflowDefinition(ctx, ws, name)
}

// Text renders the card for the terminal.
func (c Card) Text() string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	p := c.Proposal
	if p.Source == eviedb.WorkflowSourceGlobal {
		line("Global workflow %s: every Workspace without its own approved %s runs it in place of the built-in", p.Name, p.Name)
	} else {
		line("Workflow %s in Workspace %s", p.Name, p.WorkspaceID)
	}
	line("Proposal %s (commit %s), proposed by %s via %s at %s", p.ID, short(p.Commit), p.ProposedBy, p.Surface, p.ProposedAt.Format("2006-01-02 15:04:05Z07:00"))
	if c.Template != "" {
		line("Template: %s (bundled with Evie); its README explains it, and its bundled fixtures are the dry run", c.Template)
	}
	line("Executable hash: %s", p.ExecutableHash)
	line("Authority hash:  %s", p.AuthorityHash)
	if c.Current == nil {
		line("Current version: none approved")
	} else {
		status := string(c.CurrentStatus)
		if len(c.CurrentBlocked) > 0 {
			status += ": " + strings.Join(c.CurrentBlocked, "; ")
		}
		line("Current version: %d (%s), %s", c.Current.Version, status, c.Current.ExecutableHash)
	}
	line("")
	b.WriteString(c.Diff.Text())
	line("")
	if len(c.Requirements.Authority) == 0 {
		line("Requested Standing Authority: none")
	} else {
		line("Requested Standing Authority")
		for _, op := range c.Requirements.Authority {
			line("  %s on %s: %s, at most %d calls per run", op.Capability, op.Connection, strings.Join(op.Resources, ", "), op.MaxCallsPerRun)
		}
	}
	if len(c.Requirements.Workers) > 0 {
		line("Workers: %s (needs the Workspace's allowance for each)", strings.Join(c.Requirements.Workers, ", "))
	}
	if s := c.Requirements.Schedule; s != nil {
		line("Schedule: %s %s, catch-up %s; approving also activates this schedule: its runs start at their times until you pause it (evie workflows schedule pause) or deactivate the workflow", s.Cron, s.Timezone, s.CatchUp)
	} else {
		line("Schedule: none (manual starts only)")
	}
	line("Validation: valid, %d warnings", len(p.Report.Warnings))
	for _, w := range p.Report.Warnings {
		line("  %s", w)
	}
	if sim := p.Simulation; sim != nil {
		summary := fmt.Sprintf("Dry run: %s, %d attempts, %d tokens, %d effect previews (none executed)", sim.Status, sim.Attempts, sim.Tokens, len(sim.Previews))
		if sim.Error != "" {
			summary += ": " + sim.Error
		}
		line("%s", summary)
	} else {
		line("Dry run: none")
	}
	line("")
	switch {
	case !c.NeedsApproval && p.DocumentsVersion > 0:
		line("No approval needed: this changes documentation of active version %d and is recorded.", p.DocumentsVersion)
	case !c.NeedsApproval:
		line("No approval needed: this is the active version.")
	case len(c.Refusals) > 0:
		line("Approval would be refused now:")
		for _, reason := range c.Refusals {
			line("  %s", reason)
		}
	default:
		line("Approving activates exactly this version and its Standing Authority for future runs; it starts no run.")
		line("To approve:")
		line("  %s", c.ApproveCommand)
	}
	if c.Patch != "" {
		line("")
		line("Files")
		b.WriteString(c.Patch)
	}
	return b.String()
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
