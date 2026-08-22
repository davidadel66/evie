package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type fakeREPLSessionStore struct {
	project               memory.Project
	findErr               error
	registeredProject     memory.Project
	projectAfterRegister  memory.Project
	registerErr           error
	registrationAttempted bool
	findRoots             []string
	registerNames         []string
	registerRoots         []string
	projectSessionIDs     []memory.ProjectID
	globalSessions        int
}

func (f *fakeREPLSessionStore) FindProjectByRoot(_ context.Context, root string) (memory.Project, error) {
	f.findRoots = append(f.findRoots, root)
	if f.registrationAttempted && f.projectAfterRegister.ID != "" {
		return f.projectAfterRegister, nil
	}
	if f.findErr != nil {
		return memory.Project{}, f.findErr
	}
	return f.project, nil
}

func (f *fakeREPLSessionStore) RegisterProject(_ context.Context, displayName, root string) (memory.Project, error) {
	f.registrationAttempted = true
	f.registerNames = append(f.registerNames, displayName)
	f.registerRoots = append(f.registerRoots, root)
	if f.registerErr != nil {
		return memory.Project{}, f.registerErr
	}
	f.registeredProject = memory.Project{
		ID:            "registered-project",
		DisplayName:   displayName,
		CanonicalRoot: root,
	}
	return f.registeredProject, nil
}

func (f *fakeREPLSessionStore) CreateProjectSession(_ context.Context, projectID memory.ProjectID) (memory.Session, error) {
	f.projectSessionIDs = append(f.projectSessionIDs, projectID)
	root := f.project.CanonicalRoot
	if projectID == f.registeredProject.ID {
		root = f.registeredProject.CanonicalRoot
	}
	return memory.Session{
		ID:                  "project-session",
		ProjectID:           projectID,
		ProjectRootSnapshot: root,
		Status:              memory.SessionActive,
	}, nil
}

func (f *fakeREPLSessionStore) CreateGlobalSession(context.Context) (memory.Session, error) {
	f.globalSessions++
	return memory.Session{ID: "global-session", Status: memory.SessionActive}, nil
}

func TestSelectREPLSession(t *testing.T) {
	tests := []struct {
		name             string
		matched          bool
		archived         bool
		input            string
		wantProjectID    memory.ProjectID
		wantGlobal       bool
		wantRegisterName string
		wantOutput       []string
	}{
		{
			name:          "matching cwd confirms suggested project",
			matched:       true,
			input:         "project\n",
			wantProjectID: "matched-project",
			wantOutput:    []string{"matches active project \"Evie\"", "[p]roject or [g]lobal"},
		},
		{
			name:       "matching cwd explicitly chooses global",
			matched:    true,
			input:      "\ng\n",
			wantGlobal: true,
			wantOutput: []string{"Please enter p or g."},
		},
		{
			name:       "archived cwd explicitly chooses global",
			matched:    true,
			archived:   true,
			input:      "g\n",
			wantGlobal: true,
			wantOutput: []string{"belongs to archived project \"Evie\"", "[g]lobal scope"},
		},
		{
			name:             "unmatched cwd explicitly registers project",
			input:            "register\nWorkspace\n",
			wantProjectID:    "registered-project",
			wantRegisterName: "Workspace",
			wantOutput:       []string{"No active project is registered", "[r]egister this directory or use [g]lobal", "Project name"},
		},
		{
			name:       "unmatched cwd explicitly chooses global",
			input:      "global\n",
			wantGlobal: true,
			wantOutput: []string{"No active project is registered"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			launchDir := t.TempDir()
			canonicalRoot, err := memory.CanonicalProjectRoot(launchDir)
			if err != nil {
				t.Fatalf("canonicalize launch directory: %v", err)
			}

			store := &fakeREPLSessionStore{
				project: memory.Project{
					ID:            "matched-project",
					DisplayName:   "Evie",
					CanonicalRoot: canonicalRoot,
				},
			}
			store.project.Archived = tt.archived
			if !tt.matched {
				store.findErr = fmt.Errorf("lookup: %w", eviedb.ErrProjectNotFound)
			}

			var output bytes.Buffer
			session, err := selectREPLSession(
				context.Background(),
				store,
				launchDir,
				bufio.NewScanner(strings.NewReader(tt.input)),
				&output,
			)
			if err != nil {
				t.Fatalf("select REPL session: %v", err)
			}

			if len(store.findRoots) != 1 || store.findRoots[0] != canonicalRoot {
				t.Errorf("lookup roots = %q, want canonical launch root %q", store.findRoots, canonicalRoot)
			}
			if tt.wantGlobal {
				if session.ProjectID != "" || store.globalSessions != 1 || len(store.projectSessionIDs) != 0 {
					t.Errorf("global selection created session %+v; global calls=%d project calls=%v",
						session, store.globalSessions, store.projectSessionIDs)
				}
			} else if session.ProjectID != tt.wantProjectID || store.globalSessions != 0 ||
				len(store.projectSessionIDs) != 1 || store.projectSessionIDs[0] != tt.wantProjectID {
				t.Errorf("project selection created session %+v; global calls=%d project calls=%v",
					session, store.globalSessions, store.projectSessionIDs)
			}

			if tt.wantRegisterName == "" {
				if len(store.registerNames) != 0 {
					t.Errorf("unexpected registrations: names=%v roots=%v", store.registerNames, store.registerRoots)
				}
			} else if len(store.registerNames) != 1 || store.registerNames[0] != tt.wantRegisterName ||
				len(store.registerRoots) != 1 || store.registerRoots[0] != canonicalRoot {
				t.Errorf("registrations: names=%v roots=%v, want %q at %q",
					store.registerNames, store.registerRoots, tt.wantRegisterName, canonicalRoot)
			}

			for _, want := range tt.wantOutput {
				if !strings.Contains(output.String(), want) {
					t.Errorf("output %q does not contain %q", output.String(), want)
				}
			}
		})
	}
}

