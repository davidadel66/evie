// The terminal frontend: an agent.Events implementation over the smooth
// printer plus the y/N approval gate. All terminal I/O for the agent
// lives here — the loop itself is internal/agent and never prints.
package main

import (
	"bufio"
	"context"
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
)

type replSessionStore interface {
	ListProjects(context.Context, bool) ([]memory.Project, error)
	ListActiveSessions(context.Context) ([]memory.SessionListing, error)
	FindProjectByRoot(context.Context, string) (memory.Project, error)
	RegisterProject(context.Context, string, string) (memory.Project, error)
	CreateProjectSession(context.Context, memory.ProjectID) (memory.Session, error)
	CreateGlobalSession(context.Context) (memory.Session, error)
	GetActiveSession(context.Context, memory.SessionID) (memory.Session, error)
}

type replChooserAction struct {
	kind      string
	projectID memory.ProjectID
	sessionID memory.SessionID
}

const (
	replActionNewProject = "new-project"
	replActionNewGlobal  = "new-global"
	replActionResume     = "resume"
	replActionRegister   = "register"
	replStateChanged     = "Session choices changed; refreshing."
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
		actions, err := renderREPLChooser(out, canonicalRoot, projects, sessions)
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
			session, err := store.GetActiveSession(ctx, action.sessionID)
			if errors.Is(err, eviedb.ErrSessionNotActive) {
				if _, writeErr := fmt.Fprintln(out, replStateChanged); writeErr != nil {
					return memory.Session{}, fmt.Errorf("write stale session notice: %w", writeErr)
				}
				continue
			}
			if err != nil {
				return memory.Session{}, fmt.Errorf("resume session: %w", err)
			}
			return session, nil
		case replActionNewProject:
			session, err := store.CreateProjectSession(ctx, action.projectID)
			if errors.Is(err, eviedb.ErrProjectNotActive) {
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
			return createGlobalREPLSession(ctx, store)
		case replActionRegister:
			session, retry, err := registerREPLProject(ctx, store, canonicalRoot, scanner, out)
			if err != nil {
				return memory.Session{}, err
			}
			if retry {
				continue
			}
			return session, nil
		}
	}
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
	for _, group := range byProject {
		sortSessionListings(group)
	}
	sortSessionListings(globals)

	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Archived != projects[j].Archived {
			return !projects[i].Archived
		}
		leftName, rightName := memory.TerminalSafeLine(projects[i].DisplayName), memory.TerminalSafeLine(projects[j].DisplayName)
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
		name := memory.TerminalSafeLine(project.DisplayName)
		if name == "" {
			name = "Untitled project"
		}
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
			actions = append(actions, replChooserAction{kind: replActionNewProject, projectID: project.ID})
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

func sortSessionListings(sessions []memory.SessionListing) {
	sort.Slice(sessions, func(i, j int) bool {
		if !sessions[i].ActivityAt.Equal(sessions[j].ActivityAt) {
			return sessions[i].ActivityAt.After(sessions[j].ActivityAt)
		}
		return sessions[i].ID < sessions[j].ID
	})
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
	if defaultName == "" {
		defaultName = "Project"
	}
	if _, err := fmt.Fprintf(out, "Project name [%s]: ", defaultName); err != nil {
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
	session, err := store.CreateProjectSession(ctx, project.ID)
	if errors.Is(err, eviedb.ErrProjectNotActive) {
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

func createGlobalREPLSession(ctx context.Context, store replSessionStore) (memory.Session, error) {
	session, err := store.CreateGlobalSession(ctx)
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

func runREPLContext(ctx context.Context, session *agent.Session, scanner *bufio.Scanner) {
	runREPLContextIO(ctx, session, scanner, os.Stdout)
}

func runREPLContextIO(ctx context.Context, session *agent.Session, scanner *bufio.Scanner, out io.Writer) {
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
		err := session.Send(ctx, scanner.Text(), ev, approve)
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
