// Command todo is the owner CLI for Evie's durable SQLite Task Trees.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
	"github.com/google/uuid"
)

const usageText = `todo - manage Evie's durable Task Trees

Usage:
  todo list [--session ID] [--scope context|global] [--root ID] [--parent ID] [--status STATUS ...] [--history]
  todo get [--session ID] [--tree] [--max-depth N] [--history] <id>
  todo add [--session ID] [--scope context|global] [--parent ID --parent-revision N] [--priority N] [--due YYYY-MM-DD] [--desc TEXT] <title>
  todo update [--session ID] [--revision N] [--title TEXT] [--desc TEXT] [--priority N] [--due YYYY-MM-DD] [--result TEXT] [--status STATUS] [--override-reason TEXT] <id>
  todo start|block|done|cancel|reopen [--session ID] [--revision N] <id>
  todo release-claim [--session ID] --reason TEXT <id>
  todo delete [--session ID] [--revision N] <id>  (deprecated: cancels and retains)
  todo help

STATUS is open, in_progress, blocked, completed, or cancelled. A primary
session selects its trusted Workspace or project context; without --session,
commands operate in Global scope. Flags precede positional arguments.`

const todoCLISessionID memory.SessionID = "todo-cli"

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usageText)
		return 2
	}
	if args[0] == "help" {
		fmt.Fprintln(stdout, usageText)
		return 0
	}
	if !knownCommand(args[0]) {
		fmt.Fprintf(stderr, "todo: unknown command %q\n\n%s\n", args[0], usageText)
		return 2
	}

	db, err := eviedb.OpenDBContext(ctx)
	if err != nil {
		return reportError(stderr, fmt.Errorf("open Evie database: %w", err))
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	if _, err := store.ImportDefaultLegacyTodoList(ctx); err != nil {
		return reportError(stderr, fmt.Errorf("import legacy Todo list: %w", err))
	}
	if _, err := store.RecoverInactiveTaskClaims(ctx); err != nil {
		return reportError(stderr, fmt.Errorf("recover inactive Task claims: %w", err))
	}

	switch args[0] {
	case "list":
		return runList(ctx, store, args[1:], stdout, stderr)
	case "get":
		return runGet(ctx, store, args[1:], stdout, stderr)
	case "add":
		return runAdd(ctx, store, args[1:], stdout, stderr)
	case "update":
		return runUpdate(ctx, store, args[1:], stdout, stderr)
	case "start":
		return runLifecycle(ctx, store, args[0], task.StatusInProgress, args[1:], stdout, stderr)
	case "block":
		return runLifecycle(ctx, store, args[0], task.StatusBlocked, args[1:], stdout, stderr)
	case "done":
		return runLifecycle(ctx, store, args[0], task.StatusCompleted, args[1:], stdout, stderr)
	case "cancel":
		return runLifecycle(ctx, store, args[0], task.StatusCancelled, args[1:], stdout, stderr)
	case "reopen":
		return runLifecycle(ctx, store, args[0], task.StatusOpen, args[1:], stdout, stderr)
	case "delete":
		fmt.Fprintln(stderr, "todo: warning: delete is deprecated; cancelling and retaining Task history")
		return runLifecycle(ctx, store, args[0], task.StatusCancelled, args[1:], stdout, stderr)
	case "release-claim":
		return runReleaseClaim(ctx, store, args[1:], stdout, stderr)
	default:
		return 2
	}
}

func knownCommand(command string) bool {
	switch command {
	case "list", "get", "add", "update", "start", "block", "done", "cancel", "reopen", "delete", "release-claim":
		return true
	default:
		return false
	}
}

func commandFlags(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string, stderr io.Writer) bool {
	if err := flags.Parse(args); err != nil {
		fmt.Fprintf(stderr, "todo: %v\n", err)
		return false
	}
	return true
}

func reportUsage(stderr io.Writer, command, synopsis string) int {
	fmt.Fprintf(stderr, "todo: usage: todo %s %s\n", command, synopsis)
	return 2
}

