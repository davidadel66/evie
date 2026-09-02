// The terminal frontend: an agent.Events implementation over the smooth
// printer plus the y/N approval gate. All terminal I/O for the agent
// lives here — the loop itself is internal/agent and never prints.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

type replSessionStore interface {
	ListProjects(context.Context, bool) ([]memory.Project, error)
	ListWorkspaces(context.Context, bool) ([]memory.Workspace, error)
	ListActiveSessions(context.Context) ([]memory.SessionListing, error)
	FindProjectByRoot(context.Context, string) (memory.Project, error)
	RegisterProject(context.Context, string, string) (memory.Project, error)
	RegisterWorkspace(context.Context, string) (memory.Workspace, error)
	CreateProjectSessionForChooser(context.Context, memory.ProjectID, string, string, memory.ProjectID) (memory.Session, error)
	CreateWorkspaceSessionForChooser(context.Context, memory.WorkspaceID, memory.WorkspaceRevisionID) (memory.Session, error)
	CreateGlobalSessionForChooser(context.Context, string, memory.ProjectID) (memory.Session, error)
	GetActiveSessionForChooser(context.Context, memory.SessionID, string, memory.ProjectID) (memory.Session, error)
}

type replChooserAction struct {
	kind              string
	projectID         memory.ProjectID
	projectRoot       string
	workspaceID       memory.WorkspaceID
	workspaceRevision memory.WorkspaceRevisionID
	workspaceLabel    string
	sessionID         memory.SessionID
}

const (
	replActionNewProject        = "new-project"
	replActionNewGlobal         = "new-global"
	replActionResume            = "resume"
	replActionRegister          = "register"
	replActionNewWorkspace      = "new-workspace"
	replActionRegisterWorkspace = "register-workspace"
	replStateChanged            = "Session choices changed; refreshing."
)

func selectREPLSession(
	ctx context.Context,
	store replSessionStore,
	launchDir string,
	scanner *bufio.Scanner,
	out io.Writer,
) (memory.Session, error) {
	canonicalRoot, err := memory.CanonicalProjectRoot(launchDir)
	if err != nil {
		return memory.Session{}, fmt.Errorf("canonicalize launch directory: %w", err)
	}
	for {
		projects, err := store.ListProjects(ctx, true)
		if err != nil {
			return memory.Session{}, fmt.Errorf("list projects: %w", err)
		}
		sessions, err := store.ListActiveSessions(ctx)
		if err != nil {
			return memory.Session{}, fmt.Errorf("list active sessions: %w", err)
		}
		renderedCWDProjectID := projectOwningRoot(projects, canonicalRoot)
		workspaces, err := store.ListWorkspaces(ctx, true)
		if err != nil {
			return memory.Session{}, fmt.Errorf("list Workspaces: %w", err)
		}
		actions, err := renderREPLChooserWithWorkspaces(out, canonicalRoot, projects, workspaces, sessions)
		if err != nil {
			return memory.Session{}, err
		}

		if _, err := fmt.Fprint(out, "Select session: "); err != nil {
			return memory.Session{}, fmt.Errorf("write chooser prompt: %w", err)
		}
		line, err := scanREPLLine(scanner)
		if err != nil {
			return memory.Session{}, fmt.Errorf("choose startup session: %w", err)
		}
		choice, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || choice < 1 || choice > len(actions) {
			if _, err := fmt.Fprintln(out, "Please enter a listed number."); err != nil {
				return memory.Session{}, fmt.Errorf("write invalid chooser choice: %w", err)
			}
			continue
		}

		action := actions[choice-1]
		switch action.kind {
		case replActionResume:
			session, err := store.GetActiveSessionForChooser(ctx, action.sessionID, canonicalRoot, renderedCWDProjectID)
			if errors.Is(err, eviedb.ErrSessionNotActive) || errors.Is(err, eviedb.ErrChooserStateChanged) {
				if _, writeErr := fmt.Fprintln(out, replStateChanged); writeErr != nil {
					return memory.Session{}, fmt.Errorf("write stale session notice: %w", writeErr)
				}
				continue
			}
			if err != nil {
				return memory.Session{}, fmt.Errorf("resume session: %w", err)
			}
			if session.WorkspaceID != "" {
				if _, err := fmt.Fprintf(out, "Context Scope: Workspace — %s\n", action.workspaceLabel); err != nil {
					return memory.Session{}, fmt.Errorf("write selected Workspace scope: %w", err)
				}
			}
			return session, nil
		case replActionNewWorkspace:
			session, err := store.CreateWorkspaceSessionForChooser(ctx, action.workspaceID, action.workspaceRevision)
			if errors.Is(err, eviedb.ErrChooserStateChanged) {
				if _, writeErr := fmt.Fprintln(out, replStateChanged); writeErr != nil {
					return memory.Session{}, fmt.Errorf("write stale Workspace notice: %w", writeErr)
				}
				continue
			}
			if err != nil {
				return memory.Session{}, fmt.Errorf("create Workspace session: %w", err)
			}
			if _, err := fmt.Fprintf(out, "Context Scope: Workspace — %s\n", action.workspaceLabel); err != nil {
				return memory.Session{}, fmt.Errorf("write selected Workspace scope: %w", err)
			}
			return session, nil
		case replActionNewProject:
			session, err := store.CreateProjectSessionForChooser(
				ctx, action.projectID, action.projectRoot, canonicalRoot, renderedCWDProjectID,
			)
			if errors.Is(err, eviedb.ErrChooserStateChanged) {
				if _, writeErr := fmt.Fprintln(out, replStateChanged); writeErr != nil {
					return memory.Session{}, fmt.Errorf("write stale project notice: %w", writeErr)
				}
				continue
			}
			if err != nil {
				return memory.Session{}, fmt.Errorf("create project session: %w", err)
			}
			return session, nil
		case replActionNewGlobal:
			session, err := createGlobalREPLSession(ctx, store, canonicalRoot, renderedCWDProjectID)
			if errors.Is(err, eviedb.ErrChooserStateChanged) {
				if _, writeErr := fmt.Fprintln(out, replStateChanged); writeErr != nil {
					return memory.Session{}, fmt.Errorf("write stale cwd notice: %w", writeErr)
				}
				continue
			}
			return session, err
		case replActionRegister:
			session, retry, err := registerREPLProject(ctx, store, canonicalRoot, scanner, out)
			if err != nil {
				return memory.Session{}, err
			}
			if retry {
				continue
			}
			return session, nil
		case replActionRegisterWorkspace:
			session, err := registerREPLWorkspace(ctx, store, scanner, out)
			if err != nil {
				return memory.Session{}, err
			}
			return session, nil
		}
	}
}

