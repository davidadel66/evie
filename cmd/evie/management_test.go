package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/web"
)

type cliReceiptStore struct {
	receipt composition.Receipt
}

func (s cliReceiptStore) GetCompositionReceipt(context.Context, memory.SessionID) (composition.Receipt, error) {
	return s.receipt, nil
}
func (cliReceiptStore) GetCompatibilityResolutions(context.Context, memory.SessionID) ([]composition.CompatibilityResolution, error) {
	return []composition.CompatibilityResolution{}, nil
}

func cliManager(t *testing.T) *plugins.Manager {
	t.Helper()
	manager, err := plugins.NewManager(tools.NewToolset(nil), plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(testTaskStore(t)))
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestManagementCLIListsAndChangesCompiledPlugins(t *testing.T) {
	manager := cliManager(t)
	var output bytes.Buffer
	handled, err := runManagementCommand(context.Background(), []string{"plugins", "list"}, &output, manager, cliReceiptStore{})
	if err != nil || !handled || !strings.Contains(output.String(), `"id": "web"`) ||
		!strings.Contains(output.String(), `"lifecycle": "disabled"`) {
		t.Fatalf("plugins list handled=%v err=%v output=%s", handled, err, output.String())
	}

	output.Reset()
	handled, err = runManagementCommand(context.Background(), []string{"plugins", "enable", "web"}, &output, manager, cliReceiptStore{})
	if err != nil || !handled || !strings.Contains(output.String(), `"from": "disabled"`) ||
		!strings.Contains(output.String(), `"to": "ready"`) {
		t.Fatalf("plugins enable handled=%v err=%v output=%s", handled, err, output.String())
	}

	output.Reset()
	handled, err = runManagementCommand(context.Background(), []string{"plugins", "disable", "web"}, &output, manager, cliReceiptStore{})
	if err != nil || !handled || !strings.Contains(output.String(), `"from": "ready"`) ||
		!strings.Contains(output.String(), `"to": "disabled"`) {
		t.Fatalf("plugins disable handled=%v err=%v output=%s", handled, err, output.String())
	}
}

func TestManagementCLIValidationFailurePrintsEveryDiagnostic(t *testing.T) {
	manager := cliManager(t)
	var output bytes.Buffer
	handled, err := runManagementCommand(context.Background(), []string{"presets", "validate", "standard"}, &output, manager, cliReceiptStore{})
	if !handled || err == nil {
		t.Fatalf("validate handled=%v err=%v output=%s", handled, err, output.String())
	}
	for _, requirement := range plugins.BuiltinStandardPreset().RequiredCapabilities {
		if !strings.Contains(output.String(), string(requirement.ID)) {
			t.Fatalf("missing %q diagnostic in %s", requirement.ID, output.String())
		}
	}
}

func TestManagementCLISessionInspectionShowsPinnedAuditData(t *testing.T) {
	manager := cliManager(t)
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	handled, err := runManagementCommand(
		context.Background(), []string{"sessions", "inspect", "session-1"}, &output,
		manager, cliReceiptStore{receipt: resolved.Receipt},
	)
	if err != nil || !handled || !strings.Contains(output.String(), `"receipt"`) ||
		!strings.Contains(output.String(), `"compatibilityResolutions": []`) {
		t.Fatalf("session inspect handled=%v err=%v output=%s", handled, err, output.String())
	}
}

func TestManagementCLISessionInspectionMapsErrorsWithoutRawStorageDetails(t *testing.T) {
	secret := "sqlite-password-never-print"
	for name, tc := range map[string]struct {
		id   string
		err  error
		want string
	}{
		"invalid":   {id: " ", want: "invalid_session_id"},
		"not found": {id: "missing", err: eviedb.ErrCompositionReceiptNotFound, want: "session_not_found"},
		"internal":  {id: "session-1", err: errors.New(secret), want: "session_inspection_unavailable"},
	} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			_, err := runManagementCommand(context.Background(), []string{"sessions", "inspect", tc.id}, &output, cliManager(t), cliErrorReceiptStore{err: tc.err})
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error()+output.String(), secret) {
				t.Fatalf("err=%v output=%s", err, output.String())
			}
		})
	}
}