func reportError(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "todo: %v\n", err)
	return 1
}

func ownerContext(ctx context.Context, store *eviedb.Store, sessionID, runID string) (context.Context, error) {
	session, err := ownerSession(ctx, store, sessionID)
	if err != nil {
		return nil, err
	}
	bound := task.WithMutationAttribution(ctx, task.MutationAttribution{
		ActorID: string(memory.LocalOwnerID), SessionID: string(session.ID), RunID: runID,
		WorkspaceID: string(session.WorkspaceID), ProjectID: string(session.ProjectID),
	})
	if sessionID == "" {
		bound = task.WithGlobalScopeCompatibility(bound)
	}
	return bound, nil
}

func ownerSession(ctx context.Context, store *eviedb.Store, sessionID string) (memory.Session, error) {
	if sessionID == "" {
		return store.EnsureGlobalSession(ctx, todoCLISessionID)
	}
	session, err := store.GetActiveSession(ctx, memory.SessionID(sessionID))
	if err != nil {
		return memory.Session{}, fmt.Errorf("select session %q: %w", sessionID, err)
	}
	if session.ParentSessionID != "" {
		return memory.Session{}, fmt.Errorf("select session %q: delegated sessions cannot authorize owner CLI access", sessionID)
	}
	return session, nil
}

func runList(ctx context.Context, store *eviedb.Store, args []string, stdout, stderr io.Writer) int {
	flags := commandFlags("list")
	var sessionID, scope, rootID, parentID string
	var statuses repeatedString
	var history bool
	flags.StringVar(&sessionID, "session", "", "active primary session")
	flags.StringVar(&scope, "scope", "", "context or global")
	flags.StringVar(&rootID, "root", "", "Task Tree root")
	flags.StringVar(&parentID, "parent", "", "direct parent")
	flags.Var(&statuses, "status", "lifecycle status (repeatable)")
	flags.BoolVar(&history, "history", false, "include retained history")
	if !parseFlags(flags, args, stderr) {
		return 2
	}
	if flags.NArg() != 0 {
		return reportUsage(stderr, "list", "[flags]")
	}
	selection, err := parseScope(scope)
	if err != nil {
		return reportError(stderr, err)
	}
	parsedStatuses, err := parseStatuses(statuses)
	if err != nil {
		return reportError(stderr, err)
	}
	if history && len(parsedStatuses) > 0 {
		return reportUsage(stderr, "list", "choose either --history or one or more --status filters")
	}
	bound, err := ownerContext(ctx, store, sessionID, uuid.NewString())
	if err != nil {
		return reportError(stderr, err)
	}
	values, err := store.ListGlobalTasks(bound, task.ListFilter{
		Statuses: parsedStatuses, IncludeHistory: history, RootID: task.ID(rootID), ParentID: task.ID(parentID), Scope: selection,
	})
	if err != nil {
		return reportError(stderr, err)
	}
	if err := renderTaskList(bound, store, stdout, values); err != nil {
		return reportError(stderr, err)
	}
	return 0
}

func runGet(ctx context.Context, store *eviedb.Store, args []string, stdout, stderr io.Writer) int {
	flags := commandFlags("get")
	var sessionID string
	var tree, history bool
	var maxDepth int
	flags.StringVar(&sessionID, "session", "", "active primary session")
	flags.BoolVar(&tree, "tree", false, "render descendants")
	flags.IntVar(&maxDepth, "max-depth", 0, "tree depth bound")
	flags.BoolVar(&history, "history", false, "include retained history")
	if !parseFlags(flags, args, stderr) {
		return 2
	}
	if flags.NArg() != 1 {
		return reportUsage(stderr, "get", "[flags] <id>")
	}
	bound, err := ownerContext(ctx, store, sessionID, uuid.NewString())
	if err != nil {
		return reportError(stderr, err)
	}
	id := task.ID(flags.Arg(0))
	if tree {
		value, err := store.GetGlobalTaskTree(bound, id, task.TreeQuery{MaxDepth: maxDepth, IncludeHistory: history})
		if err != nil {
			return reportError(stderr, err)
		}
		if err := renderTree(bound, store, stdout, value, 0); err != nil {
			return reportError(stderr, err)
		}
		if value.Truncated {
			fmt.Fprintln(stdout, "  [truncated]")
		}
		return 0
	}
	value, err := store.GetGlobalTask(bound, id)
	if err != nil {
		return reportError(stderr, err)
	}
	if err := renderTask(bound, store, stdout, value, 0); err != nil {
		return reportError(stderr, err)
	}
	return 0
}