func renderREPLChooserWithWorkspaces(
	out io.Writer,
	canonicalRoot string,
	projects []memory.Project,
	workspaces []memory.Workspace,
	sessions []memory.SessionListing,
) ([]replChooserAction, error) {
	var ordinarySessions []memory.SessionListing
	workspaceSessions := make(map[memory.WorkspaceID][]memory.SessionListing)
	for _, session := range sessions {
		if session.WorkspaceID == "" {
			ordinarySessions = append(ordinarySessions, session)
		} else {
			workspaceSessions[session.WorkspaceID] = append(workspaceSessions[session.WorkspaceID], session)
		}
	}
	actions, err := renderREPLChooser(out, canonicalRoot, projects, ordinarySessions)
	if err != nil {
		return actions, err
	}
	sort.Slice(workspaces, func(i, j int) bool {
		left, right := replWorkspaceLabel(workspaces[i]), replWorkspaceLabel(workspaces[j])
		if left != right {
			return left < right
		}
		return workspaces[i].ID < workspaces[j].ID
	})
	if _, err := fmt.Fprintln(out, "Workspaces"); err != nil {
		return nil, fmt.Errorf("write Workspace chooser heading: %w", err)
	}
	for _, workspace := range workspaces {
		workspaceSessions := workspaceSessions[workspace.ID]
		if workspace.State == memory.WorkspaceArchived && len(workspaceSessions) == 0 {
			continue
		}
		label := replWorkspaceLabel(workspace)
		archived := ""
		if workspace.State == memory.WorkspaceArchived {
			archived = " (archived)"
		}
		if _, err := fmt.Fprintf(out, "%s%s\n", label, archived); err != nil {
			return nil, fmt.Errorf("write Workspace heading: %w", err)
		}
		if workspace.State == memory.WorkspaceActive {
			actions = append(actions, replChooserAction{kind: replActionNewWorkspace, workspaceID: workspace.ID, workspaceRevision: workspace.CurrentRevisionID, workspaceLabel: label})
			if _, err := fmt.Fprintf(out, "  %d. New session\n", len(actions)); err != nil {
				return nil, fmt.Errorf("write Workspace new-session action: %w", err)
			}
		}
		for _, session := range workspaceSessions {
			actions = append(actions, replChooserAction{kind: replActionResume, sessionID: session.ID, workspaceLabel: label})
			if _, err := fmt.Fprintf(out, "  %d. %s\n", len(actions), replSessionLabel(session.Session)); err != nil {
				return nil, fmt.Errorf("write Workspace session action: %w", err)
			}
		}
	}
	actions = append(actions, replChooserAction{kind: replActionRegisterWorkspace})
	if _, err := fmt.Fprintf(out, "  %d. Register Workspace\n", len(actions)); err != nil {
		return nil, fmt.Errorf("write Workspace registration action: %w", err)
	}
	return actions, nil
}