type cliErrorReceiptStore struct{ err error }

func (s cliErrorReceiptStore) GetCompositionReceipt(context.Context, memory.SessionID) (composition.Receipt, error) {
	return composition.Receipt{}, s.err
}

func (s cliErrorReceiptStore) GetCompatibilityResolutions(context.Context, memory.SessionID) ([]composition.CompatibilityResolution, error) {
	return nil, s.err
}

type failedCLIPlugin struct{}

const cliManagementSecret = "sk-cli-secret-never-expose"

func (failedCLIPlugin) Manifest() plugins.Manifest {
	return plugins.Manifest{
		ID: "broken", ImplementationVersion: "1.0.0",
		KernelCompatibility: plugins.VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
	}
}
func (failedCLIPlugin) Start(context.Context) error { return errors.New(cliManagementSecret) }
func (failedCLIPlugin) Stop(context.Context) error  { return nil }
func (failedCLIPlugin) ConnectionReadiness() plugins.ConnectionReadiness {
	return plugins.ConnectionReadiness{State: plugins.ConnectionNotReady, Diagnostic: cliManagementSecret}
}

type secretStopCLIPlugin struct{}

func (secretStopCLIPlugin) Manifest() plugins.Manifest {
	return plugins.Manifest{
		ID: "secret-stop", ImplementationVersion: "1.0.0",
		KernelCompatibility: plugins.VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
	}
}
func (secretStopCLIPlugin) Start(context.Context) error { return nil }
func (secretStopCLIPlugin) Stop(context.Context) error  { return errors.New(cliManagementSecret) }

func TestManagementCLIListsDegradedStartupAndSerializesConcurrentLifecycle(t *testing.T) {
	degraded, err := plugins.NewManager(tools.NewToolset(nil), failedCLIPlugin{})
	if err != nil {
		t.Fatal(err)
	}
	if err := degraded.Enable(context.Background(), "broken"); err != nil {
		t.Fatal(err)
	}
	var degradedOutput bytes.Buffer
	if _, err := runManagementCommand(context.Background(), []string{"plugins", "list"}, &degradedOutput, degraded, cliReceiptStore{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(degradedOutput.String(), `"degraded": true`) ||
		!strings.Contains(degradedOutput.String(), `"health": "unhealthy"`) {
		t.Fatalf("degraded CLI output=%s", degradedOutput.String())
	}
	if strings.Contains(degradedOutput.String(), cliManagementSecret) {
		t.Fatalf("plugin secret escaped CLI output: %s", degradedOutput.String())
	}

	manager := cliManager(t)
	const requests = 10
	var wg sync.WaitGroup
	errs := make(chan error, requests)
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(enable bool) {
			defer wg.Done()
			action := "disable"
			if enable {
				action = "enable"
			}
			var output bytes.Buffer
			_, err := runManagementCommand(context.Background(), []string{"plugins", action, "web"}, &output, manager, cliReceiptStore{})
			errs <- err
		}(i%2 == 0)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent CLI lifecycle: %v", err)
		}
	}
}