func runAdd(ctx context.Context, store *eviedb.Store, args []string, stdout, stderr io.Writer) int {
	flags := commandFlags("add")
	var sessionID, scope, parentID, description, dueDate, idempotencyKey string
	var parentRevision uint64
	var priority int
	flags.StringVar(&sessionID, "session", "", "active primary session")
	flags.StringVar(&scope, "scope", "", "context or global")
	flags.StringVar(&parentID, "parent", "", "parent Task")
	flags.Uint64Var(&parentRevision, "parent-revision", 0, "expected parent revision")
	flags.IntVar(&priority, "priority", 0, "priority 0-5")
	flags.StringVar(&dueDate, "due", "", "due date YYYY-MM-DD")
	flags.StringVar(&description, "desc", "", "description")
	flags.StringVar(&idempotencyKey, "idempotency-key", "", "retry identity")
	if !parseFlags(flags, args, stderr) {
		return 2
	}
	if flags.NArg() != 1 {
		return reportUsage(stderr, "add", "[flags] <title>")
	}
	selection, err := parseScope(scope)
	if err != nil {
		return reportError(stderr, err)
	}
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	bound, err := ownerContext(ctx, store, sessionID, uuid.NewString())
	if err != nil {
		return reportError(stderr, err)
	}
	if parentID != "" && parentRevision == 0 {
		parent, err := store.GetGlobalTask(bound, task.ID(parentID))
		if err != nil {
			return reportError(stderr, err)
		}
		parentRevision = parent.Revision
	}
	created, err := store.CreateGlobalTask(bound, task.CreateInput{
		Title: flags.Arg(0), Description: description, Priority: priority, DueDate: dueDate,
		ParentID: task.ID(parentID), ExpectedParentRevision: parentRevision, Scope: selection,
		IdempotencyKey: task.IdempotencyKey(idempotencyKey),
	})
	if err != nil {
		return reportError(stderr, err)
	}
	fmt.Fprintf(stdout, "Added id=%s revision=%d\n", created.ID, created.Revision)
	return 0
}

func runUpdate(ctx context.Context, store *eviedb.Store, args []string, stdout, stderr io.Writer) int {
	flags := commandFlags("update")
	var sessionID, idempotencyKey, overrideReason string
	var revision uint64
	var title, description, dueDate, result, status optionalString
	var priority optionalInt
	flags.StringVar(&sessionID, "session", "", "active primary session")
	flags.Uint64Var(&revision, "revision", 0, "expected revision (defaults to current)")
	flags.Var(&title, "title", "title")
	flags.Var(&description, "desc", "description; empty clears")
	flags.Var(&priority, "priority", "priority 0-5")
	flags.Var(&dueDate, "due", "due date; empty clears")
	flags.Var(&result, "result", "result summary")
	flags.Var(&status, "status", "lifecycle status")
	flags.StringVar(&idempotencyKey, "idempotency-key", "", "retry identity")
	flags.StringVar(&overrideReason, "override-reason", "", "audited recovery reason")
	if !parseFlags(flags, args, stderr) {
		return 2
	}
	if flags.NArg() != 1 {
		return reportUsage(stderr, "update", "[flags] <id>")
	}
	bound, err := ownerContext(ctx, store, sessionID, uuid.NewString())
	if err != nil {
		return reportError(stderr, err)
	}
	id := task.ID(flags.Arg(0))
	if idempotencyKey != "" && revision == 0 {
		return reportUsage(stderr, "update", "--revision is required with --idempotency-key")
	}
	if revision == 0 {
		current, err := store.GetGlobalTask(bound, id)
		if err != nil {
			return reportError(stderr, err)
		}
		revision = current.Revision
	}
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	input := task.UpdateInput{ExpectedRevision: revision, IdempotencyKey: task.IdempotencyKey(idempotencyKey)}
	input.Title = title.pointer()
	input.Description = description.pointer()
	input.Priority = priority.pointer()
	input.DueDate = dueDate.pointer()
	input.ResultSummary = result.pointer()
	if status.set {
		parsed := task.Status(status.value)
		input.Status = &parsed
	}
	updated, err := applyCLIUpdate(ctx, store, sessionID, id, input, overrideReason)
	if err != nil {
		return reportError(stderr, err)
	}
	if err := renderTask(bound, store, stdout, updated, 0); err != nil {
		return reportError(stderr, err)
	}
	return 0
}