func replWorkspaceLabel(workspace memory.Workspace) string {
	return memory.WorkspaceDisplayLabel(workspace.DisplayName, workspace.CreatedAt)
}

func registerREPLWorkspace(
	ctx context.Context,
	store replSessionStore,
	scanner *bufio.Scanner,
	out io.Writer,
) (memory.Session, error) {
	if _, err := fmt.Fprint(out, "Workspace name: "); err != nil {
		return memory.Session{}, fmt.Errorf("write Workspace-name prompt: %w", err)
	}
	displayName, err := scanREPLLine(scanner)
	if err != nil {
		return memory.Session{}, fmt.Errorf("read Workspace name: %w", err)
	}
	workspace, err := store.RegisterWorkspace(ctx, strings.TrimSpace(displayName))
	if err != nil {
		return memory.Session{}, fmt.Errorf("register Workspace: %w", err)
	}
	session, err := store.CreateWorkspaceSessionForChooser(ctx, workspace.ID, workspace.CurrentRevisionID)
	if err != nil {
		return memory.Session{}, fmt.Errorf("create registered Workspace session: %w", err)
	}
	if _, err := fmt.Fprintf(out, "Context Scope: Workspace — %s\n", replWorkspaceLabel(workspace)); err != nil {
		return memory.Session{}, fmt.Errorf("write selected Workspace scope: %w", err)
	}
	return session, nil
}

func renderREPLChooser(
	out io.Writer,
	canonicalRoot string,
	projects []memory.Project,
	sessions []memory.SessionListing,
) ([]replChooserAction, error) {
	byProject := make(map[memory.ProjectID][]memory.SessionListing)
	var globals []memory.SessionListing
	for _, session := range sessions {
		if session.ProjectID == "" {
			globals = append(globals, session)
			continue
		}
		byProject[session.ProjectID] = append(byProject[session.ProjectID], session)
	}

	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Archived != projects[j].Archived {
			return !projects[i].Archived
		}
		leftName, rightName := replProjectLabel(projects[i]), replProjectLabel(projects[j])
		if leftName != rightName {
			return leftName < rightName
		}
		leftRoot, rightRoot := escapedREPLPath(projects[i].CanonicalRoot), escapedREPLPath(projects[j].CanonicalRoot)
		if leftRoot != rightRoot {
			return leftRoot < rightRoot
		}
		return projects[i].ID < projects[j].ID
	})

	if _, err := fmt.Fprintln(out, "Sessions"); err != nil {
		return nil, fmt.Errorf("write chooser heading: %w", err)
	}
	ownedCWD := false
	actions := make([]replChooserAction, 0, len(sessions)+len(projects)+2)
	for _, project := range projects {
		projectSessions := byProject[project.ID]
		if project.CanonicalRoot == canonicalRoot {
			ownedCWD = true
		}
		if project.Archived && len(projectSessions) == 0 {
			continue
		}
		name := replProjectLabel(project)
		archived := ""
		if project.Archived {
			archived = " (archived)"
		}
		current := ""
		if !project.Archived && project.CanonicalRoot == canonicalRoot {
			current = " (current directory)"
		}
		if _, err := fmt.Fprintf(out, "%s — %s%s%s\n", name, escapedREPLPath(project.CanonicalRoot), archived, current); err != nil {
			return nil, fmt.Errorf("write project heading: %w", err)
		}
		if !project.Archived {
			actions = append(actions, replChooserAction{
				kind: replActionNewProject, projectID: project.ID, projectRoot: project.CanonicalRoot,
			})
			if _, err := fmt.Fprintf(out, "  %d. New session\n", len(actions)); err != nil {
				return nil, fmt.Errorf("write project new-session action: %w", err)
			}
		}
		for _, session := range projectSessions {
			actions = append(actions, replChooserAction{kind: replActionResume, sessionID: session.ID})
			if _, err := fmt.Fprintf(out, "  %d. %s%s\n", len(actions), replSessionLabel(session.Session), relocatedRootLabel(project, session.Session)); err != nil {
				return nil, fmt.Errorf("write project session action: %w", err)
			}
		}
	}

	if _, err := fmt.Fprintln(out, "Global"); err != nil {
		return nil, fmt.Errorf("write global heading: %w", err)
	}
	actions = append(actions, replChooserAction{kind: replActionNewGlobal})
	if _, err := fmt.Fprintf(out, "  %d. New session\n", len(actions)); err != nil {
		return nil, fmt.Errorf("write global new-session action: %w", err)
	}
	for _, session := range globals {
		actions = append(actions, replChooserAction{kind: replActionResume, sessionID: session.ID})
		if _, err := fmt.Fprintf(out, "  %d. %s\n", len(actions), replSessionLabel(session.Session)); err != nil {
			return nil, fmt.Errorf("write global session action: %w", err)
		}
	}
	if !ownedCWD {
		actions = append(actions, replChooserAction{kind: replActionRegister})
		if _, err := fmt.Fprintf(out, "  %d. Register current directory — %s\n", len(actions), escapedREPLPath(canonicalRoot)); err != nil {
			return nil, fmt.Errorf("write registration action: %w", err)
		}
	}
	return actions, nil
}

