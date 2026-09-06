package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type compilerDiagnosticsKernel interface {
	LocalOwnerReviewContext(context.Context, string) (eviedb.OwnerReviewContext, error)
	ListOwnerCompilerSessions(context.Context, eviedb.OwnerReviewContext, memory.CompilerDiagnosticSessionQuery) (memory.CompilerDiagnosticSessions, error)
	InspectOwnerCompilerDiagnostics(context.Context, eviedb.OwnerReviewContext, memory.CompilerDiagnosticsQuery) (memory.CompilerDiagnostics, error)
}

// Health is a trusted owner command before provider construction. It cannot
// infer acceptance, change configuration, inspect raw SQL, or select history.
func runCompilerDiagnostics(ctx context.Context, args []string, out io.Writer, k compilerDiagnosticsKernel) (bool, error) {
	if len(args) == 0 || args[0] != "memory-health" {
		return false, nil
	}
	flags := flag.NewFlagSet("memory-health", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	scope := flags.String("scope", "", "exact memory destination scope")
	session := flags.String("session", "", "exact source session; omit to list sessions")
	view := flags.String("view", "jobs", "jobs, candidates, activations, history, selections, live_roots, selection, or foreground")
	generation := flags.String("generation", "", "generation identity for selected/outside membership")
	cursor := flags.String("cursor", "", "previous opaque page cursor")
	limit := flags.Int("limit", 32, "maximum page size, 1 through 32")
	if err := flags.Parse(args[1:]); err != nil {
		return true, err
	}
	if flags.NArg() != 0 || *scope == "" {
		return true, errors.New("memory-health requires --scope and no positional arguments")
	}
	if *session == "" && (*view != "jobs" || *generation != "") {
		return true, errors.New("a diagnostic view requires --session")
	}
	a, err := k.LocalOwnerReviewContext(ctx, *scope)
	if err != nil {
		return true, err
	}
	var result any
	if *session == "" {
		result, err = k.ListOwnerCompilerSessions(ctx, a, memory.CompilerDiagnosticSessionQuery{Cursor: *cursor, Limit: *limit})
	} else {
		result, err = k.InspectOwnerCompilerDiagnostics(ctx, a, memory.CompilerDiagnosticsQuery{SessionID: memory.SessionID(*session), View: *view, GenerationID: *generation, Cursor: *cursor, Limit: *limit})
	}
	if err != nil {
		return true, err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return true, encoder.Encode(result)
}
