package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type fakeREPLSessionStore struct {
	projects                 []memory.Project
	workspaces               []memory.Workspace
	sessions                 []memory.SessionListing
	findProject              memory.Project
	findErr                  error
	registerErr              error
	registerCalls            int
	registeredNames          []string
	createdProject           []memory.ProjectID
	createdGlobal            int
	createdWorkspace         []memory.WorkspaceID
	registeredWorkspaceNames []string
	activeGets               []memory.SessionID
	createHook               func(memory.ProjectID) (memory.Session, error)
	getHook                  func(memory.SessionID) (memory.Session, error)
	listHook                 func() ([]memory.Project, []memory.SessionListing)
}

type withoutWorkspaceREPLStore struct {
	*eviedb.Store
}

func (s *withoutWorkspaceREPLStore) CreateWorkspaceSessionForChooser(
	context.Context, memory.WorkspaceID, memory.WorkspaceRevisionID,
) (memory.Session, error) {
	return memory.Session{}, errors.New("unexpected Workspace session selection")
}

func (f *fakeREPLSessionStore) ListWorkspaces(context.Context, bool) ([]memory.Workspace, error) {
	return append([]memory.Workspace(nil), f.workspaces...), nil
}

func (f *fakeREPLSessionStore) RegisterWorkspace(_ context.Context, displayName string) (memory.Workspace, error) {
	f.registeredWorkspaceNames = append(f.registeredWorkspaceNames, displayName)
	workspace := memory.Workspace{ID: "workspace-registered", DisplayName: displayName, State: memory.WorkspaceActive, CurrentRevisionID: "revision-1"}
	f.workspaces = append(f.workspaces, workspace)
	return workspace, nil
}

func (f *fakeREPLSessionStore) CreateWorkspaceSessionForChooser(
	_ context.Context, id memory.WorkspaceID, revision memory.WorkspaceRevisionID,
) (memory.Session, error) {
	f.createdWorkspace = append(f.createdWorkspace, id)
	return memory.Session{ID: "new-workspace", WorkspaceID: id, WorkspaceRevisionSnapshot: revision, Status: memory.SessionActive}, nil
}

type relocatingREPLStore struct {
	*eviedb.Store
	db               *sql.DB
	newRoot          string
	relocateNext     bool
	relocatedRoot    string
	sessionsAtReject int
}

func (s *relocatingREPLStore) CreateWorkspaceSessionForChooser(
	context.Context, memory.WorkspaceID, memory.WorkspaceRevisionID,
) (memory.Session, error) {
	return memory.Session{}, errors.New("unexpected Workspace session selection")
}

func (s *relocatingREPLStore) CreateProjectSessionForChooser(
	ctx context.Context,
	projectID memory.ProjectID,
	expectedProjectRoot string,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
) (memory.Session, error) {
	if s.relocateNext {
		s.relocateNext = false
		relocated, err := s.RelocateProject(ctx, projectID, s.newRoot)
		if err != nil {
			return memory.Session{}, err
		}
		s.relocatedRoot = relocated.CanonicalRoot
	}
	session, err := s.Store.CreateProjectSessionForChooser(
		ctx, projectID, expectedProjectRoot, cwdRoot, expectedCWDProjectID,
	)
	if errors.Is(err, eviedb.ErrChooserStateChanged) {
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&s.sessionsAtReject)
	}
	return session, err
}

type concurrentCWDREPLStore struct {
	*eviedb.Store
	db               *sql.DB
	cwd              string
	triggerNext      bool
	sessionsAtReject int
}

func (s *concurrentCWDREPLStore) CreateWorkspaceSessionForChooser(
	context.Context, memory.WorkspaceID, memory.WorkspaceRevisionID,
) (memory.Session, error) {
	return memory.Session{}, errors.New("unexpected Workspace session selection")
}

type deletingProjectREPLStore struct {
	*eviedb.Store
	db               *sql.DB
	deleteNext       bool
	sessionsAtReject int
}