func replProjectLabel(project memory.Project) string {
	return memory.ProjectDisplayLabel(project.DisplayName, project.CreatedAt)
}

func projectOwningRoot(projects []memory.Project, root string) memory.ProjectID {
	for _, project := range projects {
		if project.CanonicalRoot == root {
			return project.ID
		}
	}
	return ""
}

func replSessionLabel(session memory.Session) string {
	if title := memory.TerminalSafeLine(session.Title); title != "" {
		return title
	}
	return "Untitled — " + session.CreatedAt.Format(time.RFC3339Nano)
}

func relocatedRootLabel(project memory.Project, session memory.Session) string {
	if session.ProjectRootSnapshot == "" || session.ProjectRootSnapshot == project.CanonicalRoot {
		return ""
	}
	return " (stored root: " + escapedREPLPath(session.ProjectRootSnapshot) + ")"
}

func escapedREPLPath(path string) string { return strconv.Quote(path) }

func registerREPLProject(
	ctx context.Context,
	store replSessionStore,
	canonicalRoot string,
	scanner *bufio.Scanner,
	out io.Writer,
) (memory.Session, bool, error) {
	defaultName := memory.TerminalSafeLine(filepath.Base(canonicalRoot))
	prompt := "Project name: "
	if defaultName != "" {
		prompt = fmt.Sprintf("Project name [%s]: ", defaultName)
	}
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return memory.Session{}, false, fmt.Errorf("write project-name prompt: %w", err)
	}
	displayName, err := scanREPLLine(scanner)
	if err != nil {
		return memory.Session{}, false, fmt.Errorf("read project name: %w", err)
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = defaultName
	}
	project, err := store.RegisterProject(ctx, displayName, canonicalRoot)
	if err != nil {
		if _, lookupErr := store.FindProjectByRoot(ctx, canonicalRoot); lookupErr == nil {
			if _, writeErr := fmt.Fprintln(out, replStateChanged); writeErr != nil {
				return memory.Session{}, false, fmt.Errorf("write concurrent registration notice: %w", writeErr)
			}
			return memory.Session{}, true, nil
		}
		return memory.Session{}, false, fmt.Errorf("register launch directory: %w", err)
	}
	session, err := store.CreateProjectSessionForChooser(
		ctx, project.ID, project.CanonicalRoot, canonicalRoot, project.ID,
	)
	if errors.Is(err, eviedb.ErrChooserStateChanged) {
		if _, writeErr := fmt.Fprintln(out, replStateChanged); writeErr != nil {
			return memory.Session{}, false, fmt.Errorf("write stale registered project notice: %w", writeErr)
		}
		return memory.Session{}, true, nil
	}
	if err != nil {
		return memory.Session{}, false, fmt.Errorf("create registered project session: %w", err)
	}
	return session, false, nil
}

func scanREPLLine(scanner *bufio.Scanner) (string, error) {
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

func createGlobalREPLSession(
	ctx context.Context,
	store replSessionStore,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
) (memory.Session, error) {
	session, err := store.CreateGlobalSessionForChooser(ctx, cwdRoot, expectedCWDProjectID)
	if err != nil {
		return memory.Session{}, fmt.Errorf("create global session: %w", err)
	}
	return session, nil
}

// smoothPrinter decouples token arrival from display so bursty network
// chunks render as steady typing. Deltas go into a channel; a printer
// goroutine drains them into a buffer and prints a few characters per
// tick — more when the buffer is deep, so it catches up instead of
// lagging. Call the returned onDelta from the stream, then done() to
// flush the tail and stop the printer.
func smoothPrinter() (onDelta func(string), done func()) {
	return smoothPrinterTo(os.Stdout)
}

func smoothPrinterTo(out io.Writer) (onDelta func(string), done func()) {
	ch := make(chan string, 64)
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		var buf []rune
		ticker := time.NewTicker(12 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case s, ok := <-ch:
				if !ok {
					_, _ = fmt.Fprint(out, string(buf))
					return
				}
				buf = append(buf, []rune(s)...)
			case <-ticker.C:
				if len(buf) == 0 {
					continue
				}
				n := 1 + len(buf)/20
				if n > len(buf) {
					n = len(buf)
				}
				_, _ = fmt.Fprint(out, string(buf[:n]))
				buf = buf[n:]
			}
		}
	}()

	return func(s string) { ch <- s },
		func() { close(ch); <-finished }
}

