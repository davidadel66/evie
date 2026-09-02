package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/web"
)

type webContextCompositionManager interface {
	sessionCompositionResolver
	ResolvePresetContext(context.Context, plugins.PresetID) (plugins.ResolvedComposition, error)
}

type webContextSessionController struct {
	store    *eviedb.Store
	manager  webContextCompositionManager
	newAgent func(memory.Session, plugins.ResolvedComposition) (*agent.Session, error)
}

func newWebContextSessionController(
	store *eviedb.Store,
	manager webContextCompositionManager,
	newAgent func(memory.Session, plugins.ResolvedComposition) (*agent.Session, error),
) *webContextSessionController {
	return &webContextSessionController{store: store, manager: manager, newAgent: newAgent}
}

func (c *webContextSessionController) Snapshot(ctx context.Context) (web.ContextSessionSnapshot, error) {
	workspaces, err := c.store.ListWorkspaces(ctx, true)
	if err != nil {
		return web.ContextSessionSnapshot{}, err
	}
	projects, err := c.store.ListProjects(ctx, true)
	if err != nil {
		return web.ContextSessionSnapshot{}, err
	}
	sessions, err := c.store.ListActiveSessions(ctx)
	if err != nil {
		return web.ContextSessionSnapshot{}, err
	}
	return web.ContextSessionSnapshot{Workspaces: workspaces, Projects: projects, Sessions: sessions}, nil
}

func (c *webContextSessionController) RegisterWorkspace(ctx context.Context, displayName string) (memory.Workspace, error) {
	return c.store.RegisterWorkspace(ctx, displayName)
}

func (c *webContextSessionController) SelectSession(
	ctx context.Context,
	selection web.ContextSessionSelection,
) (web.OpenedContextSession, error) {
	var (
		session  memory.Session
		standard plugins.ResolvedComposition
		created  *plugins.ResolvedComposition
		err      error
	)
	resolveStandard := func() error {
		standard, err = c.manager.ResolvePresetContext(ctx, plugins.StandardPresetID)
		if err != nil {
			return fmt.Errorf("resolve standard Agent Preset: %w", err)
		}
		return nil
	}
	switch {
	case selection.SessionID != "":
		session, err = c.store.GetActiveSession(ctx, selection.SessionID)
		if err == nil {
			_, receiptErr := c.store.GetCompositionReceipt(ctx, session.ID)
			if errors.Is(receiptErr, eviedb.ErrCompositionReceiptNotFound) {
				err = resolveStandard()
			} else if receiptErr != nil {
				err = receiptErr
			}
		}
	case selection.WorkspaceID != "":
		if err = resolveStandard(); err == nil {
			session, err = c.store.CreateWorkspaceSessionForChooserWithComposition(
				ctx, selection.WorkspaceID, selection.WorkspaceRevision, standard.Receipt,
			)
			created = &standard
		}
	case selection.ProjectID != "":
		if err = resolveStandard(); err == nil {
			session, err = c.store.CreateProjectSessionWithComposition(ctx, selection.ProjectID, standard.Receipt)
			created = &standard
		}
	case selection.Unscoped:
		if err = resolveStandard(); err == nil {
			session, err = c.store.CreateGlobalSessionWithComposition(ctx, standard.Receipt)
			created = &standard
		}
	default:
		return web.OpenedContextSession{}, fmt.Errorf("invalid Context Scope selection")
	}
	if err != nil {
		return web.OpenedContextSession{}, err
	}
	bound, err := bindSessionComposition(ctx, c.store, c.manager, session.ID, standard, created)
	if err != nil {
		return web.OpenedContextSession{}, fmt.Errorf("compose selected session: %w", err)
	}
	openedAgent, err := c.newAgent(session, bound.Resolved)
	if err != nil {
		return web.OpenedContextSession{}, err
	}
	return web.OpenedContextSession{Session: session, Agent: openedAgent}, nil
}