func (s *deletingProjectREPLStore) CreateWorkspaceSessionForChooser(
	context.Context, memory.WorkspaceID, memory.WorkspaceRevisionID,
) (memory.Session, error) {
	return memory.Session{}, errors.New("unexpected Workspace session selection")
}

func (s *deletingProjectREPLStore) CreateProjectSessionForChooser(
	ctx context.Context,
	projectID memory.ProjectID,
	expectedProjectRoot string,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
) (memory.Session, error) {
	if s.deleteNext {
		s.deleteNext = false
		if _, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, projectID); err != nil {
			return memory.Session{}, err
		}
	}
	session, err := s.Store.CreateProjectSessionForChooser(
		ctx, projectID, expectedProjectRoot, cwdRoot, expectedCWDProjectID,
	)
	if errors.Is(err, eviedb.ErrChooserStateChanged) {
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&s.sessionsAtReject)
	}
	return session, err
}

func (s *concurrentCWDREPLStore) registerBeforeAction(ctx context.Context) error {
	if !s.triggerNext {
		return nil
	}
	s.triggerNext = false
	_, err := s.RegisterProject(ctx, "Current", s.cwd)
	return err
}

func (s *concurrentCWDREPLStore) recordRejection(err error) {
	if errors.Is(err, eviedb.ErrChooserStateChanged) {
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&s.sessionsAtReject)
	}
}

func (s *concurrentCWDREPLStore) CreateProjectSessionForChooser(
	ctx context.Context,
	projectID memory.ProjectID,
	expectedProjectRoot string,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
) (memory.Session, error) {
	if err := s.registerBeforeAction(ctx); err != nil {
		return memory.Session{}, err
	}
	session, err := s.Store.CreateProjectSessionForChooser(
		ctx, projectID, expectedProjectRoot, cwdRoot, expectedCWDProjectID,
	)
	s.recordRejection(err)
	return session, err
}

func (s *concurrentCWDREPLStore) CreateGlobalSessionForChooser(
	ctx context.Context,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
) (memory.Session, error) {
	if err := s.registerBeforeAction(ctx); err != nil {
		return memory.Session{}, err
	}
	session, err := s.Store.CreateGlobalSessionForChooser(ctx, cwdRoot, expectedCWDProjectID)
	s.recordRejection(err)
	return session, err
}

func (s *concurrentCWDREPLStore) GetActiveSessionForChooser(
	ctx context.Context,
	sessionID memory.SessionID,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
) (memory.Session, error) {
	if err := s.registerBeforeAction(ctx); err != nil {
		return memory.Session{}, err
	}
	session, err := s.Store.GetActiveSessionForChooser(ctx, sessionID, cwdRoot, expectedCWDProjectID)
	s.recordRejection(err)
	return session, err
}

func (f *fakeREPLSessionStore) ListProjects(context.Context, bool) ([]memory.Project, error) {
	if f.listHook != nil {
		projects, _ := f.listHook()
		return projects, nil
	}
	return append([]memory.Project(nil), f.projects...), nil
}

func (f *fakeREPLSessionStore) ListActiveSessions(context.Context) ([]memory.SessionListing, error) {
	if f.listHook != nil {
		_, sessions := f.listHook()
		return sessions, nil
	}
	return append([]memory.SessionListing(nil), f.sessions...), nil
}

func (f *fakeREPLSessionStore) FindProjectByRoot(context.Context, string) (memory.Project, error) {
	return f.findProject, f.findErr
}

func (f *fakeREPLSessionStore) RegisterProject(_ context.Context, displayName, root string) (memory.Project, error) {
	f.registerCalls++
	f.registeredNames = append(f.registeredNames, displayName)
	if f.registerErr != nil {
		return memory.Project{}, f.registerErr
	}
	return memory.Project{ID: "registered", DisplayName: displayName, CanonicalRoot: root}, nil
}

