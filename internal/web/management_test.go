package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

type managementReceiptStore struct {
	receipt     composition.Receipt
	resolutions []composition.CompatibilityResolution
	err         error
}

func (s managementReceiptStore) GetCompositionReceipt(context.Context, memory.SessionID) (composition.Receipt, error) {
	return s.receipt, s.err
}

func (s managementReceiptStore) GetCompatibilityResolutions(context.Context, memory.SessionID) ([]composition.CompatibilityResolution, error) {
	return append([]composition.CompatibilityResolution(nil), s.resolutions...), s.err
}

func managementRequest(path, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6687"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func managedBuiltinServer(t *testing.T) (*plugins.Manager, http.Handler) {
	t.Helper()
	manager, err := plugins.NewManager(tools.NewToolset(nil), plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(testTaskStore(t)))
	if err != nil {
		t.Fatal(err)
	}
	return manager, NewManagedServer(nil, manager, managementReceiptStore{}).Handler()
}

func TestHTTPManagementListsAndValidatesWithoutNeedingAChatSession(t *testing.T) {
	manager, handler := managedBuiltinServer(t)
	missingResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingResponse, managementRequest("/api/presets/validate", `{"id":"standard"}`))
	if missingResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unavailable standard status=%d body=%s", missingResponse.Code, missingResponse.Body.String())
	}
	for _, requirement := range plugins.BuiltinStandardPreset().RequiredCapabilities {
		if !strings.Contains(missingResponse.Body.String(), string(requirement.ID)) {
			t.Fatalf("unavailable standard omitted %q: %s", requirement.ID, missingResponse.Body.String())
		}
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}

	pluginsResponse := httptest.NewRecorder()
	handler.ServeHTTP(pluginsResponse, managementRequest("/api/plugins/list", `{}`))
	if pluginsResponse.Code != http.StatusOK ||
		!strings.Contains(pluginsResponse.Body.String(), `"lifecycle":"ready"`) ||
		!strings.Contains(pluginsResponse.Body.String(), `"connectionReadiness":{"state":"not_required"}`) {
		t.Fatalf("plugin list status=%d body=%s", pluginsResponse.Code, pluginsResponse.Body.String())
	}

	presetsResponse := httptest.NewRecorder()
	handler.ServeHTTP(presetsResponse, managementRequest("/api/presets/list", `{}`))
	if presetsResponse.Code != http.StatusOK ||
		!strings.Contains(presetsResponse.Body.String(), `"id":"standard"`) ||
		!strings.Contains(presetsResponse.Body.String(), `"immutable":true`) ||
		!strings.Contains(presetsResponse.Body.String(), `"valid":true`) {
		t.Fatalf("preset list status=%d body=%s", presetsResponse.Code, presetsResponse.Body.String())
	}

	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, managementRequest("/api/presets/validate", `{"id":"missing"}`))
	if invalidResponse.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(invalidResponse.Body.String(), `Agent Preset \"missing\" is not allowed`) {
		t.Fatalf("invalid preset status=%d body=%s", invalidResponse.Code, invalidResponse.Body.String())
	}
}

func TestHTTPManagementEnableDisableAndConcurrentLifecycleRequests(t *testing.T) {
	_, handler := managedBuiltinServer(t)
	disable := httptest.NewRecorder()
	handler.ServeHTTP(disable, managementRequest("/api/plugins/lifecycle", `{"id":"web","enabled":false}`))
	if disable.Code != http.StatusOK ||
		!strings.Contains(disable.Body.String(), `"from":"disabled"`) ||
		!strings.Contains(disable.Body.String(), `"to":"disabled"`) {
		t.Fatalf("disable status=%d body=%s", disable.Code, disable.Body.String())
	}

	const requests = 12
	var wg sync.WaitGroup
	statuses := make(chan int, requests)
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(enabled bool) {
			defer wg.Done()
			recorder := httptest.NewRecorder()
			body, _ := json.Marshal(map[string]any{"id": "web", "enabled": enabled})
			handler.ServeHTTP(recorder, managementRequest("/api/plugins/lifecycle", string(body)))
			statuses <- recorder.Code
		}(i%2 == 0)
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("concurrent lifecycle status = %d", status)
		}
	}
}

type failedManagementPlugin struct{}

const managementSecret = "oauth-client-secret-never-expose"

func (failedManagementPlugin) Manifest() plugins.Manifest {
	return plugins.Manifest{
		ID: "broken", ImplementationVersion: "1.0.0",
		KernelCompatibility: plugins.VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
	}
}
func (failedManagementPlugin) Start(context.Context) error { return errors.New(managementSecret) }
func (failedManagementPlugin) Stop(context.Context) error  { return nil }
func (failedManagementPlugin) ConnectionReadiness() plugins.ConnectionReadiness {
	return plugins.ConnectionReadiness{State: plugins.ConnectionNotReady, Diagnostic: managementSecret}
}

