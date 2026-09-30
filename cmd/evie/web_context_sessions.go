package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/web"
)

type webContextCompositionManager interface {
	sessionCompositionResolver
	ResolvePresetContext(context.Context, plugins.PresetID) (plugins.ResolvedComposition, error)
}

type webContextSessionController struct {
	modelClient interface {
		ListChatModels(context.Context) ([]openrouter.Model, error)
	}
	defaultModel  string
	newModelAgent func(context.Context, memory.Session, plugins.ResolvedComposition, string, int64) (*agent.Session, error)
	store         *eviedb.Store
	manager       webContextCompositionManager
	newAgent      func(memory.Session, plugins.ResolvedComposition) (*agent.Session, error)
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
	archived, err := c.store.ListArchivedSessions(ctx)
	if err != nil {
		return web.ContextSessionSnapshot{}, err
	}
	return web.ContextSessionSnapshot{Workspaces: workspaces, Projects: projects, Sessions: sessions, ArchivedSessions: archived}, nil
}

func (c *webContextSessionController) ArchiveSession(ctx context.Context, id memory.SessionID) (memory.Session, error) {
	return c.store.ArchiveSession(ctx, id)
}

func (c *webContextSessionController) RestoreSession(ctx context.Context, id memory.SessionID) (memory.Session, error) {
	return c.store.RestoreSession(ctx, id)
}

func (c *webContextSessionController) RegisterWorkspace(ctx context.Context, displayName string) (memory.Workspace, error) {
	return c.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: displayName})
}

func (c *webContextSessionController) WorkspaceFolder(ctx context.Context, id memory.WorkspaceID) (memory.WorkspaceFolder, error) {
	return c.store.WorkspaceFolder(ctx, id)
}

func (c *webContextSessionController) SetWorkspaceFolder(ctx context.Context, id memory.WorkspaceID, revision int64, path string) (memory.WorkspaceFolder, error) {
	return c.store.SetWorkspaceFolder(ctx, id, revision, path)
}

func (c *webContextSessionController) SelectSession(
	ctx context.Context,
	selection web.ContextSessionSelection,
) (web.OpenedContextSession, error) {
	return c.selectSession(ctx, selection, "")
}

func (c *webContextSessionController) selectSession(ctx context.Context, selection web.ContextSessionSelection, modelOverride string) (web.OpenedContextSession, error) {
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
		var presetID string
		presetID, err = c.store.WorkspaceDefaultPreset(ctx, selection.WorkspaceID, selection.WorkspaceRevision)
		if err == nil {
			standard, err = c.manager.ResolvePresetContext(ctx, plugins.PresetID(presetID))
		}
		if err == nil {
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
	setting, err := c.store.SessionModel(ctx, session.ID)
	if err != nil {
		return web.OpenedContextSession{}, err
	}
	var openedAgent *agent.Session
	if c.newModelAgent != nil {
		model := setting.Model
		if model == "" {
			model = c.defaultModel
		}
		if modelOverride != "" {
			model = modelOverride
		}
		revision := setting.Revision
		if modelOverride != "" {
			revision++
		}
		openedAgent, err = c.newModelAgent(ctx, session, bound.Resolved, model, revision)
	} else {
		openedAgent, err = c.newAgent(session, bound.Resolved)
	}
	if err != nil {
		return web.OpenedContextSession{}, err
	}
	return web.OpenedContextSession{Session: session, Agent: openedAgent, ModelRevision: setting.Revision}, nil
}

func (c *webContextSessionController) PreviewRepositoryInstructions(ctx context.Context, id memory.WorkspaceID) (memory.RepositoryInstructionSnapshot, error) {
	return c.store.PreviewRepositoryInstructions(ctx, id)
}
func (c *webContextSessionController) SetRepositoryInstructionSettings(ctx context.Context, id memory.WorkspaceID, revision int64, enabled bool) (memory.RepositoryInstructionSettings, error) {
	return c.store.SetRepositoryInstructionSettings(ctx, id, revision, enabled)
}
func (c *webContextSessionController) RepositoryInstructionSnapshot(ctx context.Context, session memory.SessionID, turn memory.EventID) (memory.RepositoryInstructionSnapshot, error) {
	return c.store.RepositoryInstructionSnapshot(ctx, session, turn)
}

func (c *webContextSessionController) ListChatModels(ctx context.Context) ([]openrouter.Model, error) {
	if c.modelClient == nil {
		return nil, errors.New("model catalog unavailable")
	}
	return c.modelClient.ListChatModels(ctx)
}

func (c *webContextSessionController) SelectModel(ctx context.Context, id memory.SessionID, revision int64, model string) (web.OpenedContextSession, error) {
	models, err := c.ListChatModels(ctx)
	if err != nil {
		return web.OpenedContextSession{}, err
	}
	found := false
	for _, candidate := range models {
		if candidate.ID == model {
			found = true
			break
		}
	}
	if !found || c.newModelAgent == nil {
		return web.OpenedContextSession{}, errors.New("model is not available")
	}
	opened, err := c.selectSession(ctx, web.ContextSessionSelection{SessionID: id}, model)
	if err != nil {
		return web.OpenedContextSession{}, err
	}
	setting, err := c.store.SetSessionModel(ctx, id, revision, model)
	if err != nil {
		return web.OpenedContextSession{}, err
	}
	opened.ModelRevision = setting.Revision
	return opened, nil
}