func (f *fakeREPLSessionStore) CreateProjectSessionForChooser(
	_ context.Context,
	id memory.ProjectID,
	_ string,
	_ string,
	_ memory.ProjectID,
) (memory.Session, error) {
	f.createdProject = append(f.createdProject, id)
	if f.createHook != nil {
		return f.createHook(id)
	}
	return memory.Session{ID: "new-project", ProjectID: id, Status: memory.SessionActive}, nil
}

func (f *fakeREPLSessionStore) CreateGlobalSessionForChooser(context.Context, string, memory.ProjectID) (memory.Session, error) {
	f.createdGlobal++
	return memory.Session{ID: "new-global", Status: memory.SessionActive}, nil
}

func (f *fakeREPLSessionStore) GetActiveSessionForChooser(
	_ context.Context,
	id memory.SessionID,
	_ string,
	_ memory.ProjectID,
) (memory.Session, error) {
	f.activeGets = append(f.activeGets, id)
	if f.getHook != nil {
		return f.getHook(id)
	}
	for _, listing := range f.sessions {
		if listing.ID == id {
			return listing.Session, nil
		}
	}
	return memory.Session{}, eviedb.ErrSessionNotActive
}

func TestRenderREPLChooserCombinedHierarchyAndSafeLabels(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	projects := []memory.Project{
		{ID: "alpha", DisplayName: "Alpha", CanonicalRoot: "/alpha"},
		{ID: "fallback", DisplayName: "\x1b\u200b", CanonicalRoot: "/fallback", CreatedAt: now},
		{ID: "z", DisplayName: "Same\x1b[31m", CanonicalRoot: "/z\nroot"},
		{ID: "a", DisplayName: "Same\u200b", CanonicalRoot: "/a\troot"},
		{ID: "empty", DisplayName: "Empty", CanonicalRoot: "/empty"},
		{ID: "archived", DisplayName: "Old", CanonicalRoot: "/old", Archived: true},
		{ID: "archived-empty", DisplayName: "Hidden", CanonicalRoot: "/hidden", Archived: true},
	}
	sessions := []memory.SessionListing{
		{Session: memory.Session{ID: "a-new", ProjectID: "a", ProjectRootSnapshot: "/a\troot", Title: "new\n\x1btitle", CreatedAt: now}, ActivityAt: now.Add(time.Minute)},
		{Session: memory.Session{ID: "a-old", ProjectID: "a", ProjectRootSnapshot: "/moved\x1b", Title: "older", CreatedAt: now}, ActivityAt: now},
		{Session: memory.Session{ID: "archived-session", ProjectID: "archived", ProjectRootSnapshot: "/old", CreatedAt: now}, ActivityAt: now},
		{Session: memory.Session{ID: "global", Title: "global", CreatedAt: now}, ActivityAt: now},
	}

	var out bytes.Buffer
	actions, err := renderREPLChooser(&out, "/a\troot", projects, sessions)
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"Empty — \"/empty\"",
		"Untitled project — 2026-08-24T12:00:00Z — \"/fallback\"",
		"Same — \"/a\\troot\" (current directory)",
		"new title",
		"stored root: \"/moved\\x1b\"",
		"Old — \"/old\" (archived)",
		"Untitled — 2026-08-24T12:00:00Z",
		"Global",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("chooser output %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "Hidden") || strings.ContainsRune(got, '\x1b') || strings.Contains(got, "\u200b") {
		t.Fatalf("chooser leaked hidden or terminal-active label: %q", got)
	}
	if strings.Index(got, "Empty") > strings.Index(got, "Old") || strings.Index(got, "Old") > strings.Index(got, "Global") {
		t.Fatalf("project/archive/global ordering is wrong: %q", got)
	}
	if strings.Index(got, "Alpha") > strings.Index(got, "Untitled project") {
		t.Fatalf("project fallback was not used as the sort label: %q", got)
	}
	if gotNew := strings.Index(got, "new title"); gotNew > strings.Index(got, "older") {
		t.Fatalf("session activity ordering is wrong: %q", got)
	}
	if len(actions) != 10 {
		t.Fatalf("actions = %d, want 10", len(actions))
	}
}