// replEvents renders a turn to the terminal. A smooth printer is started
// lazily on the first delta of each assistant message and flushed when
// the message completes, so pacing never leaks across messages.
type replEvents struct {
	deltaIn         func(string)
	flush           func()
	out             io.Writer
	streamedContent strings.Builder
	reasoningOpen   bool
}

const (
	replUnsavedStreamCorrection = "[streamed response above was not saved]"
	replCommittedResponseLabel  = "[committed response]"
	replEmptyCommittedResponse  = "(empty response)"
)

func (r *replEvents) writer() io.Writer {
	if r.out != nil {
		return r.out
	}
	return os.Stdout
}

func (r *replEvents) Delta(text string) {
	if r.deltaIn == nil {
		r.deltaIn, r.flush = smoothPrinterTo(r.writer())
	}
	r.streamedContent.WriteString(text)
	r.deltaIn(text)
}

// Reasoning streams thinking in dim grey through the same smooth printer
// as content — one channel, one goroutine. The prefix goes out once with
// the colour escape so the whole block reads as a scratchpad, and
// ReasoningDone resets the colour, flushes, and leaves a blank line
// before whatever comes next.
func (r *replEvents) Reasoning(text string) {
	if r.deltaIn == nil {
		r.deltaIn, r.flush = smoothPrinterTo(r.writer())
		r.deltaIn("\x1b[90mthinking…\n")
	}
	r.reasoningOpen = true
	r.deltaIn(text)
}

func (r *replEvents) ReasoningDone() {
	r.reasoningOpen = false
	if r.deltaIn == nil {
		return
	}
	r.deltaIn("\x1b[0m")
	r.flush()
	r.deltaIn, r.flush = nil, nil
	_, _ = fmt.Fprint(r.writer(), "\n\n")
}

func (r *replEvents) AssistantDone(content string) {
	r.reasoningOpen = false
	if r.deltaIn != nil {
		r.flush()
		r.deltaIn, r.flush = nil, nil
	}
	streamed := r.streamedContent.String()
	matchesStream := strings.HasPrefix(content, streamed)
	if matchesStream {
		_, _ = fmt.Fprint(r.writer(), content[len(streamed):])
	} else {
		// Terminal output cannot retract divergent streamed bytes. Mark those
		// bytes honestly and print the complete durable response separately;
		// AssistantDone is successful committed content, not a discard event.
		_, _ = fmt.Fprintln(r.writer())
		_, _ = fmt.Fprintln(r.writer(), replUnsavedStreamCorrection)
		_, _ = fmt.Fprintln(r.writer(), replCommittedResponseLabel)
		if content == "" {
			_, _ = fmt.Fprintln(r.writer(), replEmptyCommittedResponse)
		} else {
			_, _ = fmt.Fprintln(r.writer(), content)
		}
	}
	r.streamedContent.Reset()
	if content != "" && matchesStream {
		_, _ = fmt.Fprintln(r.writer())
	}
}

func (r *replEvents) ToolCall(id, name, args string) {
	_, _ = fmt.Fprintf(r.writer(), "[calling %s]\n", name)
}

func (r *replEvents) ToolResult(id, content string, isErr bool) {}

func (r *replEvents) ResponseDiscarded(_ agent.DiscardReason, message string) {
	if r.deltaIn != nil {
		if r.reasoningOpen {
			r.deltaIn("\x1b[0m")
		}
		r.flush()
		r.deltaIn, r.flush = nil, nil
		_, _ = fmt.Fprintln(r.writer())
	}
	r.reasoningOpen = false
	r.streamedContent.Reset()
	_, _ = fmt.Fprintln(r.writer(), message)
}

// runREPL is the outer loop: one prompt, one Send, repeat. Turn failures
// print and return to the prompt rather than killing the session.
func runREPL(session *agent.Session, scanner *bufio.Scanner) {
	runREPLContext(context.Background(), session, scanner)
}

func runREPLWithMemory(session *agent.Session, scanner *bufio.Scanner, semantic agent.SemanticMemory) {
	runREPLContextIOWithMemory(context.Background(), session, scanner, os.Stdout, semantic)
}

func runREPLContext(ctx context.Context, session *agent.Session, scanner *bufio.Scanner) {
	runREPLContextIO(ctx, session, scanner, os.Stdout)
}

func runREPLContextIO(ctx context.Context, session *agent.Session, scanner *bufio.Scanner, out io.Writer) {
	runREPLContextIOWithMemory(ctx, session, scanner, out, nil)
}