type secretStopManagementPlugin struct{}

func (secretStopManagementPlugin) Manifest() plugins.Manifest {
	return plugins.Manifest{
		ID: "secret-stop", ImplementationVersion: "1.0.0",
		KernelCompatibility: plugins.VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
	}
}
func (secretStopManagementPlugin) Start(context.Context) error { return nil }
func (secretStopManagementPlugin) Stop(context.Context) error  { return errors.New(managementSecret) }

func TestHTTPManagementRemainsAvailableDuringDegradedStartup(t *testing.T) {
	manager, err := plugins.NewManager(tools.NewToolset(nil), failedManagementPlugin{})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), "broken"); err != nil {
		t.Fatal(err)
	}
	handler := NewManagedServer(nil, manager, managementReceiptStore{}).Handler()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, managementRequest("/api/plugins/list", `{}`))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"degraded":true`) ||
		!strings.Contains(recorder.Body.String(), `"health":"unhealthy"`) {
		t.Fatalf("degraded list status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), managementSecret) {
		t.Fatalf("plugin secret escaped degraded HTTP inspection: %s", recorder.Body.String())
	}
	chat := httptest.NewRecorder()
	handler.ServeHTTP(chat, chatRequest(`{"message":"hi"}`))
	if chat.Code != http.StatusServiceUnavailable {
		t.Fatalf("degraded chat status=%d body=%s", chat.Code, chat.Body.String())
	}
}

func TestHTTPLifecycleErrorNeverExposesRawStopError(t *testing.T) {
	manager, err := plugins.NewManager(tools.NewToolset(nil), secretStopManagementPlugin{})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), "secret-stop"); err != nil {
		t.Fatal(err)
	}
	handler := NewManagedServer(nil, manager, managementReceiptStore{}).Handler()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, managementRequest("/api/plugins/lifecycle", `{"id":"secret-stop","enabled":false}`))
	if recorder.Code != http.StatusUnprocessableEntity || strings.Contains(recorder.Body.String(), managementSecret) ||
		!strings.Contains(recorder.Body.String(), `plugin \"secret-stop\" cleanup failed`) {
		t.Fatalf("secret-safe lifecycle status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHTTPLifecyclePersistsAcrossFreshManagerAndSQLiteReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	manager, _ := managedBuiltinServer(t)
	defaults := map[plugins.PluginID]bool{plugins.WebPluginID: true, plugins.FinancePluginID: true}
	if err := manager.ConfigureEnabledState(context.Background(), store, defaults); err != nil {
		t.Fatal(err)
	}
	handler := NewManagedServer(nil, manager, store).Handler()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, managementRequest("/api/plugins/lifecycle", `{"id":"web","enabled":false}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	restarted, _ := managedBuiltinServer(t)
	if err := restarted.ConfigureEnabledState(context.Background(), eviedb.NewStore(db), defaults); err != nil {
		t.Fatal(err)
	}
	status, err := restarted.Status(plugins.WebPluginID)
	if err != nil || status.Enabled || status.State != plugins.StateDisabled {
		t.Fatalf("restarted Web status=%+v err=%v", status, err)
	}
}

func TestRunningHTTPManagerRefreshesExternalSQLiteChangesAndPublishesItsOwn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbA.Close()
	dbB, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()
	storeA, storeB := eviedb.NewStore(dbA), eviedb.NewStore(dbB)
	managerA, _ := managedBuiltinServer(t)
	managerB, _ := managedBuiltinServer(t)
	defaults := map[plugins.PluginID]bool{plugins.WebPluginID: true, plugins.FinancePluginID: true}
	if err := managerA.ConfigureEnabledState(context.Background(), storeA, defaults); err != nil {
		t.Fatal(err)
	}
	if err := managerB.ConfigureEnabledState(context.Background(), storeB, defaults); err != nil {
		t.Fatal(err)
	}
	if _, err := storeA.SetPluginEnabled(context.Background(), string(plugins.WebPluginID), false); err != nil {
		t.Fatal(err)
	}
	handler := NewManagedServer(nil, managerB, storeB).Handler()
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, managementRequest("/api/plugins/list", `{}`))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"id":"web","version":"1.0.0","enabled":false`) {
		t.Fatalf("running web list did not refresh CLI change: status=%d body=%s", list.Code, list.Body.String())
	}
	enable := httptest.NewRecorder()
	handler.ServeHTTP(enable, managementRequest("/api/plugins/lifecycle", `{"id":"web","enabled":true}`))
	if enable.Code != http.StatusOK {
		t.Fatalf("web enable status=%d body=%s", enable.Code, enable.Body.String())
	}
	inspection, err := managerA.InspectContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range inspection.Plugins {
		if status.ID == plugins.WebPluginID && !status.Enabled {
			t.Fatalf("CLI Manager did not refresh web lifecycle change: %+v", status)
		}
	}
}