func runLifecycle(ctx context.Context, store *eviedb.Store, command string, status task.Status, args []string, stdout, stderr io.Writer) int {
	flags := commandFlags(command)
	var sessionID, idempotencyKey, overrideReason, result string
	var revision uint64
	flags.StringVar(&sessionID, "session", "", "active primary session")
	flags.Uint64Var(&revision, "revision", 0, "expected revision (defaults to current)")
	flags.StringVar(&result, "result", "", "result summary")
	flags.StringVar(&idempotencyKey, "idempotency-key", "", "retry identity")
	flags.StringVar(&overrideReason, "override-reason", "", "audited recovery reason")
	if !parseFlags(flags, args, stderr) {
		return 2
	}
	if flags.NArg() != 1 {
		return reportUsage(stderr, command, "[flags] <id>")
	}
	bound, err := ownerContext(ctx, store, sessionID, uuid.NewString())
	if err != nil {
		return reportError(stderr, err)
	}
	id := task.ID(flags.Arg(0))
	if idempotencyKey != "" && revision == 0 {
		return reportUsage(stderr, command, "--revision is required with --idempotency-key")
	}
	if revision == 0 {
		current, err := store.GetGlobalTask(bound, id)
		if err != nil {
			return reportError(stderr, err)
		}
		revision = current.Revision
	}
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	input := task.UpdateInput{ExpectedRevision: revision, Status: &status, IdempotencyKey: task.IdempotencyKey(idempotencyKey)}
	if result != "" {
		input.ResultSummary = &result
	}
	if command == "delete" && overrideReason == "" {
		overrideReason = "todo_cli_delete"
	}
	updated, err := applyCLIUpdate(ctx, store, sessionID, id, input, overrideReason)
	if err != nil {
		return reportError(stderr, err)
	}
	if err := renderTask(bound, store, stdout, updated, 0); err != nil {
		return reportError(stderr, err)
	}
	return 0
}

func applyCLIUpdate(
	ctx context.Context,
	store *eviedb.Store,
	sessionID string,
	id task.ID,
	input task.UpdateInput,
	reason string,
) (task.Task, error) {
	bound, err := ownerContext(ctx, store, sessionID, uuid.NewString())
	if err != nil {
		return task.Task{}, err
	}
	if reason != "" {
		return store.ManagementUpdateGlobalTask(bound, id, input, reason)
	}
	current, err := store.GetGlobalTask(bound, id)
	if err != nil {
		return task.Task{}, err
	}
	// Durable idempotency replay is decided before revision and claim checks in
	// the store. A stale revision may therefore be an exact retry; let the
	// authoritative service distinguish that from a real conflict without
	// creating or contending for a new execution claim first.
	if input.ExpectedRevision != current.Revision {
		return store.UpdateGlobalTask(bound, id, input)
	}
	_, claimed, err := store.GetGlobalTaskClaim(bound, id)
	if err != nil {
		return task.Task{}, err
	}
	if !cliUpdateNeedsClaim(current, input, claimed) {
		return store.UpdateGlobalTask(bound, id, input)
	}
	return applyClaimedCLIUpdate(ctx, store, sessionID, id, input)
}