func runREPLContextIOWithMemory(
	ctx context.Context,
	session *agent.Session,
	scanner *bufio.Scanner,
	out io.Writer,
	semantic agent.SemanticMemory,
) {
	// approve is the terminal half of the write gate: gated tools show
	// what they're about to run and wait for a y/yes before executing.
	// It shares the REPL's scanner — stdin has exactly one reader.
	approve := func(approvalCtx context.Context, name, args string, _ *tools.FileChangePreview) tools.Decision {
		_, _ = fmt.Fprintf(out, "\n[%s wants to run]\n%s\napprove? [y/N] ", name, args)
		if !scanner.Scan() {
			return tools.Declined
		}
		if approvalCtx.Err() != nil {
			return tools.Expired
		}
		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if answer == "y" || answer == "yes" {
			return tools.Approved
		}
		return tools.Declined
	}

	ev := &replEvents{out: out}
	for {
		if ctx.Err() != nil {
			return
		}
		_, _ = fmt.Fprint(out, "< ")
		if !scanner.Scan() {
			break
		}
		if ctx.Err() != nil {
			return
		}
		input := scanner.Text()
		if input == "/memory" {
			inspection, err := session.InspectSemanticMemory(ctx, semantic)
			if err != nil {
				_, _ = fmt.Fprintf(out, "memory inspection failed: %v\n", err)
				continue
			}
			writeMemoryInspection(out, inspection)
			continue
		}
		if input == "/remember" {
			_, _ = fmt.Fprintln(out, "Usage: /remember <predicate> <text>")
			continue
		}
		if strings.HasPrefix(input, "/remember ") || strings.HasPrefix(input, "/remember\t") {
			arguments := strings.Fields(strings.TrimSpace(strings.TrimPrefix(input, "/remember")))
			if len(arguments) < 2 {
				_, _ = fmt.Fprintln(out, "Usage: /remember <predicate> <text>")
				continue
			}
			predicate := arguments[0]
			valueStart := strings.Index(strings.TrimSpace(strings.TrimPrefix(input, "/remember")), predicate) + len(predicate)
			remainder := strings.TrimSpace(strings.TrimSpace(strings.TrimPrefix(input, "/remember"))[valueStart:])
			id, err := uuid.NewRandom()
			if err != nil {
				_, _ = fmt.Fprintf(out, "remember failed: generate idempotency key: %v\n", err)
				continue
			}
			proposal, err := session.PrepareRememberLiteral(ctx, semantic, input, memory.RememberLiteralRequest{
				IdempotencyKey: "idem:v1:" + id.String(), Predicate: predicate,
				PredicateLabel: strings.ReplaceAll(predicate, "_", " "),
				Literal:        memory.TypedLiteral{Kind: memory.LiteralText, Value: remainder},
			})
			if err != nil {
				_, _ = fmt.Fprintf(out, "remember failed: %v\n", err)
				continue
			}
			writeRememberProposal(out, proposal)
			decision := approve(ctx, "memory.remember", rememberProposalApprovalJSON(proposal), nil)
			result, err := session.ResolveRememberLiteral(ctx, semantic, proposal, decision)
			if err != nil {
				_, _ = fmt.Fprintf(out, "remember failed: %v\n", err)
				continue
			}
			if decision != tools.Approved {
				_, _ = fmt.Fprintln(out, "Memory proposal declined; Semantic Memory unchanged.")
				continue
			}
			_, _ = fmt.Fprintf(out, "Remembered Claim %s in %s at revision %d (operation %s).\n",
				result.ClaimID, proposal.Scope.Key, result.ScopeRevision, result.OperationID)
			continue
		}
		if input == "/context" {
			diagnostics, err := session.InspectContext(ctx)
			if err != nil {
				_, _ = fmt.Fprintf(out, "context failed: %v\n", err)
				continue
			}
			writeContextDiagnostics(out, diagnostics)
			continue
		}
		if input == "/compact" {
			_, err := session.Compact(ctx)
			switch {
			case errors.Is(err, agent.ErrNothingEligibleForCompaction):
				_, _ = fmt.Fprintln(out, "Nothing eligible for compaction.")
			case errors.Is(err, agent.ErrBusy), errors.Is(err, agent.ErrLeaseConflict):
				_, _ = fmt.Fprintln(out, "Session busy; message not sent.")
			case errors.Is(err, agent.ErrSessionUnavailable):
				_, _ = fmt.Fprintln(out, "Session unavailable; message not sent.")
			case err != nil:
				_, _ = fmt.Fprintf(out, "compaction failed: %v\n", err)
			default:
				_, _ = fmt.Fprintln(out, "Context compacted.")
			}
			continue
		}
		if strings.HasPrefix(input, "/compact ") || strings.HasPrefix(input, "/compact\t") {
			_, _ = fmt.Fprintln(out, "Usage: /compact")
			continue
		}
		err := session.Send(ctx, input, ev, approve)
		switch {
		case errors.Is(err, agent.ErrLeaseConflict):
			_, _ = fmt.Fprintln(out, "Session busy; message not sent.")
		case errors.Is(err, agent.ErrSessionUnavailable):
			_, _ = fmt.Fprintln(out, "Session unavailable; message not sent.")
		case err != nil:
			_, _ = fmt.Fprintf(out, "request failed: %v\n", err)
		}
	}
}