func TestHTTPSessionInspectionReturnsReceiptAndResolutionsWithoutSecrets(t *testing.T) {
	manager, _ := managedBuiltinServer(t)
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	resolution := composition.CompatibilityResolution{
		OriginalProvider:                 resolved.Receipt.Providers[0],
		ReplacementImplementationVersion: "1.1.0",
		Capabilities: []composition.CompatibilityCapability{{
			ID:              resolved.Receipt.Capabilities[0].ID,
			ContractVersion: resolved.Receipt.Capabilities[0].ContractVersion,
			SchemaSHA256:    resolved.Receipt.Capabilities[0].SchemaSHA256,
		}},
		KernelAPIVersion: plugins.KernelAPIVersion,
		ResolvedAt:       time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
	}
	handler := NewManagedServer(nil, manager, managementReceiptStore{
		receipt: resolved.Receipt, resolutions: []composition.CompatibilityResolution{resolution},
	}).Handler()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, managementRequest("/api/sessions/inspect", `{"sessionId":"session-1"}`))
	if recorder.Code != http.StatusOK ||
		!strings.Contains(recorder.Body.String(), `"receipt"`) ||
		!strings.Contains(recorder.Body.String(), `"compatibilityResolutions"`) ||
		strings.Contains(strings.ToLower(recorder.Body.String()), "secret") {
		t.Fatalf("session inspection status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHTTPSessionInspectionMapsPublicErrorsWithoutRawStorageDetails(t *testing.T) {
	manager, _ := managedBuiltinServer(t)
	for name, tc := range map[string]struct {
		body string
		err  error
		want int
		code string
	}{
		"invalid":   {body: `{"sessionId":" "}`, want: http.StatusBadRequest, code: "invalid_session_id"},
		"not found": {body: `{"sessionId":"missing"}`, err: eviedb.ErrCompositionReceiptNotFound, want: http.StatusNotFound, code: "session_not_found"},
		"internal":  {body: `{"sessionId":"session-1"}`, err: errors.New(managementSecret), want: http.StatusInternalServerError, code: "session_inspection_unavailable"},
	} {
		t.Run(name, func(t *testing.T) {
			handler := NewManagedServer(nil, manager, managementReceiptStore{err: tc.err}).Handler()
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, managementRequest("/api/sessions/inspect", tc.body))
			if recorder.Code != tc.want || !strings.Contains(recorder.Body.String(), `"code":"`+tc.code+`"`) ||
				strings.Contains(recorder.Body.String(), managementSecret) {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestManagementRoutesRetainOriginAndContentTypeDefenses(t *testing.T) {
	_, handler := managedBuiltinServer(t)
	badType := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6687/api/plugins/list", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain")
	handler.ServeHTTP(badType, req)
	if badType.Code != http.StatusForbidden {
		t.Fatalf("bad content type status=%d", badType.Code)
	}
	badOrigin := httptest.NewRecorder()
	req = managementRequest("/api/plugins/list", `{}`)
	req.Header.Set("Origin", "https://attacker.example")
	handler.ServeHTTP(badOrigin, req)
	if badOrigin.Code != http.StatusForbidden {
		t.Fatalf("bad origin status=%d", badOrigin.Code)
	}

	for name, tc := range map[string]struct {
		request *http.Request
		want    int
	}{
		"wrong method": {
			request: httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6687/api/plugins/list", nil),
			want:    http.StatusMethodNotAllowed,
		},
		"unknown list field": {request: managementRequest("/api/plugins/list", `{"extra":true}`), want: http.StatusBadRequest},
		"null list body":     {request: managementRequest("/api/plugins/list", `null`), want: http.StatusBadRequest},
		"trailing JSON":      {request: managementRequest("/api/presets/list", `{} {}`), want: http.StatusBadRequest},
		"missing preset ID":  {request: managementRequest("/api/presets/validate", `{}`), want: http.StatusBadRequest},
		"oversized body":     {request: managementRequest("/api/plugins/lifecycle", `{"id":"`+strings.Repeat("x", 5000)+`","enabled":true}`), want: http.StatusRequestEntityTooLarge},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, tc.request)
		if recorder.Code != tc.want {
			t.Errorf("%s status=%d body=%s, want %d", name, recorder.Code, recorder.Body.String(), tc.want)
		}
	}
}
