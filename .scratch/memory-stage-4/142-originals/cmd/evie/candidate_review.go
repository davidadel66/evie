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

type ownerReviewKernel interface {
	LocalOwnerReviewContext(context.Context, string) (eviedb.OwnerReviewContext, error)
	ListOwnerCandidates(context.Context, eviedb.OwnerReviewContext, memory.OwnerCandidateQuery) (memory.OwnerCandidatePage, error)
	InspectOwnerCandidate(context.Context, eviedb.OwnerReviewContext, string) (memory.OwnerCandidate, error)
	PrepareOwnerCandidateReview(context.Context, eviedb.OwnerReviewContext, memory.CandidateRef, string) (memory.ReviewPreview, error)
	ResolveOwnerCandidateReview(context.Context, eviedb.OwnerReviewContext, memory.ReviewDecision) (memory.ReviewResult, error)
	InspectOwnerReviewOperation(context.Context, eviedb.OwnerReviewContext, memory.SemanticID) (memory.OwnerReviewOperation, error)
}

type ownerIdentityReviewKernel interface {
	InspectOwnerCandidateIdentityRevision(context.Context, eviedb.OwnerReviewContext, string, int64) (memory.ReviewIdentityRevision, error)
	OwnerCandidateIdentityOptions(context.Context, eviedb.OwnerReviewContext, memory.CandidateRef) (memory.ReviewIdentityOptions, error)
	ChooseOwnerCandidateIdentity(context.Context, eviedb.OwnerReviewContext, memory.ReviewIdentityDecision) (memory.OwnerCandidate, error)
}

// Every invocation is a trusted local owner command selecting one exact scope.
// A resolve accepts only immutable preview identity, never replacement effects.
func runOwnerReviewManagement(ctx context.Context, args []string, out io.Writer, kernel ownerReviewKernel) (bool, error) {
	if len(args) == 0 || args[0] != "memory-review" {
		return false, nil
	}
	if len(args) < 2 {
		return true, errors.New("usage: memory-review inbox|inspect|alternatives|choose|identity-revision|prepare|resolve|operation --scope SCOPE")
	}
	command := args[1]
	flags := flag.NewFlagSet("memory-review "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	scope := flags.String("scope", "", "one exact global/workspace/project/session scope")
	id := flags.String("id", "", "candidate or accepted operation ID")
	revision := flags.Int64("revision", 0, "exact candidate review revision")
	interpretation := flags.Int64("interpretation", 0, "exact interpretation revision")
	action := flags.String("action", "", "explicit accept or reject")
	preview := flags.String("preview", "", "frozen preview ID")
	digest := flags.String("digest", "", "frozen preview SHA256")
	delivery := flags.String("delivery", "", "unique idem:v1 UUID delivery key")
	reason := flags.String("reason", "", "optional owner reason")
	limit := flags.Int("limit", 0, "inbox page size, default50 maximum100")
	choices := flags.String("choices", "", "exact identity choices JSON from reviewed alternatives")
	options := flags.String("options", "", "reviewed alternatives SHA256")
	cursor := flags.String("cursor", "", "revision-bound inbox cursor")
	if err := flags.Parse(args[2:]); err != nil {
		return true, err
	}
	if flags.NArg() != 0 || *scope == "" {
		return true, errors.New("review requires explicit --scope and no positional arguments")
	}
	allowed := map[string]map[string]bool{
		"inbox":             {"scope": true, "limit": true, "cursor": true},
		"inspect":           {"scope": true, "id": true},
		"operation":         {"scope": true, "id": true},
		"identity-revision": {"scope": true, "id": true, "interpretation": true},
		"alternatives":      {"scope": true, "id": true, "revision": true, "interpretation": true},
		"choose":            {"scope": true, "id": true, "revision": true, "interpretation": true, "options": true, "choices": true},
		"prepare":           {"scope": true, "id": true, "revision": true, "interpretation": true, "action": true},
		"resolve":           {"scope": true, "preview": true, "digest": true, "delivery": true, "action": true, "reason": true},
	}
	accepts, ok := allowed[command]
	if !ok {
		return true, errors.New("unknown memory-review command")
	}
	invalid := false
	flags.Visit(func(f *flag.Flag) {
		if !accepts[f.Name] {
			invalid = true
		}
	})
	if invalid {
		return true, errors.New("flag is not allowed for this review command")
	}
	if (command == "inspect" || command == "operation" || command == "alternatives" || command == "choose" || command == "identity-revision") && *id == "" || command == "prepare" && *id == "" || command == "resolve" && (*preview == "" || *digest == "" || *delivery == "") {
		return true, errors.New("review command is missing an exact identity")
	}
	if (command == "prepare" || command == "resolve") && *action != "accept" && *action != "reject" {
		return true, errors.New("review requires explicit --action accept or reject")
	}
	authority, err := kernel.LocalOwnerReviewContext(ctx, *scope)
	if err != nil {
		return true, err
	}
	var result any
	switch command {
	case "alternatives", "choose", "identity-revision":
		identityKernel, ok := kernel.(ownerIdentityReviewKernel)
		if !ok {
			return true, errors.New("identity review unavailable")
		}
		ref := memory.CandidateRef{ID: *id, ReviewRevision: *revision, InterpretationRevision: *interpretation}
		if command == "identity-revision" {
			result, err = identityKernel.InspectOwnerCandidateIdentityRevision(ctx, authority, *id, *interpretation)
		} else if command == "alternatives" {
			result, err = identityKernel.OwnerCandidateIdentityOptions(ctx, authority, ref)
		} else {
			var selected memory.ReviewIdentityChoices
			if *options == "" {
				return true, errors.New("choose requires reviewed --options digest")
			}
			if err := memory.DecodeCompilerJSON([]byte(*choices), &selected); err != nil {
				return true, err
			}
			result, err = identityKernel.ChooseOwnerCandidateIdentity(ctx, authority, memory.ReviewIdentityDecision{Candidate: ref, OptionsSHA256: *options, Choices: selected})
		}
	case "inbox":
		result, err = kernel.ListOwnerCandidates(ctx, authority, memory.OwnerCandidateQuery{Limit: *limit, Cursor: *cursor})
	case "inspect":
		result, err = kernel.InspectOwnerCandidate(ctx, authority, *id)
	case "operation":
		result, err = kernel.InspectOwnerReviewOperation(ctx, authority, memory.SemanticID(*id))
	case "prepare":
		result, err = kernel.PrepareOwnerCandidateReview(ctx, authority, memory.CandidateRef{ID: *id, ReviewRevision: *revision, InterpretationRevision: *interpretation}, *action)
	case "resolve":
		result, err = kernel.ResolveOwnerCandidateReview(ctx, authority, memory.ReviewDecision{DeliveryKey: *delivery, PreviewID: *preview, PreviewSHA256: *digest, Action: *action, Reason: *reason})
	}
	if err != nil && !errors.Is(err, eviedb.ErrReviewResolved) {
		return true, err
	}
	resolutionErr := err
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return true, errors.Join(encoder.Encode(result), resolutionErr)
}