func TestRenderREPLChooserOrdersTimestampedProjectFallbackLabels(t *testing.T) {
	earlier := time.Date(2026, 8, 24, 12, 0, 0, 1, time.FixedZone("east", 2*60*60))
	later := earlier.Add(time.Nanosecond)
	projects := []memory.Project{
		{ID: "later", DisplayName: "\x1b", CanonicalRoot: "/a", CreatedAt: later},
		{ID: "earlier", DisplayName: "\u200b", CanonicalRoot: "/z", CreatedAt: earlier},
	}
	var out bytes.Buffer
	if _, err := renderREPLChooser(&out, "/other", projects, nil); err != nil {
		t.Fatal(err)
	}
	earlierLabel := "Untitled project — 2026-08-24T10:00:00.000000001Z"
	laterLabel := "Untitled project — 2026-08-24T10:00:00.000000002Z"
	if strings.Index(out.String(), earlierLabel) >= strings.Index(out.String(), laterLabel) {
		t.Fatalf("timestamp fallback sort order is wrong: %q", out.String())
	}
}

func TestIdenticalSanitizedProjectNamesRemainRootDisambiguatedAndSelectable(t *testing.T) {
	rootA, rootB := t.TempDir(), t.TempDir()
	projects := []memory.Project{
		{ID: "plain", DisplayName: "Same", CanonicalRoot: rootB},
		{ID: "formatted", DisplayName: "Sa\u200bme", CanonicalRoot: rootA},
	}
	if replProjectLabel(projects[0]) != replProjectLabel(projects[1]) {
		t.Fatalf("fixture labels differ: %q and %q", replProjectLabel(projects[0]), replProjectLabel(projects[1]))
	}
	var rendered bytes.Buffer
	if _, err := renderREPLChooser(&rendered, t.TempDir(), projects, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Count(rendered.String(), "Same — ") != 2 ||
		!strings.Contains(rendered.String(), escapedREPLPath(rootA)) ||
		!strings.Contains(rendered.String(), escapedREPLPath(rootB)) {
		t.Fatalf("identical labels were not root-disambiguated: %q", rendered.String())
	}

	wantID := memory.ProjectID("plain")
	if rootA > rootB {
		wantID = "formatted"
	}
	store := &fakeREPLSessionStore{projects: projects}
	selected, err := selectREPLSession(
		context.Background(), store, t.TempDir(),
		bufio.NewScanner(strings.NewReader("2\n")), io.Discard,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ProjectID != wantID || len(store.createdProject) != 1 || store.createdProject[0] != wantID {
		t.Fatalf("selected=%+v created=%v, want project %q", selected, store.createdProject, wantID)
	}
}

func TestRenderREPLChooserPreservesStorageSessionOrderWhilePartitioning(t *testing.T) {
	project := memory.Project{ID: "project", DisplayName: "Project", CanonicalRoot: "/project"}
	sessions := []memory.SessionListing{
		{Session: memory.Session{ID: "project-first", ProjectID: project.ID, Title: "project first"}, ActivityAt: time.Unix(1, 0)},
		{Session: memory.Session{ID: "global-first", Title: "global first"}, ActivityAt: time.Unix(1, 0)},
		{Session: memory.Session{ID: "project-second", ProjectID: project.ID, Title: "project second"}, ActivityAt: time.Unix(2, 0)},
		{Session: memory.Session{ID: "global-second", Title: "global second"}, ActivityAt: time.Unix(2, 0)},
	}
	var out bytes.Buffer
	if _, err := renderREPLChooser(&out, "/other", []memory.Project{project}, sessions); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Index(got, "project first") > strings.Index(got, "project second") ||
		strings.Index(got, "global first") > strings.Index(got, "global second") {
		t.Fatalf("REPL re-sorted storage-owned session order: %q", got)
	}
}

func TestSelectREPLSessionExplicitNumberedPaths(t *testing.T) {
	root := t.TempDir()
	project := memory.Project{ID: "project", DisplayName: "Project", CanonicalRoot: root}
	projectSession := memory.Session{ID: "project-existing", ProjectID: project.ID, ProjectRootSnapshot: root, Status: memory.SessionActive, CreatedAt: time.Now()}
	globalSession := memory.Session{ID: "global-existing", Status: memory.SessionActive, CreatedAt: time.Now()}

	tests := []struct {
		name       string
		input      string
		wantID     memory.SessionID
		wantOutput string
	}{
		{name: "new project after invalid choice", input: "project\n1\n", wantID: "new-project", wantOutput: "Please enter a listed number."},
		{name: "resume project", input: "2\n", wantID: projectSession.ID},
		{name: "new global", input: "3\n", wantID: "new-global"},
		{name: "resume global", input: "4\n", wantID: globalSession.ID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeREPLSessionStore{
				projects: []memory.Project{project},
				sessions: []memory.SessionListing{{Session: projectSession}, {Session: globalSession}},
			}
			var out bytes.Buffer
			got, err := selectREPLSession(context.Background(), store, root, bufio.NewScanner(strings.NewReader(tt.input)), &out)
			if err != nil {
				t.Fatal(err)
			}
			if got.ID != tt.wantID {
				t.Fatalf("selected %q, want %q; output=%q", got.ID, tt.wantID, out.String())
			}
			if tt.wantOutput != "" && !strings.Contains(out.String(), tt.wantOutput) {
				t.Errorf("output %q missing %q", out.String(), tt.wantOutput)
			}
		})
	}
}

func TestSelectREPLSessionRegistersEntersAndResumesWorkspaceExplicitly(t *testing.T) {
	root := t.TempDir()
	store := &fakeREPLSessionStore{}
	var out bytes.Buffer
	created, err := selectREPLSession(
		context.Background(), store, root,
		bufio.NewScanner(strings.NewReader("3\nCairo's Kitchen\n")), &out,
	)
	if err != nil {
		t.Fatal(err)
	}
	if created.WorkspaceID != "workspace-registered" || created.WorkspaceRevisionSnapshot != "revision-1" ||
		len(store.createdWorkspace) != 1 || len(store.registeredWorkspaceNames) != 1 {
		t.Fatalf("created=%+v registered=%v entered=%v", created, store.registeredWorkspaceNames, store.createdWorkspace)
	}
	if !strings.Contains(out.String(), "Register Workspace") ||
		!strings.Contains(out.String(), "Context Scope: Workspace — Cairo's Kitchen") {
		t.Fatalf("Workspace selection was not explicit and visible: %q", out.String())
	}

	store.sessions = []memory.SessionListing{{Session: created}}
	out.Reset()
	resumed, err := selectREPLSession(
		context.Background(), store, root,
		bufio.NewScanner(strings.NewReader("4\n")), &out,
	)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ID != created.ID || resumed.WorkspaceID != created.WorkspaceID {
		t.Fatalf("resumed=%+v, want %+v", resumed, created)
	}
	if !strings.Contains(out.String(), "Cairo's Kitchen") ||
		!strings.Contains(out.String(), "Context Scope: Workspace — Cairo's Kitchen") {
		t.Fatalf("resumed Workspace scope not displayed: %q", out.String())
	}
}

func TestSelectREPLSessionRegistrationIsExplicit(t *testing.T) {
	root := t.TempDir()
	store := &fakeREPLSessionStore{findErr: eviedb.ErrProjectNotFound}
	var out bytes.Buffer
	session, err := selectREPLSession(context.Background(), store, root, bufio.NewScanner(strings.NewReader("2\n\n")), &out)
	if err != nil {
		t.Fatal(err)
	}
	if session.ProjectID != "registered" || store.registerCalls != 1 {
		t.Fatalf("registration selection = %+v calls=%d", session, store.registerCalls)
	}
	if !strings.Contains(out.String(), "Register current directory") || !strings.Contains(out.String(), filepath.Base(root)) {
		t.Fatalf("registration UI missing: %q", out.String())
	}
}

func TestSelectREPLSessionPersistsTimestampedFallbackForUnsafeCWDBasename(t *testing.T) {
	root := filepath.Join(t.TempDir(), "\u200b")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := eviedb.NewStore(db)
	chooserStore := &withoutWorkspaceREPLStore{Store: store}
	var out bytes.Buffer
	session, err := selectREPLSession(
		context.Background(), chooserStore, root,
		bufio.NewScanner(strings.NewReader("2\n\n")), &out,
	)
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.FindProjectByRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	want := memory.ProjectDisplayLabel("", project.CreatedAt)
	if session.ProjectID != project.ID || project.DisplayName != want || replProjectLabel(project) != want {
		t.Fatalf("registered session=%+v project=%+v, want persisted label %q", session, project, want)
	}
	if !strings.Contains(out.String(), "Project name: ") || strings.Contains(out.String(), "Project name [Project]") {
		t.Fatalf("unsafe basename used a timestamp-free prompt fallback: %q", out.String())
	}
}

func TestSelectREPLSessionRefreshesConcurrentRegistrationWithoutScopeGrant(t *testing.T) {
	root := t.TempDir()
	project := memory.Project{ID: "concurrent", DisplayName: "Concurrent", CanonicalRoot: root}
	store := &fakeREPLSessionStore{registerErr: errors.New("unique root collision"), findProject: project}
	store.listHook = func() ([]memory.Project, []memory.SessionListing) {
		if store.registerCalls > 0 {
			return []memory.Project{project}, nil
		}
		return nil, nil
	}
	var out bytes.Buffer
	session, err := selectREPLSession(context.Background(), store, root, bufio.NewScanner(strings.NewReader("2\nMine\n1\n")), &out)
	if err != nil {
		t.Fatal(err)
	}
	if session.ProjectID != project.ID || len(store.createdProject) != 1 {
		t.Fatalf("concurrent registration result=%+v created=%v", session, store.createdProject)
	}
	if !strings.Contains(out.String(), replStateChanged) || strings.Count(out.String(), "Select session:") != 2 {
		t.Fatalf("chooser did not require refreshed explicit choice: %q", out.String())
	}
}

func TestSelectREPLSessionRefreshesArchiveBetweenRenderAndCreate(t *testing.T) {
	root := t.TempDir()
	project := memory.Project{ID: "project", DisplayName: "Project", CanonicalRoot: root}
	archived := false
	store := &fakeREPLSessionStore{}
	store.listHook = func() ([]memory.Project, []memory.SessionListing) {
		project.Archived = archived
		return []memory.Project{project}, nil
	}
	store.createHook = func(memory.ProjectID) (memory.Session, error) {
		archived = true
		return memory.Session{}, eviedb.ErrChooserStateChanged
	}
	var out bytes.Buffer
	session, err := selectREPLSession(context.Background(), store, root, bufio.NewScanner(strings.NewReader("1\n1\n")), &out)
	if err != nil {
		t.Fatal(err)
	}
	if session.ProjectID != "" || store.createdGlobal != 1 || len(store.createdProject) != 1 {
		t.Fatalf("archive race silently selected scope: session=%+v project=%v global=%d", session, store.createdProject, store.createdGlobal)
	}
	if !strings.Contains(out.String(), replStateChanged) {
		t.Fatalf("archive race notice missing: %q", out.String())
	}
}

func TestSelectREPLSessionRefreshesMissingProjectBeforeCreate(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	base := eviedb.NewStore(db)
	root := t.TempDir()
	project, err := base.RegisterProject(context.Background(), "Vanishing", root)
	if err != nil {
		t.Fatal(err)
	}
	store := &deletingProjectREPLStore{Store: base, db: db, deleteNext: true}
	var out bytes.Buffer
	selected, err := selectREPLSession(
		context.Background(), store, root,
		bufio.NewScanner(strings.NewReader("1\n1\n")), &out,
	)
	if err != nil {
		t.Fatal(err)
	}
	var sessions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if store.sessionsAtReject != 0 || sessions != 1 || selected.ProjectID != "" {
		t.Fatalf("missing-project race selected=%+v sessions-at-reject=%d final-sessions=%d", selected, store.sessionsAtReject, sessions)
	}
	if !strings.Contains(out.String(), replStateChanged) || strings.Count(out.String(), "Select session:") != 2 ||
		strings.Count(out.String(), "Vanishing") != 1 {
		t.Fatalf("missing project did not refresh and require reselection: %q", out.String())
	}
	if _, err := base.FindProjectByRoot(context.Background(), project.CanonicalRoot); !errors.Is(err, eviedb.ErrProjectNotFound) {
		t.Fatalf("deleted project lookup error=%v, want ErrProjectNotFound", err)
	}
}

func TestSelectREPLSessionRefreshesRelocationBeforeProjectCreation(t *testing.T) {
	tests := []struct {
		name       string
		registered bool
		input      string
	}{
		{name: "rendered existing project", registered: true, input: "1\n1\n"},
		{name: "registration to creation", input: "2\nRegistered\n1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			base := eviedb.NewStore(db)
			launchRoot := t.TempDir()
			if tt.registered {
				if _, err := base.RegisterProject(context.Background(), "Existing", launchRoot); err != nil {
					t.Fatal(err)
				}
			}
			store := &relocatingREPLStore{
				Store: base, db: db, newRoot: t.TempDir(), relocateNext: true,
			}
			var out bytes.Buffer
			session, err := selectREPLSession(
				context.Background(), store, launchRoot,
				bufio.NewScanner(strings.NewReader(tt.input)), &out,
			)
			if err != nil {
				t.Fatal(err)
			}
			if store.sessionsAtReject != 0 {
				t.Fatalf("relocation created %d sessions before reselection", store.sessionsAtReject)
			}
			if session.ProjectRootSnapshot != store.relocatedRoot {
				t.Fatalf("selected root=%q, want refreshed root=%q", session.ProjectRootSnapshot, store.relocatedRoot)
			}
			if !strings.Contains(out.String(), replStateChanged) ||
				!strings.Contains(out.String(), escapedREPLPath(store.relocatedRoot)) ||
				strings.Count(out.String(), "Select session:") != 2 {
				t.Fatalf("relocation did not refresh with new root and explicit reselection: %q", out.String())
			}
		})
	}
}

