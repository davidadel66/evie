package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"io"
)

type ownerBatchReviewKernel interface {
	EditOwnerCandidate(context.Context, eviedb.OwnerReviewContext, memory.ReviewEditDecision) (memory.OwnerCandidate, error)
	InspectOwnerCandidateEditRevision(context.Context, eviedb.OwnerReviewContext, string, int64) (memory.ReviewEditRevision, error)
	PrepareOwnerCandidateBatch(context.Context, eviedb.OwnerReviewContext, memory.ReviewBatchRequest) (memory.ReviewBatchPreview, error)
	InspectOwnerCandidateBatch(context.Context, eviedb.OwnerReviewContext, string) (memory.ReviewBatchPreview, error)
	ResolveOwnerCandidateBatch(context.Context, eviedb.OwnerReviewContext, memory.ReviewBatchDecision) (memory.ReviewBatchResult, error)
}

func runOwnerBatchReviewManagement(ctx context.Context, args []string, out io.Writer, kernel ownerReviewKernel) (bool, error) {
	if len(args) < 2 || args[0] != "memory-review" {
		return false, nil
	}
	allowed := map[string]map[string]bool{
		"edit":          {"scope": true, "id": true, "revision": true, "interpretation": true, "proposal": true, "reason": true},
		"edit-revision": {"scope": true, "id": true, "interpretation": true},
		"batch-prepare": {"scope": true, "request": true},
		"batch-inspect": {"scope": true, "id": true},
		"batch-resolve": {"scope": true, "decision": true},
	}
	fields, ok := allowed[args[1]]
	if !ok {
		return false, nil
	}
	k, ok := kernel.(ownerBatchReviewKernel)
	if !ok {
		return true, errors.New("owner batch review unavailable")
	}
	flags := flag.NewFlagSet(args[1], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	scope := flags.String("scope", "", "one exact destination")
	id := flags.String("id", "", "exact candidate or preview ID")
	revision := flags.Int64("revision", 0, "exact review revision")
	interpretation := flags.Int64("interpretation", 0, "exact interpretation revision")
	proposal := flags.String("proposal", "", "exact edited proposal JSON")
	reason := flags.String("reason", "", "optional owner reason")
	request := flags.String("request", "", "complete ordered groups and dependencies JSON")
	decision := flags.String("decision", "", "exact batch approval JSON")
	if err := flags.Parse(args[2:]); err != nil {
		return true, err
	}
	invalid := false
	flags.Visit(func(f *flag.Flag) {
		if !fields[f.Name] {
			invalid = true
		}
	})
	if invalid || flags.NArg() != 0 || *scope == "" {
		return true, errors.New("review requires exact scope and command-specific flags")
	}
	a, err := kernel.LocalOwnerReviewContext(ctx, *scope)
	if err != nil {
		return true, err
	}
	var result any
	switch args[1] {
	case "edit":
		if *id == "" || *proposal == "" {
			return true, errors.New("edit requires candidate and exact proposal")
		}
		var p memory.ExtractorCandidate
		if err = memory.DecodeCompilerJSON([]byte(*proposal), &p); err != nil {
			return true, err
		}
		result, err = k.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: memory.CandidateRef{ID: *id, InterpretationRevision: *interpretation, ReviewRevision: *revision}, Proposal: p, Reason: *reason})
	case "edit-revision":
		if *id == "" || *interpretation < 1 {
			return true, errors.New("exact edit identity required")
		}
		result, err = k.InspectOwnerCandidateEditRevision(ctx, a, *id, *interpretation)
	case "batch-prepare":
		var r memory.ReviewBatchRequest
		if err = memory.DecodeCompilerJSON([]byte(*request), &r); err != nil {
			return true, err
		}
		result, err = k.PrepareOwnerCandidateBatch(ctx, a, r)
	case "batch-inspect":
		if *id == "" {
			return true, errors.New("exact batch preview required")
		}
		result, err = k.InspectOwnerCandidateBatch(ctx, a, *id)
	case "batch-resolve":
		var d memory.ReviewBatchDecision
		if err = memory.DecodeCompilerJSON([]byte(*decision), &d); err != nil {
			return true, err
		}
		result, err = k.ResolveOwnerCandidateBatch(ctx, a, d)
	}
	if err != nil {
		return true, err
	}
	return true, json.NewEncoder(out).Encode(result)
}