func rememberProposalApprovalJSON(proposal memory.RememberLiteralProposal) string {
	encoded, err := json.Marshal(proposal)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func writeRememberProposal(out io.Writer, proposal memory.RememberLiteralProposal) {
	_, _ = fmt.Fprintln(out, "Memory proposal")
	_, _ = fmt.Fprintf(out, "scope: %s (expected revision %d)\n", proposal.Scope.Key, proposal.ExpectedRevision)
	_, _ = fmt.Fprintf(out, "proposition: %s %s %q (%s, %s)\n",
		proposal.Subject.CanonicalName, proposal.Predicate.Token, proposal.Literal.Value, proposal.Literal.Kind, proposal.Polarity)
	_, _ = fmt.Fprintf(out, "valid time: from=%s to=%s\n", memoryTimeLabel(proposal.ValidTime.From), memoryTimeLabel(proposal.ValidTime.To))
	_, _ = fmt.Fprintf(out, "evidence: event=%s part=%s locator=%s hash=%s observed_at=%s\n",
		proposal.Source.EventID, proposal.Source.EventPart, proposal.Source.LocatorKind,
		proposal.Source.EvidenceSHA256, proposal.Source.ObservedAt)
	_, _ = fmt.Fprintf(out, "generated IDs: operation=%s scope=%s predicate=%s owner=%s evie=%s claim=%s source_link=%s\n",
		proposal.OperationID, proposal.Scope.ID, proposal.Predicate.ID, proposal.Subject.ID,
		proposal.Evie.ID, proposal.ClaimID, proposal.SourceLinkID)
}

func writeMemoryInspection(out io.Writer, inspection memory.LiteralClaimsInspection) {
	_, _ = fmt.Fprintf(out, "Semantic Memory — scope=%s revision=%d effective_at=%s\n",
		inspection.Scope.Key, inspection.ScopeRevision, inspection.EffectiveAt.UTC().Format(time.RFC3339Nano))
	if len(inspection.Claims) == 0 {
		_, _ = fmt.Fprintln(out, "No accepted Claims.")
		return
	}
	for _, claim := range inspection.Claims {
		_, _ = fmt.Fprintf(out, "Claim %s: %s %s %q (%s, %s)\n",
			claim.ID, claim.Subject.CanonicalName, claim.Predicate.Token, claim.Literal.Value,
			claim.Literal.Kind, claim.Polarity)
		_, _ = fmt.Fprintf(out, "  scope=%s operation=%s transaction_time=%s valid_from=%s valid_to=%s\n",
			claim.Scope.Key, claim.OperationID, claim.TransactionTime.UTC().Format(time.RFC3339Nano),
			memoryTimeLabel(claim.ValidTime.From), memoryTimeLabel(claim.ValidTime.To))
		_, _ = fmt.Fprintf(out, "  source=%s event=%s session=%s authority=%s observed_at=%s hash=%s\n",
			claim.Source.ID, claim.Source.EventID, claim.Source.SessionID, claim.Source.Authority,
			claim.Source.ObservedAt, claim.Source.EvidenceSHA256)
	}
}

func memoryTimeLabel(value *time.Time) string {
	if value == nil {
		return "unknown"
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func writeContextDiagnostics(out io.Writer, diagnostics agent.ContextDiagnostics) {
	profile := diagnostics.Profile
	canonicalModel := profile.CanonicalModel
	if canonicalModel == "" {
		canonicalModel = profile.ConfiguredModel
	}
	_, _ = fmt.Fprintln(out, "Context")
	_, _ = fmt.Fprintf(out, "profile: %s\n", profile.Source)
	_, _ = fmt.Fprintf(out, "model: selected=%s canonical=%s\n", profile.ConfiguredModel, canonicalModel)
	_, _ = fmt.Fprintf(
		out,
		"budgets: hard=%d working=%d output_reserve=%d estimation_margin=%d\n",
		profile.HardWindowTokens,
		profile.WorkingTokens,
		profile.OutputReserveTokens,
		profile.EstimationMarginTokens,
	)
	_, _ = fmt.Fprintf(out, "usable input bytes: %d\n", diagnostics.Projection.UsableInputBytes)
	if diagnostics.LatestSnapshot == nil {
		_, _ = fmt.Fprintln(out, "latest snapshot: none")
	} else {
		latest := diagnostics.LatestSnapshot
		manifest := latest.Manifest
		_, _ = fmt.Fprintf(
			out,
			"latest snapshot: id=%s sequence=%d iteration=%d bytes=%d rough_tokens=%d request_sha256=%s\n",
			latest.EventID,
			latest.Sequence,
			manifest.Iteration,
			manifest.SerializedBytes,
			manifest.RoughTokenEstimate,
			manifest.RequestSHA256,
		)
		_, _ = fmt.Fprintf(
			out,
			"latest snapshot bytes: system=%d summary=%d history=%d tools=%d settings=%d\n",
			manifest.SystemMessageBytes,
			manifest.SummaryMessageBytes,
			manifest.HistoryMessageBytes,
			manifest.ToolSchemaBytes,
			manifest.RequestSettingsBytes,
		)
		_, _ = fmt.Fprintf(
			out,
			"latest snapshot counts: messages=%d tools=%d placeholders=%d\n",
			manifest.MessageCount,
			manifest.ToolSchemaCount,
			len(manifest.Placeholders),
		)
		_, _ = fmt.Fprintf(
			out,
			"latest snapshot budgets: source=%s hard=%d working=%d output_reserve=%d estimation_margin=%d usable=%d\n",
			manifest.ProfileSource,
			manifest.HardWindowTokens,
			manifest.WorkingCeilingTokens,
			manifest.OutputReserveTokens,
			manifest.EstimationMarginTokens,
			manifest.UsableInputBytes,
		)
		_, _ = fmt.Fprintf(
			out,
			"latest snapshot frontier: first=%s:%d last=%s:%d active_compaction=%s compaction_failure=%s\n",
			manifest.RetainedFirstEventID,
			manifest.RetainedFirstSequence,
			manifest.RetainedLastEventID,
			manifest.RetainedLastSequence,
			contextDiagnosticValue(string(manifest.ActiveCompactionEventID)),
			contextDiagnosticValue(string(manifest.CompactionFailureCategory)),
		)
		writePlaceholderDiagnostics(out, "latest snapshot", manifest.Placeholders)
	}
	projection := diagnostics.Projection
	_, _ = fmt.Fprintf(
		out,
		"hypothetical projection: bytes=%d rough_tokens=%d messages=%d tools=%d placeholders=%d\n",
		projection.SerializedBytes,
		projection.RoughTokenEstimate,
		projection.MessageCount,
		projection.ToolSchemaCount,
		len(projection.Placeholders),
	)
	_, _ = fmt.Fprintf(
		out,
		"projection bytes: system=%d summary=%d history=%d tools=%d settings=%d\n",
		projection.SystemMessageBytes,
		projection.SummaryMessageBytes,
		projection.HistoryMessageBytes,
		projection.ToolSchemaBytes,
		projection.RequestSettingsBytes,
	)
	writePlaceholderDiagnostics(out, "projection", projection.Placeholders)
	_, _ = fmt.Fprintf(out, "headroom bytes: %d\n", diagnostics.HeadroomBytes)
	_, _ = fmt.Fprintf(
		out,
		"retained frontier: first=%s:%d last=%s:%d\n",
		projection.RetainedFirstEventID,
		projection.RetainedFirstSequence,
		projection.RetainedLastEventID,
		projection.RetainedLastSequence,
	)
	if projection.ActiveCompactionEventID == "" {
		_, _ = fmt.Fprintln(out, "active compaction: none")
	} else {
		_, _ = fmt.Fprintf(out, "active compaction: %s\n", projection.ActiveCompactionEventID)
	}
	for _, warning := range diagnostics.Warnings {
		_, _ = fmt.Fprintf(out, "warning: %s\n", warning)
	}
}

func writePlaceholderDiagnostics(out io.Writer, label string, placeholders []memory.ContextPlaceholderManifest) {
	var originalBytes, projectedBytes int64
	for _, placeholder := range placeholders {
		originalBytes += placeholder.OriginalBytes
		projectedBytes += placeholder.ProjectedBytes
		_, _ = fmt.Fprintf(
			out,
			"%s placeholder: event=%s original_bytes=%d projected_bytes=%d sha256=%s\n",
			label,
			placeholder.EventID,
			placeholder.OriginalBytes,
			placeholder.ProjectedBytes,
			placeholder.SHA256,
		)
	}
	_, _ = fmt.Fprintf(
		out,
		"%s placeholder bytes: original=%d projected=%d saved=%d\n",
		label,
		originalBytes,
		projectedBytes,
		originalBytes-projectedBytes,
	)
}

func contextDiagnosticValue(value string) string {
	if value == "" {
		return "none"
	}
	return value
}
