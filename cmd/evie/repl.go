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
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
)

type replSessionStore interface {
	FindProjectByRoot(context.Context, string) (memory.Project, error)
	RegisterProject(context.Context, string, string) (memory.Project, error)
	CreateProjectSession(context.Context, memory.ProjectID) (memory.Session, error)
	CreateGlobalSession(context.Context) (memory.Session, error)
}

const (
	projectScopeChoice  = "project"
	registerScopeChoice = "register"
	globalScopeChoice   = "global"
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

	project, err := store.FindProjectByRoot(ctx, canonicalRoot)
	if err == nil {
		return selectKnownREPLProject(ctx, store, canonicalRoot, project, scanner, out)
	}
	if !errors.Is(err, eviedb.ErrProjectNotFound) {
		return memory.Session{}, fmt.Errorf("discover project for launch directory: %w", err)
	}

	if _, err := fmt.Fprintf(out, "No active project is registered for %q.\n", canonicalRoot); err != nil {
		return memory.Session{}, fmt.Errorf("write unregistered directory notice: %w", err)
	}
	choice, err := readREPLChoice(
		scanner,
		out,
		"[r]egister this directory or use [g]lobal scope? ",
		"Please enter r or g.\n",
		map[string]string{
			"r":        registerScopeChoice,
			"register": registerScopeChoice,
			"g":        globalScopeChoice,
			"global":   globalScopeChoice,
		},
	)
	if err != nil {
		return memory.Session{}, fmt.Errorf("choose startup scope: %w", err)
	}
	if choice == globalScopeChoice {
		return createGlobalREPLSession(ctx, store)
	}

	defaultName := filepath.Base(canonicalRoot)
	if _, err := fmt.Fprintf(out, "Project name [%s]: ", defaultName); err != nil {
		return memory.Session{}, fmt.Errorf("write project-name prompt: %w", err)
	}
	displayName, err := scanREPLLine(scanner)
	if err != nil {
		return memory.Session{}, fmt.Errorf("read project name: %w", err)
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = defaultName
	}

	project, err = store.RegisterProject(ctx, displayName, canonicalRoot)
	if err != nil {
		concurrentProject, lookupErr := store.FindProjectByRoot(ctx, canonicalRoot)
		if lookupErr == nil {
			return selectKnownREPLProject(ctx, store, canonicalRoot, concurrentProject, scanner, out)
		}
		return memory.Session{}, fmt.Errorf("register launch directory: %w", err)
	}
	session, err := store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		return memory.Session{}, fmt.Errorf("create registered project session: %w", err)
	}
	return session, nil
}

func selectKnownREPLProject(
	ctx context.Context,
	store replSessionStore,
	canonicalRoot string,
	project memory.Project,
	scanner *bufio.Scanner,
	out io.Writer,
) (memory.Session, error) {
	if project.Archived {
		if _, err := fmt.Fprintf(
			out,
			"Launch directory %q belongs to archived project %q; archived projects cannot start sessions.\n",
			canonicalRoot,
			project.DisplayName,
		); err != nil {
			return memory.Session{}, fmt.Errorf("write archived project notice: %w", err)
		}
		_, err := readREPLChoice(
			scanner,
			out,
			"Start a new session with [g]lobal scope? ",
			"Please enter g.\n",
			map[string]string{
				"g":      globalScopeChoice,
				"global": globalScopeChoice,
			},
		)
		if err != nil {
			return memory.Session{}, fmt.Errorf("choose startup scope: %w", err)
		}
		return createGlobalREPLSession(ctx, store)
	}

	if _, err := fmt.Fprintf(out, "Launch directory %q matches active project %q.\n", canonicalRoot, project.DisplayName); err != nil {
		return memory.Session{}, fmt.Errorf("write project suggestion: %w", err)
	}
	choice, err := readREPLChoice(
		scanner,
		out,
		"Start a new session with [p]roject or [g]lobal scope? ",
		"Please enter p or g.\n",
		map[string]string{
			"p":       projectScopeChoice,
			"project": projectScopeChoice,
			"g":       globalScopeChoice,
			"global":  globalScopeChoice,
		},
	)
	if err != nil {
		return memory.Session{}, fmt.Errorf("choose startup scope: %w", err)
	}
	if choice == projectScopeChoice {
		session, err := store.CreateProjectSession(ctx, project.ID)
		if err != nil {
			return memory.Session{}, fmt.Errorf("create project-scoped session: %w", err)
		}
		return session, nil
	}
	return createGlobalREPLSession(ctx, store)
}

func readREPLChoice(
	scanner *bufio.Scanner,
	out io.Writer,
	prompt string,
	invalidMessage string,
	choices map[string]string,
) (string, error) {
	for {
		if _, err := fmt.Fprint(out, prompt); err != nil {
			return "", fmt.Errorf("write prompt: %w", err)
		}
		line, err := scanREPLLine(scanner)
		if err != nil {
			return "", err
		}
		if choice, ok := choices[strings.ToLower(strings.TrimSpace(line))]; ok {
			return choice, nil
		}
		if _, err := fmt.Fprint(out, invalidMessage); err != nil {
			return "", fmt.Errorf("write invalid-choice message: %w", err)
		}
	}
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
	r.deltaIn(text)
}

func (r *replEvents) ReasoningDone() {
	if r.deltaIn == nil {
		return
	}
	r.deltaIn("\x1b[0m")
	r.flush()
	r.deltaIn, r.flush = nil, nil
	_, _ = fmt.Fprint(r.writer(), "\n\n")
}

func (r *replEvents) AssistantDone(content string) {
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
		r.flush()
		r.deltaIn, r.flush = nil, nil
		_, _ = fmt.Fprintln(r.writer())
	}
	r.streamedContent.Reset()
	_, _ = fmt.Fprintln(r.writer(), message)
}

// runREPL is the outer loop: one prompt, one Send, repeat. Turn failures
// print and return to the prompt rather than killing the session.
func runREPL(session *agent.Session, scanner *bufio.Scanner) {
	runREPLContext(context.Background(), session, scanner)
}

func runREPLContext(ctx context.Context, session *agent.Session, scanner *bufio.Scanner) {
	// approve is the terminal half of the write gate: gated tools show
	// what they're about to run and wait for a y/yes before executing.
	// It shares the REPL's scanner — stdin has exactly one reader.
	approve := func(approvalCtx context.Context, name, args string, _ *tools.FileChangePreview) tools.Decision {
		fmt.Printf("\n[%s wants to run]\n%s\napprove? [y/N] ", name, args)
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

	ev := &replEvents{out: os.Stdout}
	for {
		if ctx.Err() != nil {
			return
		}
		fmt.Print("< ")
		if !scanner.Scan() {
			break
		}
		if ctx.Err() != nil {
			return
		}
		if err := session.Send(ctx, scanner.Text(), ev, approve); err != nil {
			fmt.Printf("request failed: %v\n", err)
		}
	}
}