func TestManagementCLILifecycleErrorNeverExposesRawPluginError(t *testing.T) {
	manager, err := plugins.NewManager(tools.NewToolset(nil), secretStopCLIPlugin{})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), "secret-stop"); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	_, err = runManagementCommand(context.Background(), []string{"plugins", "disable", "secret-stop"}, &output, manager, cliReceiptStore{})
	serialized := output.String() + " " + errorString(err)
	if err == nil || strings.Contains(serialized, cliManagementSecret) ||
		!strings.Contains(serialized, `plugin "secret-stop" cleanup failed`) {
		t.Fatalf("secret-safe CLI result=%s", serialized)
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestManagementCLIPersistsDesiredStateAcrossFreshManagerAndSQLiteReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	manager := cliManager(t)
	defaults := map[plugins.PluginID]bool{plugins.WebPluginID: true, plugins.FinancePluginID: true}
	if err := manager.ConfigureEnabledState(context.Background(), store, defaults); err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID} {
		status, _ := manager.Status(id)
		if !status.Enabled || status.State != plugins.StateReady {
			t.Fatalf("fresh default status(%q)=%+v", id, status)
		}
	}
	const concurrentRequests = 12
	var wg sync.WaitGroup
	errs := make(chan error, concurrentRequests)
	for i := range concurrentRequests {
		wg.Add(1)
		go func(enabled bool) {
			defer wg.Done()
			action := "disable"
			if enabled {
				action = "enable"
			}
			var concurrentOutput bytes.Buffer
			_, err := runManagementCommand(context.Background(), []string{"plugins", action, "web"}, &concurrentOutput, manager, store)
			errs <- err
		}(i%2 == 0)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	durableEnabled, _, found, err := store.PluginEnabled(context.Background(), "web")
	if err != nil || !found {
		t.Fatalf("concurrent durable Web state found=%v err=%v", found, err)
	}
	concurrentStatus, _ := manager.Status(plugins.WebPluginID)
	if durableEnabled != concurrentStatus.Enabled {
		t.Fatalf("concurrent durable=%v manager=%v", durableEnabled, concurrentStatus.Enabled)
	}
	var output bytes.Buffer
	if _, err := runManagementCommand(context.Background(), []string{"plugins", "disable", "web"}, &output, manager, store); err != nil {
		t.Fatal(err)
	}
	if _, err := runManagementCommand(context.Background(), []string{"plugins", "disable", "missing"}, &output, manager, store); err == nil {
		t.Fatal("unknown CLI plugin was accepted")
	}
	if _, _, found, err := store.PluginEnabled(context.Background(), "missing"); err != nil || found {
		t.Fatalf("unknown CLI plugin persisted found=%v err=%v", found, err)
	}
	if _, err := store.SetPluginEnabled(context.Background(), "removed", true); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	restarted := cliManager(t)
	if err := restarted.ConfigureEnabledState(context.Background(), eviedb.NewStore(db), defaults); err != nil {
		t.Fatal(err)
	}
	webStatus, _ := restarted.Status(plugins.WebPluginID)
	financeStatus, _ := restarted.Status(plugins.FinancePluginID)
	if webStatus.Enabled || webStatus.State != plugins.StateDisabled ||
		!financeStatus.Enabled || financeStatus.State != plugins.StateReady {
		t.Fatalf("reopened statuses web=%+v finance=%+v", webStatus, financeStatus)
	}
}

func TestCLIAndRunningWebManagerConvergeThroughSQLiteOnEachRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	dbCLI, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbCLI.Close()
	dbWeb, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbWeb.Close()
	storeCLI, storeWeb := eviedb.NewStore(dbCLI), eviedb.NewStore(dbWeb)
	cli, runningWeb := cliManager(t), cliManager(t)
	defaults := map[plugins.PluginID]bool{plugins.WebPluginID: true, plugins.FinancePluginID: true}
	if err := cli.ConfigureEnabledState(context.Background(), storeCLI, defaults); err != nil {
		t.Fatal(err)
	}
	if err := runningWeb.ConfigureEnabledState(context.Background(), storeWeb, defaults); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if _, err := runManagementCommand(context.Background(), []string{"plugins", "disable", "web"}, &output, cli, storeCLI); err != nil {
		t.Fatal(err)
	}
	handler := web.NewManagedServer(nil, runningWeb, storeWeb).Handler()
	request := func(path, body string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6687"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		return req
	}
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, request("/api/plugins/list", `{}`))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"id":"web","version":"1.0.0","enabled":false`) {
		t.Fatalf("running web list did not observe CLI disable: status=%d body=%s", list.Code, list.Body.String())
	}
	if _, err := runningWeb.ResolvePreset(plugins.StandardPresetID); err == nil {
		t.Fatal("running web Manager composed a new standard session from stale state")
	}
	enable := httptest.NewRecorder()
	handler.ServeHTTP(enable, request("/api/plugins/lifecycle", `{"id":"web","enabled":true}`))
	if enable.Code != http.StatusOK {
		t.Fatalf("web enable status=%d body=%s", enable.Code, enable.Body.String())
	}
	output.Reset()
	if _, err := runManagementCommand(context.Background(), []string{"plugins", "list"}, &output, cli, storeCLI); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"enabled": true`) {
		t.Fatalf("CLI list did not observe web enable: %s", output.String())
	}
}