func cliUpdateNeedsClaim(current task.Task, input task.UpdateInput, claimed bool) bool {
	if input.ResultSummary != nil {
		return true
	}
	if input.Status == nil {
		return false
	}
	if *input.Status == current.Status {
		return false
	}
	switch *input.Status {
	case task.StatusInProgress, task.StatusBlocked, task.StatusCompleted:
		return true
	case task.StatusOpen:
		return current.Status == task.StatusInProgress || current.Status == task.StatusBlocked
	case task.StatusCancelled:
		return claimed
	default:
		return false
	}
}

func applyClaimedCLIUpdate(
	ctx context.Context,
	store *eviedb.Store,
	sessionID string,
	id task.ID,
	input task.UpdateInput,
) (task.Task, error) {
	runID := uuid.NewString()
	session, err := ownerSession(ctx, store, sessionID)
	if err != nil {
		return task.Task{}, err
	}
	holder := memory.LeaseHolderID("todo-cli:" + runID)
	lease, err := store.AcquireTurnLease(ctx, session.ID, holder, time.Minute)
	if err != nil {
		return task.Task{}, err
	}
	leaseReleased := false
	defer func() {
		if !leaseReleased {
			_ = store.ReleaseTurnLease(context.WithoutCancel(ctx), session.ID, holder, lease.FencingToken)
		}
	}()
	bound := task.WithMutationAttribution(ctx, task.MutationAttribution{
		ActorID: string(memory.LocalOwnerID), SessionID: string(session.ID), RunID: runID,
		WorkspaceID: string(session.WorkspaceID), ProjectID: string(session.ProjectID),
		LeaseHolderID: string(holder), LeaseToken: uint64(lease.FencingToken), LeaseGeneration: uint64(lease.Generation),
	})
	if _, err := store.ClaimGlobalTask(bound, id, task.ClaimInput{IdempotencyKey: task.IdempotencyKey(uuid.NewString())}); err != nil {
		return task.Task{}, err
	}
	claimReleased := false
	defer func() {
		if !claimReleased {
			_, _ = store.ReleaseGlobalTaskClaim(
				context.WithoutCancel(bound), id, task.ReleaseInput{IdempotencyKey: task.IdempotencyKey(uuid.NewString())},
			)
		}
	}()
	updated, err := store.UpdateGlobalTask(bound, id, input)
	if err != nil {
		return task.Task{}, err
	}
	if _, found, err := store.GetGlobalTaskClaim(bound, id); err != nil {
		return task.Task{}, err
	} else if found {
		if _, err := store.ReleaseGlobalTaskClaim(bound, id, task.ReleaseInput{IdempotencyKey: task.IdempotencyKey(uuid.NewString())}); err != nil {
			return task.Task{}, err
		}
	}
	claimReleased = true
	if err := store.ReleaseTurnLease(ctx, session.ID, holder, lease.FencingToken); err != nil {
		return task.Task{}, err
	}
	leaseReleased = true
	return updated, nil
}

func runReleaseClaim(ctx context.Context, store *eviedb.Store, args []string, stdout, stderr io.Writer) int {
	flags := commandFlags("release-claim")
	var sessionID, reason string
	flags.StringVar(&sessionID, "session", "", "active primary session")
	flags.StringVar(&reason, "reason", "", "audited recovery reason")
	if !parseFlags(flags, args, stderr) {
		return 2
	}
	if flags.NArg() != 1 || strings.TrimSpace(reason) == "" {
		return reportUsage(stderr, "release-claim", "[--session ID] --reason TEXT <id>")
	}
	bound, err := ownerContext(ctx, store, sessionID, uuid.NewString())
	if err != nil {
		return reportError(stderr, err)
	}
	released, err := store.OverrideReleaseGlobalTaskClaim(bound, task.ID(flags.Arg(0)), reason)
	if err != nil {
		return reportError(stderr, err)
	}
	fmt.Fprintf(stdout, "Released claim=%s task=%s reason=%q at=%s\n", released.Claim.ID, released.Claim.TaskID, strings.TrimSpace(reason), released.ReleasedAt.Format(time.RFC3339Nano))
	return 0
}

