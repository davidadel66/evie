package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/localextractor"
	"github.com/davidadel66/evie/internal/memory"
)

type compilerManagementKernel interface {
	GetSession(context.Context, memory.SessionID) (memory.Session, error)
	CompileCandidateUnit(context.Context, memory.ScopeContext, memory.CompilationSelection, memory.CompilerGeneration, eviedb.CompilerExtractor) (memory.Compilation, error)
	InspectCompilation(context.Context, memory.ScopeContext, string) (memory.Compilation, error)
	InspectCompilerStatus(context.Context, memory.ScopeContext, string, int) (memory.CompilerStatus, error)
	CancelCompilation(context.Context, memory.ScopeContext, string) (memory.Compilation, error)
	ResumeCompilation(context.Context, memory.ScopeContext, string) (memory.Compilation, error)
}

// The short owner command runs before conversational provider construction.
// Configuration is explicit; this change installs no live model configuration.
func runCompilerManagement(ctx context.Context, args []string, out io.Writer, kernel compilerManagementKernel) (bool, error) {
	if len(args) == 0 || (args[0] != "memory-compile" && args[0] != "memory-candidates") {
		return false, nil
	}
	command := args[0]
	args = args[1:]
	action := "compile"
	if command == "memory-candidates" {
		if len(args) == 0 || (args[0] != "inspect" && args[0] != "status" && args[0] != "cancel" && args[0] != "resume") {
			return true, errors.New("usage: memory-candidates inspect|cancel|resume --session ID --id SELECTION_OR_JOB; status --session ID [--after JOB_ID] [--limit 32]")
		}
		action = args[0]
		args = args[1:]
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sessionID := flags.String("session", "", "source session ID")
	id := flags.String("id", "", "selection or job ID")
	after := flags.String("after", "", "status job cursor")
	limit := flags.Int("limit", 32, "bounded status page size")
	root := flags.String("root", "", "root event ID")
	cutoff := flags.Int64("cutoff", 0, "inclusive captured sequence")
	configPath := flags.String("config", "", "pinned local compiler configuration")
	sessionScope := flags.Bool("session-scope", false, "restrict destination to source session")
	if err := flags.Parse(args); err != nil {
		return true, err
	}
	if flags.NArg() != 0 || *sessionID == "" {
		return true, errors.New("compiler command requires an exact --session and no positional arguments")
	}
	session, err := kernel.GetSession(ctx, memory.SessionID(*sessionID))
	if err != nil {
		return true, err
	}
	owner := session.ScopeContext()
	var result any
	if command == "memory-candidates" {
		if *root != "" || *cutoff != 0 || *configPath != "" || *sessionScope {
			return true, errors.New("candidate management does not accept compile flags")
		}
		if action == "status" {
			if *id != "" {
				return true, errors.New("status accepts --after rather than --id")
			}
			result, err = kernel.InspectCompilerStatus(ctx, owner, *after, *limit)
		} else {
			if *id == "" || *after != "" || *limit != 32 {
				return true, errors.New("inspection/cancel/resume requires --id and no pagination")
			}
			switch action {
			case "inspect":
				result, err = kernel.InspectCompilation(ctx, owner, *id)
			case "cancel":
				result, err = kernel.CancelCompilation(ctx, owner, *id)
			case "resume":
				result, err = kernel.ResumeCompilation(ctx, owner, *id)
			}
		}
	} else {
		if *after != "" || *limit != 32 {
			return true, errors.New("compile does not accept status pagination")
		}
		if *configPath == "" {
			return true, eviedb.ErrCompilerNotConfigured
		}
		if *root == "" || *cutoff <= 0 || *id != "" {
			return true, errors.New("compile requires --root and positive --cutoff")
		}
		file, err := os.Open(*configPath)
		if err != nil {
			return true, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, memory.CompilerMaxBytes+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return true, errors.Join(readErr, closeErr)
		}
		var config localextractor.Config
		if err := memory.DecodeCompilerJSON(data, &config); err != nil {
			return true, fmt.Errorf("read compiler configuration: %w", err)
		}
		extractor, err := localextractor.New(config)
		if err != nil {
			return true, err
		}
		destination := "global"
		if owner.WorkspaceID != "" {
			destination = "workspace:" + string(owner.WorkspaceID)
		} else if owner.ProjectID != "" {
			destination = "project:" + string(owner.ProjectID)
		}
		if *sessionScope {
			destination = "session:" + *sessionID
		}
		result, err = kernel.CompileCandidateUnit(ctx, owner, memory.CompilationSelection{SessionID: session.ID, RootID: memory.EventID(*root), Cutoff: *cutoff, Destination: destination}, config.Generation, extractor)
	}
	if err != nil {
		return true, err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return true, encoder.Encode(result)
}
