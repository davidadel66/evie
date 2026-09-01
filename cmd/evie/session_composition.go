package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
)

type sessionCompositionStore interface {
	SaveCompositionReceipt(context.Context, memory.SessionID, composition.Receipt) error
	GetCompositionReceipt(context.Context, memory.SessionID) (composition.Receipt, error)
}

type sessionCompositionResolver interface {
	ResumeComposition(composition.Receipt) (plugins.ResolvedComposition, error)
}

type visibleCompositionWarning struct {
	Code    composition.WarningCode
	Message string
}

type boundSessionComposition struct {
	Resolved plugins.ResolvedComposition
	Warnings []visibleCompositionWarning
}

type storedSessionSelection struct {
	memory.Session
	createdComposition *plugins.ResolvedComposition
}

type receiptBoundREPLStore struct {
	*eviedb.Store
	composition      plugins.ResolvedComposition
	createdSessionID memory.SessionID
}

func (s *receiptBoundREPLStore) CreateProjectSessionForChooser(
	ctx context.Context,
	projectID memory.ProjectID,
	expectedProjectRoot string,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
) (memory.Session, error) {
	session, err := s.Store.CreateProjectSessionForChooserWithComposition(
		ctx, projectID, expectedProjectRoot, cwdRoot, expectedCWDProjectID, s.composition.Receipt,
	)
	if err == nil {
		s.createdSessionID = session.ID
	}
	return session, err
}

func (s *receiptBoundREPLStore) CreateGlobalSessionForChooser(
	ctx context.Context,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
) (memory.Session, error) {
	session, err := s.Store.CreateGlobalSessionForChooserWithComposition(
		ctx, cwdRoot, expectedCWDProjectID, s.composition.Receipt,
	)
	if err == nil {
		s.createdSessionID = session.ID
	}
	return session, err
}

func (s *receiptBoundREPLStore) selection(session memory.Session) storedSessionSelection {
	selection := storedSessionSelection{Session: session}
	if session.ID != "" && session.ID == s.createdSessionID {
		created := s.composition
		selection.createdComposition = &created
	}
	return selection
}

// bindSessionComposition hands a newly created session its original resolved
// snapshot when the atomic durable receipt matches. Existing pinned sessions
// reconstruct from their receipt; legacy sessions append the default once. A
// losing candidate always reconstructs the durable winner.
func bindSessionComposition(
	ctx context.Context,
	store sessionCompositionStore,
	resolver sessionCompositionResolver,
	sessionID memory.SessionID,
	defaultComposition plugins.ResolvedComposition,
	createdComposition *plugins.ResolvedComposition,
) (boundSessionComposition, error) {
	receipt, err := store.GetCompositionReceipt(ctx, sessionID)
	if err == nil {
		if createdComposition != nil && reflect.DeepEqual(receipt, createdComposition.Receipt) {
			return newBoundSessionComposition(*createdComposition), nil
		}
		resolved, resumeErr := resolver.ResumeComposition(receipt)
		if resumeErr != nil {
			return boundSessionComposition{}, fmt.Errorf("resume pinned session composition: %w", resumeErr)
		}
		return newBoundSessionComposition(resolved), nil
	}
	if !errors.Is(err, eviedb.ErrCompositionReceiptNotFound) {
		return boundSessionComposition{}, err
	}
	if err := store.SaveCompositionReceipt(ctx, sessionID, defaultComposition.Receipt); err == nil {
		return newBoundSessionComposition(defaultComposition), nil
	} else if !errors.Is(err, eviedb.ErrCompositionReceiptExists) {
		return boundSessionComposition{}, err
	}
	receipt, err = store.GetCompositionReceipt(ctx, sessionID)
	if err != nil {
		return boundSessionComposition{}, err
	}
	resolved, err := resolver.ResumeComposition(receipt)
	if err != nil {
		return boundSessionComposition{}, fmt.Errorf("resume concurrently pinned session composition: %w", err)
	}
	return newBoundSessionComposition(resolved), nil
}

func newBoundSessionComposition(resolved plugins.ResolvedComposition) boundSessionComposition {
	warnings := make([]visibleCompositionWarning, len(resolved.Receipt.Warnings))
	for i, warning := range resolved.Receipt.Warnings {
		warnings[i] = visibleCompositionWarning{
			Code: warning.Code,
			Message: fmt.Sprintf(
				"optional Capability %q from provider %q is unavailable",
				warning.CapabilityID, warning.ProviderID,
			),
		}
	}
	return boundSessionComposition{Resolved: resolved, Warnings: warnings}
}