func parseScope(value string) (task.ScopeSelection, error) {
	selection := task.ScopeSelection(value)
	if err := task.ValidateScopeSelection(selection); err != nil {
		return "", err
	}
	return selection, nil
}

func parseStatuses(values []string) ([]task.Status, error) {
	statuses := make([]task.Status, len(values))
	for i, value := range values {
		statuses[i] = task.Status(value)
		if !task.ValidStatus(statuses[i]) {
			return nil, &task.InputError{Field: "status", Message: "must be open, in_progress, blocked, completed, or cancelled"}
		}
	}
	return statuses, nil
}

func renderTaskList(ctx context.Context, store *eviedb.Store, out io.Writer, values []task.Task) error {
	if len(values) == 0 {
		fmt.Fprintln(out, "No tasks.")
		return nil
	}
	depths := make(map[task.ID]int, len(values))
	for _, value := range values {
		depth := 0
		if parentDepth, found := depths[value.ParentID]; value.ParentID != "" && found {
			depth = parentDepth + 1
		}
		depths[value.ID] = depth
		if err := renderTask(ctx, store, out, value, depth); err != nil {
			return err
		}
	}
	return nil
}

func renderTree(ctx context.Context, store *eviedb.Store, out io.Writer, value task.Tree, depth int) error {
	if err := renderTask(ctx, store, out, value.Task, depth); err != nil {
		return err
	}
	for _, child := range value.Children {
		if err := renderTree(ctx, store, out, child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func renderTask(ctx context.Context, store *eviedb.Store, out io.Writer, value task.Task, depth int) error {
	claimText := "-"
	claim, found, err := store.GetGlobalTaskClaim(ctx, value.ID)
	if err != nil {
		return err
	}
	if found {
		claimText = claim.ID + "@" + claim.ClaimedAt.Format(time.RFC3339Nano)
	}
	parent, due := string(value.ParentID), value.DueDate
	if parent == "" {
		parent = "-"
	}
	if due == "" {
		due = "-"
	}
	fmt.Fprintf(out, "%s- status=%s id=%s parent=%s root=%s scope=%s revision=%d priority=%d due=%s claim=%s title=%q",
		strings.Repeat("  ", depth), value.Status, value.ID, parent, value.RootID, value.Scope,
		value.Revision, value.Priority, due, claimText, value.Title)
	if value.Description != "" {
		fmt.Fprintf(out, " description=%q", value.Description)
	}
	if value.ResultSummary != "" {
		fmt.Fprintf(out, " result=%q", value.ResultSummary)
	}
	fmt.Fprintln(out)
	return nil
}

type repeatedString []string

func (v *repeatedString) String() string { return strings.Join(*v, ",") }
func (v *repeatedString) Set(value string) error {
	*v = append(*v, value)
	return nil
}

type optionalString struct {
	value string
	set   bool
}

func (v *optionalString) String() string { return v.value }
func (v *optionalString) Set(value string) error {
	v.value, v.set = value, true
	return nil
}
func (v optionalString) pointer() *string {
	if !v.set {
		return nil
	}
	return &v.value
}

type optionalInt struct {
	value int
	set   bool
}

func (v *optionalInt) String() string { return strconv.Itoa(v.value) }
func (v *optionalInt) Set(value string) error {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	v.value, v.set = parsed, true
	return nil
}
func (v optionalInt) pointer() *int {
	if !v.set {
		return nil
	}
	return &v.value
}