func TestSelectREPLSessionRefreshesNewCWDOwnerBeforeEveryAction(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "project new", input: "1\n4\n"},
		{name: "global new", input: "3\n4\n"},
		{name: "existing session", input: "2\n4\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			base := eviedb.NewStore(db)
			project, err := base.RegisterProject(context.Background(), "Other", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := base.CreateProjectSession(context.Background(), project.ID); err != nil {
				t.Fatal(err)
			}
			cwd := t.TempDir()
			store := &concurrentCWDREPLStore{Store: base, db: db, cwd: cwd, triggerNext: true}
			var out bytes.Buffer
			selected, err := selectREPLSession(
				context.Background(), store, cwd,
				bufio.NewScanner(strings.NewReader(tt.input)), &out,
			)
			if err != nil {
				t.Fatal(err)
			}
			if selected.ProjectID != "" || store.sessionsAtReject != 1 {
				t.Fatalf("stale action granted scope: selected=%+v sessions-at-reject=%d", selected, store.sessionsAtReject)
			}
			if !strings.Contains(out.String(), replStateChanged) ||
				!strings.Contains(out.String(), "(current directory)") ||
				strings.Count(out.String(), "Select session:") != 2 {
				t.Fatalf("new cwd owner did not force refresh: %q", out.String())
			}
		})
	}
}

