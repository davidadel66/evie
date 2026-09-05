package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type compilerHistoryKernel interface {
	GetSession(context.Context, memory.SessionID) (memory.Session, error)
	SelectCompilerHistory(context.Context, []memory.ScopeContext, memory.CompilerHistoryRequest, memory.CompilerGeneration, eviedb.CompilerExtractor) (memory.CompilerHistoryReceipt, error)
	CancelCompilerHistory(context.Context, []memory.ScopeContext, memory.CompilerHistoryChange) (memory.CompilerHistoryState, error)
	ResumeCompilerHistory(context.Context, []memory.ScopeContext, memory.CompilerHistoryChange, memory.CompilerGeneration, eviedb.CompilerExtractor) (memory.CompilerHistoryState, error)
	InspectCompilerHistory(context.Context, []memory.ScopeContext, string, int, int64, int) (memory.CompilerHistoryProgress, error)
}

// memory-backfill takes the exact reviewable selection JSON for every command.
// Its source sessions are independently resolved through the trusted local
// Kernel. No active conversational lease or provider is constructed.
func runCompilerHistoryManagement(ctx context.Context, args []string, out io.Writer, kernel compilerHistoryKernel) (bool, error) {
	if len(args) == 0 || args[0] != "memory-backfill" {
		return false, nil
	}
	if len(args) < 2 {
		return true, errors.New("usage: memory-backfill select|status|cancel|resume --selection FILE")
	}
	action := args[1]
	if action != "select" && action != "status" && action != "cancel" && action != "resume" {
		return true, errors.New("unknown history action; skip is not supported")
	}
	flags := flag.NewFlagSet("memory-backfill", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("selection", "", "exact bounded selection JSON")
	configPath := flags.String("config", "", "pinned local configuration")
	operation := flags.String("operation", "", "idempotent cancellation/resume operation ID")
	revision := flags.Int64("revision", 0, "expected request revision")
	rangeIndex := flags.Int("range", 0, "zero-based receipt range index")
	after := flags.Int64("after", 0, "last sequence from the previous progress page")
	limit := flags.Int("limit", 64, "maximum returned intervals, 1 through 64")
	if err := flags.Parse(args[2:]); err != nil {
		return true, err
	}
	if flags.NArg() != 0 || *path == "" {
		return true, errors.New("history requires --selection and no positional arguments")
	}
	file, err := os.Open(*path)
	if err != nil {
		return true, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, memory.CompilerMaxBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return true, errors.Join(readErr, closeErr)
	}
	var req memory.CompilerHistoryRequest
	if err := memory.DecodeCompilerJSON(data, &req); err != nil {
		return true, err
	}
	if len(req.Ranges) < 1 || len(req.Ranges) > 100 {
		return true, errors.New("history requires 1 through 100 exact ranges")
	}
	owners := []memory.ScopeContext{}
	seen := map[memory.SessionID]bool{}
	for _, r := range req.Ranges {
		if seen[r.SessionID] {
			continue
		}
		session, err := kernel.GetSession(ctx, r.SessionID)
		if err != nil {
			return true, err
		}
		owners = append(owners, session.ScopeContext())
		seen[r.SessionID] = true
	}
	if action != "status" && (*rangeIndex != 0 || *after != 0 || *limit != 64) {
		return true, errors.New("pagination applies only to status")
	}
	if (action == "status" || action == "select") && (*operation != "" || *revision != 0) {
		return true, errors.New("operation and revision apply only to cancel/resume")
	}
	var result any
	switch action {
	case "status":
		if *configPath != "" {
			return true, errors.New("status does not accept configuration")
		}
		result, err = kernel.InspectCompilerHistory(ctx, owners, req.RequestID, *rangeIndex, *after, *limit)
	case "cancel":
		if *configPath != "" {
			return true, errors.New("cancel does not accept configuration")
		}
		result, err = kernel.CancelCompilerHistory(ctx, owners, memory.CompilerHistoryChange{RequestID: req.RequestID, OperationID: *operation, ExpectedRevision: *revision})
	default:
		config, extractor, readErr := readCompilerHostConfiguration(*configPath)
		if readErr != nil {
			return true, readErr
		}
		if action == "select" {
			result, err = kernel.SelectCompilerHistory(ctx, owners, req, config.Generation, extractor)
		} else {
			result, err = kernel.ResumeCompilerHistory(ctx, owners, memory.CompilerHistoryChange{RequestID: req.RequestID, OperationID: *operation, ExpectedRevision: *revision}, config.Generation, extractor)
		}
	}
	if err != nil {
		return true, err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return true, encoder.Encode(result)
}
