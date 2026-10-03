package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/localextractor"
	"github.com/davidadel66/evie/internal/memory"
)

type compilerActivationKernel interface {
	GetSession(context.Context, memory.SessionID) (memory.Session, error)
	DisableCompilerActivation(context.Context, memory.ScopeContext, memory.CompilerActivationRequest) (memory.CompilerActivation, error)
	InspectCompilerActivations(context.Context, memory.ScopeContext) (memory.CompilerActivationStatus, error)
}

func readCompilerHostConfiguration(path string) (localextractor.Config, *localextractor.Ollama, error) {
	var config localextractor.Config
	if path == "" {
		return config, nil, eviedb.ErrCompilerNotConfigured
	}
	file, err := os.Open(path)
	if err != nil {
		return config, nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, memory.CompilerMaxBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return config, nil, errors.Join(readErr, closeErr)
	}
	if err := memory.DecodeCompilerJSON(data, &config); err != nil {
		return config, nil, err
	}
	extractor, err := localextractor.New(config)
	return config, extractor, err
}

// errCompilerActivationRetired: the always-on background pass (memory
// architecture Stage 4) replaced per-session compiler activation.
var errCompilerActivationRetired = errors.New("per-conversation compiler activation is retired: the memory background pass now reads every conversation " +
	"(see `evie memory-background status`); existing activations stay readable with `memory-compiler status` and can be closed with `memory-compiler disable`")

// Short activation commands never construct a conversational provider or drain
// extraction. Since the background pass, only status and disable remain:
// existing activations stay readable and closable, and no new per-session
// activation is opened or resumed.
func runCompilerActivationManagement(ctx context.Context, args []string, out io.Writer, kernel compilerActivationKernel) (bool, error) {
	if len(args) == 0 || args[0] != "memory-compiler" {
		return false, nil
	}
	if len(args) < 2 {
		return true, errors.New("usage: memory-compiler status|disable --session ID")
	}
	action := args[1]
	if action == "activate" || action == "resume" {
		return true, errCompilerActivationRetired
	}
	if action != "status" && action != "disable" {
		return true, errors.New("unknown compiler activation action")
	}
	flags := flag.NewFlagSet("memory-compiler", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sessionID := flags.String("session", "", "owner source session")
	requestID := flags.String("request", "", "idempotent operation ID")
	revision := flags.Int64("revision", 0, "expected selector revision")
	id := flags.String("id", "", "exact prior activation ID")
	configPath := flags.String("config", "", "pinned local configuration")
	lineage := flags.Bool("lineage", false, "select all sessions in the exact source lineage")
	sessionScope := flags.Bool("session-scope", false, "select the session destination")
	if err := flags.Parse(args[2:]); err != nil {
		return true, err
	}
	if flags.NArg() != 0 || *sessionID == "" {
		return true, errors.New("compiler activation requires --session and no positional arguments")
	}
	session, err := kernel.GetSession(ctx, memory.SessionID(*sessionID))
	if err != nil {
		return true, err
	}
	owner := session.ScopeContext()
	source := "global"
	if owner.WorkspaceID != "" {
		source = "workspace:" + string(owner.WorkspaceID)
	} else if owner.ProjectID != "" {
		source = "project:" + string(owner.ProjectID)
	}
	selector := memory.CompilerLiveSelector{SourceScope: source, Destination: source, SessionID: owner.SessionID}
	if *lineage {
		selector.SessionID = ""
	}
	if *sessionScope {
		selector.Destination = "session:" + string(owner.SessionID)
	}
	req := memory.CompilerActivationRequest{RequestID: *requestID, ActivationID: *id, ExpectedRevision: *revision, Selector: selector}
	var result any
	if action == "status" {
		if *requestID != "" || *revision != 0 || *id != "" || *configPath != "" || *lineage || *sessionScope {
			return true, errors.New("status accepts only --session")
		}
		result, err = kernel.InspectCompilerActivations(ctx, owner)
	} else {
		if *configPath != "" {
			return true, errors.New("disable does not accept configuration")
		}
		result, err = kernel.DisableCompilerActivation(ctx, owner, req)
	}
	if err != nil {
		return true, err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return true, encoder.Encode(result)
}