func TestSelectREPLSessionClosedSelectionRefreshes(t *testing.T) {
	root := t.TempDir()
	session := memory.Session{ID: "closed-after-render", CreatedAt: time.Now()}
	closed := false
	store := &fakeREPLSessionStore{}
	store.listHook = func() ([]memory.Project, []memory.SessionListing) {
		if closed {
			return nil, nil
		}
		return nil, []memory.SessionListing{{Session: session}}
	}
	store.getHook = func(memory.SessionID) (memory.Session, error) {
		closed = true
		return memory.Session{}, eviedb.ErrSessionNotActive
	}
	var out bytes.Buffer
	selected, err := selectREPLSession(context.Background(), store, root, bufio.NewScanner(strings.NewReader("2\n1\n")), &out)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "new-global" || len(store.activeGets) != 1 {
		t.Fatalf("closed selection result=%+v gets=%v", selected, store.activeGets)
	}
}

func TestArchivedExactCWDHasNoSuggestionOrRegistration(t *testing.T) {
	root := t.TempDir()
	project := memory.Project{ID: "archived", DisplayName: "Archived", CanonicalRoot: root, Archived: true}
	session := memory.Session{ID: "resume", ProjectID: project.ID, ProjectRootSnapshot: root, CreatedAt: time.Now()}
	var out bytes.Buffer
	actions, err := renderREPLChooser(&out, root, []memory.Project{project}, []memory.SessionListing{{Session: session}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "current directory") || strings.Contains(out.String(), "Register current directory") {
		t.Fatalf("archived cwd received suggestion: %q", out.String())
	}
	if len(actions) != 2 || actions[0].kind != replActionResume || actions[1].kind != replActionNewGlobal {
		t.Fatalf("archived actions = %+v", actions)
	}
}

func TestSelectREPLSessionRequiresInput(t *testing.T) {
	root := t.TempDir()
	store := &fakeREPLSessionStore{}
	_, err := selectREPLSession(context.Background(), store, root, bufio.NewScanner(strings.NewReader("\n")), io.Discard)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("selection error = %v, want EOF after invalid blank", err)
	}
	if store.createdGlobal != 0 || len(store.createdProject) != 0 || store.registerCalls != 0 {
		t.Fatalf("blank input granted scope: global=%d project=%v register=%d", store.createdGlobal, store.createdProject, store.registerCalls)
	}
}

func TestSelectREPLSessionRealStoreResumesArchivedSession(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := eviedb.NewStore(db)
	root := t.TempDir()
	project, err := store.RegisterProject(context.Background(), "Archived", root)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateProjectSession(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE projects SET archived = 1 WHERE id = ?`, project.ID); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	selected, err := selectREPLSession(context.Background(), &withoutWorkspaceREPLStore{Store: store}, root, bufio.NewScanner(strings.NewReader("1\n")), &out)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != created.ID || selected.ScopeContext() != created.ScopeContext() {
		t.Fatalf("resumed scope=%+v, want %+v", selected.ScopeContext(), created.ScopeContext())
	}
	if !strings.Contains(out.String(), "(archived)") {
		t.Fatalf("archived heading missing: %q", out.String())
	}
}

func Example_renderREPLChooser() {
	var out bytes.Buffer
	_, _ = renderREPLChooser(&out, "/work", nil, nil)
	fmt.Print(out.String())
	// Output:
	// Sessions
	// Global
	//   1. New session
	//   2. Register current directory — "/work"
}