func TestSelectREPLSessionReconfirmsAConcurrentRegistration(t *testing.T) {
	root := t.TempDir()
	canonicalRoot, err := memory.CanonicalProjectRoot(root)
	if err != nil {
		t.Fatalf("canonicalize root: %v", err)
	}
	store := &fakeREPLSessionStore{
		findErr:     eviedb.ErrProjectNotFound,
		registerErr: errors.New("unique root collision"),
		projectAfterRegister: memory.Project{
			ID:            "concurrent-project",
			DisplayName:   "Concurrent",
			CanonicalRoot: canonicalRoot,
		},
	}

	var output bytes.Buffer
	session, err := selectREPLSession(
		context.Background(),
		store,
		root,
		bufio.NewScanner(strings.NewReader("r\nMine\np\n")),
		&output,
	)
	if err != nil {
		t.Fatalf("select REPL session: %v", err)
	}
	if session.ProjectID != "concurrent-project" {
		t.Errorf("selected project = %q, want concurrent project", session.ProjectID)
	}
	if len(store.findRoots) != 2 {
		t.Errorf("project lookups = %d, want initial discovery plus collision retry", len(store.findRoots))
	}
	if !strings.Contains(output.String(), "matches active project \"Concurrent\"") {
		t.Errorf("output %q does not reconfirm the concurrently registered project", output.String())
	}
}

func TestSelectREPLSessionHandlesArchivedRootWithRealStore(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store := eviedb.NewStore(db)
	root := t.TempDir()

	project, err := store.RegisterProject(context.Background(), "Archived", root)
	if err != nil {
		t.Fatalf("register project: %v", err)
	}
	if _, err := db.Exec(`UPDATE projects SET archived = 1 WHERE id = ?`, project.ID); err != nil {
		t.Fatalf("archive project: %v", err)
	}

	var output bytes.Buffer
	session, err := selectREPLSession(
		context.Background(),
		store,
		root,
		bufio.NewScanner(strings.NewReader("g\n")),
		&output,
	)
	if err != nil {
		t.Fatalf("select REPL session: %v", err)
	}
	if session.ProjectID != "" {
		t.Errorf("session scope = %+v, want global", session.ScopeContext())
	}
	if !strings.Contains(output.String(), "belongs to archived project \"Archived\"") {
		t.Errorf("output %q does not explain the archived root", output.String())
	}

	var projects int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects WHERE canonical_root = ?`, project.CanonicalRoot).Scan(&projects); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if projects != 1 {
		t.Errorf("projects at archived root = %d, want no duplicate registration", projects)
	}
}

func TestSelectREPLSessionRequiresAnExplicitChoice(t *testing.T) {
	for _, matched := range []bool{true, false} {
		t.Run(fmt.Sprintf("matched=%t", matched), func(t *testing.T) {
			root := t.TempDir()
			store := &fakeREPLSessionStore{
				project: memory.Project{
					ID:            "matched-project",
					DisplayName:   "Evie",
					CanonicalRoot: root,
				},
			}
			if !matched {
				store.findErr = eviedb.ErrProjectNotFound
			}

			_, err := selectREPLSession(
				context.Background(),
				store,
				root,
				bufio.NewScanner(strings.NewReader("\n")),
				io.Discard,
			)
			if !errors.Is(err, io.EOF) {
				t.Fatalf("selection error = %v, want io.EOF", err)
			}
			if store.globalSessions != 0 || len(store.projectSessionIDs) != 0 || len(store.registerNames) != 0 {
				t.Fatalf("blank input created state: global=%d project=%v registrations=%v",
					store.globalSessions, store.projectSessionIDs, store.registerNames)
			}
		})
	}
}

func TestSelectREPLSessionUsesDefaultProjectName(t *testing.T) {
	root := t.TempDir()
	store := &fakeREPLSessionStore{findErr: eviedb.ErrProjectNotFound}

	_, err := selectREPLSession(
		context.Background(),
		store,
		root,
		bufio.NewScanner(strings.NewReader("r\n\n")),
		io.Discard,
	)
	if err != nil {
		t.Fatalf("select REPL session: %v", err)
	}
	if len(store.registerNames) != 1 || store.registerNames[0] != filepath.Base(root) {
		t.Errorf("registered names = %v, want default %q", store.registerNames, filepath.Base(root))
	}
}
