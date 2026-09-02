package plugins

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/tools"
)

type failingStopManagementPlugin struct {
	fakePlugin
	stopErr error
}

func mustInspect(t *testing.T, manager *Manager) Inspection {
	t.Helper()
	inspection, err := manager.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	return inspection
}

func mustInspectPresets(t *testing.T, manager *Manager) []PresetInspection {
	t.Helper()
	presets, err := manager.InspectPresets()
	if err != nil {
		t.Fatal(err)
	}
	return presets
}

func mustValidatePreset(t *testing.T, manager *Manager, id PresetID) PresetInspection {
	t.Helper()
	report, err := manager.ValidatePreset(id)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

type failingStartManagementPlugin struct{ fakePlugin }

func (p failingStartManagementPlugin) Start(context.Context) error {
	return errors.New("private-provider-start-credential")
}

type countedFailingStartPlugin struct {
	fakePlugin
	starts *int
}

func (p countedFailingStartPlugin) Start(context.Context) error {
	(*p.starts)++
	return errors.New("private-provider-start-credential")
}

type countedStartPlugin struct {
	fakePlugin
	starts *int
}

func (p countedStartPlugin) Start(context.Context) error {
	(*p.starts)++
	return nil
}

type scriptedCleanupPlugin struct {
	fakePlugin
	mu              sync.Mutex
	starts          int
	stops           int
	failingStarts   int
	failingCleanups int
}

func (p *scriptedCleanupPlugin) Start(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.starts++
	if p.starts <= p.failingStarts {
		return errors.New("private-scripted-start-credential")
	}
	return nil
}

func (p *scriptedCleanupPlugin) Stop(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stops++
	if p.stops <= p.failingCleanups {
		return errors.New("private-scripted-cleanup-credential")
	}
	return nil
}

func (p *scriptedCleanupPlugin) counts() (starts, stops int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.starts, p.stops
}

type canceledSecretManagementPlugin struct{ fakePlugin }

func (p canceledSecretManagementPlugin) Start(context.Context) error {
	return fmt.Errorf("private-provider-context-credential: %w", context.Canceled)
}

func (p failingStopManagementPlugin) Stop(context.Context) error { return p.stopErr }

type enabledStateMemoryStore struct {
	values    map[string]bool
	revisions map[string]uint64
	sets      []string
	err       error
}

func (s *enabledStateMemoryStore) ResolvePluginEnabled(_ context.Context, id string, defaultEnabled bool) (bool, uint64, error) {
	if s.err != nil {
		return false, 0, s.err
	}
	if value, ok := s.values[id]; ok {
		return value, s.revision(id), nil
	}
	if s.values == nil {
		s.values = map[string]bool{}
	}
	s.values[id] = defaultEnabled
	s.setRevision(id, 1)
	return defaultEnabled, 1, nil
}

func (s *enabledStateMemoryStore) PluginEnabled(_ context.Context, id string) (bool, uint64, bool, error) {
	if s.err != nil {
		return false, 0, false, s.err
	}
	value, found := s.values[id]
	return value, s.revision(id), found, nil
}

func (s *enabledStateMemoryStore) SetPluginEnabled(_ context.Context, id string, enabled bool) (uint64, error) {
	if s.err != nil {
		return 0, s.err
	}
	s.values[id] = enabled
	revision := s.revision(id) + 1
	if revision == 1 {
		revision = 2
	}
	s.setRevision(id, revision)
	s.sets = append(s.sets, id)
	return revision, nil
}

func (s *enabledStateMemoryStore) revision(id string) uint64 {
	if s.revisions == nil {
		return 1
	}
	if revision := s.revisions[id]; revision != 0 {
		return revision
	}
	return 1
}

func (s *enabledStateMemoryStore) setRevision(id string, revision uint64) {
	if s.revisions == nil {
		s.revisions = map[string]uint64{}
	}
	s.revisions[id] = revision
}

type connectionFixture struct {
	fakePlugin
	readiness ConnectionReadiness
}

type flappingConnectionPlugin struct {
	fakePlugin
	calls *int
}

type blockingReadinessPlugin struct {
	fakePlugin
	entered chan struct{}
	release chan struct{}
	active  int
	raced   bool
	calls   int
}

func (p *blockingReadinessPlugin) enterCallback() {
	p.active++
	if p.active != 1 {
		p.raced = true
	}
}

func (p *blockingReadinessPlugin) leaveCallback() { p.active-- }

func (p *blockingReadinessPlugin) Start(context.Context) error {
	p.enterCallback()
	defer p.leaveCallback()
	return nil
}

func (p *blockingReadinessPlugin) Stop(context.Context) error {
	p.enterCallback()
	defer p.leaveCallback()
	return nil
}

func (p *blockingReadinessPlugin) ConnectionReadiness() ConnectionReadiness {
	p.enterCallback()
	defer p.leaveCallback()
	p.calls++
	if p.entered != nil {
		p.entered <- struct{}{}
	}
	if p.release != nil {
		<-p.release
	}
	return ConnectionReadiness{State: ConnectionReady}
}

func (p flappingConnectionPlugin) ConnectionReadiness() ConnectionReadiness {
	*p.calls++
	if *p.calls%2 == 0 {
		return ConnectionReadiness{State: ConnectionReady}
	}
	return ConnectionReadiness{State: ConnectionNotReady, Diagnostic: "must not affect dependency reporting"}
}

func (p connectionFixture) ConnectionReadiness() ConnectionReadiness { return p.readiness }

func TestManagerInspectionDescribesCompiledPluginManagementState(t *testing.T) {
	secret := "oauth-token-never-render"
	account := connectionFixture{
		fakePlugin: fakeToolPlugin("account", "account.read", "account_read", "ok"),
		readiness: ConnectionReadiness{
			State: ConnectionReadinessState(secret), Diagnostic: secret, Message: secret,
		},
	}
	consumer := fakeToolPlugin("consumer", "consumer.use", "consumer_use", "ok")
	consumer.manifest.RequiredDependencies = []Dependency{{
		ID: "account", Compatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
	}}
	manager, err := NewManager(tools.NewToolset(nil), consumer, account)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{"account", "consumer"} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}

	inspection := mustInspect(t, manager)
	if inspection.Degraded {
		t.Fatalf("connection setup incorrectly degraded Plugin Health: %+v", inspection)
	}
	if len(inspection.Plugins) != 2 {
		t.Fatalf("plugins = %+v", inspection.Plugins)
	}
	got := inspection.Plugins[0]
	if got.ID != "account" || got.Version != "1.0.0" || !got.Enabled || got.State != StateReady ||
		got.Health != HealthHealthy || got.ConnectionReadiness.State != ConnectionNotReady ||
		got.ConnectionReadiness.Code != "connection_not_ready" ||
		got.ConnectionReadiness.Message != `plugin "account" requires connection setup` {
		t.Fatalf("account inspection = %+v", got)
	}
	if strings.Contains(fmt.Sprintf("%+v", inspection), secret) {
		t.Fatalf("provider readiness diagnostic escaped inspection: %+v", inspection)
	}
	got = inspection.Plugins[1]
	if !reflect.DeepEqual(got.RequiredDependencies, consumer.manifest.RequiredDependencies) ||
		got.ConnectionReadiness.State != ConnectionNotRequired {
		t.Fatalf("consumer inspection = %+v", got)
	}
}

func TestReadinessCallbackDoesNotHoldManagerLockAndIsSerializedPerProvider(t *testing.T) {
	plugin := &blockingReadinessPlugin{
		fakePlugin: fakeToolPlugin("account", "account.read", "account_read", "ok"),
		entered:    make(chan struct{}, 2),
		release:    make(chan struct{}, 2),
	}
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}
	enableDone := make(chan error, 1)
	go func() { enableDone <- manager.Enable(context.Background(), "account") }()
	<-plugin.entered
	blockedInspection := make(chan Inspection, 1)
	go func() {
		inspection, _ := manager.Inspect()
		blockedInspection <- inspection
	}()
	select {
	case inspection := <-blockedInspection:
		if inspection.Plugins[0].ConnectionReadiness.State != ConnectionNotRequired {
			t.Fatalf("inspection did not return prior cached readiness: %+v", inspection)
		}
	case <-time.After(time.Second):
		t.Fatal("inspection blocked on live readiness callback")
	}
	plugin.release <- struct{}{}
	if err := <-enableDone; err != nil {
		t.Fatal(err)
	}
	inspectionDone := make(chan struct{}, 2)
	for range 2 {
		go func() {
			inspection, _ := manager.Inspect()
			if inspection.Plugins[0].ConnectionReadiness.State != ConnectionReady {
				t.Errorf("cached readiness=%+v", inspection.Plugins[0].ConnectionReadiness)
			}
			inspectionDone <- struct{}{}
		}()
	}
	lifecycleDone := make(chan error, 1)
	go func() { lifecycleDone <- manager.Disable(context.Background(), "account") }()
	select {
	case err := <-lifecycleDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("lifecycle writer blocked behind plugin-owned readiness callback")
	}
	<-inspectionDone
	<-inspectionDone
	select {
	case <-plugin.entered:
		t.Fatal("inspection invoked live readiness callback")
	default:
	}
	if plugin.raced || plugin.calls != 1 {
		t.Fatalf("readiness callback serialization raced=%v calls=%d", plugin.raced, plugin.calls)
	}
}

func TestManagementCanonicalizesWrappedContextErrors(t *testing.T) {
	plugin := canceledSecretManagementPlugin{fakeToolPlugin("canceled", "canceled.echo", "canceled_echo", "ok")}
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}
	err = manager.Enable(context.Background(), "canceled")
	if err == nil || err.Error() != context.Canceled.Error() {
		t.Fatalf("wrapped context error was not canonicalized: %v", err)
	}
	inspection := fmt.Sprintf("%+v", mustInspect(t, manager))
	if strings.Contains(inspection, "private-provider-context-credential") {
		t.Fatalf("wrapped context secret escaped inspection: %s", inspection)
	}
}

func TestBuiltinPresetValidationReportsEveryRequirementWithoutFallback(t *testing.T) {
	manager, err := NewManager(tools.NewToolset(nil), NewWeb(), NewFinance())
	if err != nil {
		t.Fatal(err)
	}

	presets := mustInspectPresets(t, manager)
	if len(presets) != 1 || presets[0].ID != StandardPresetID ||
		presets[0].Version != StandardPresetVersion || !presets[0].Immutable || presets[0].Valid {
		t.Fatalf("built-in presets = %+v", presets)
	}
	if len(presets[0].Errors) != len(BuiltinStandardPreset().RequiredCapabilities) {
		t.Fatalf("validation errors = %v, want one for every requirement", presets[0].Errors)
	}
	for _, required := range BuiltinStandardPreset().RequiredCapabilities {
		found := false
		for _, diagnostic := range presets[0].Errors {
			if strings.Contains(diagnostic, string(required.ID)) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing diagnostic for %q in %v", required.ID, presets[0].Errors)
		}
	}
	if report := mustValidatePreset(t, manager, "missing"); report.Valid ||
		len(report.Errors) != 1 || !strings.Contains(report.Errors[0], `Agent Preset "missing" is not allowed`) {
		t.Fatalf("unknown preset validation = %+v", report)
	}
	if _, err := manager.ResolvePreset("missing"); err == nil || strings.Contains(err.Error(), "standard is valid") {
		t.Fatalf("unknown preset silently fell back: %v", err)
	}
}

func TestLifecycleManagementReturnsTransitionAndAffectedDependents(t *testing.T) {
	provider := fakeToolPlugin("provider", "provider.echo", "provider_echo", "ok")
	dependent := fakeToolPlugin("dependent", "dependent.echo", "dependent_echo", "ok")
	dependent.manifest.RequiredDependencies = []Dependency{{
		ID: "provider", Compatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
	}}
	manager, err := NewManager(tools.NewToolset(nil), dependent, provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{"provider", "dependent"} {
		if _, err := manager.SetEnabledWithTransition(context.Background(), id, true); err != nil {
			t.Fatal(err)
		}
	}

	transition, err := manager.SetEnabledWithTransition(context.Background(), "provider", false)
	if err != nil {
		t.Fatal(err)
	}
	if transition.PluginID != "provider" || transition.From != StateReady || transition.To != StateDisabled || transition.Enabled {
		t.Fatalf("transition = %+v", transition)
	}
	if len(transition.AffectedDependents) != 1 || transition.AffectedDependents[0].ID != "dependent" ||
		transition.AffectedDependents[0].State != StateWaiting {
		t.Fatalf("affected dependents = %+v", transition.AffectedDependents)
	}
}

func TestDisableBlocksNewDependentSessionsButPreservesExistingComposition(t *testing.T) {
	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	existing, err := manager.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	originalReceipt := composition.Clone(existing.Receipt)
	if _, err := manager.SetEnabledWithTransition(context.Background(), WebPluginID, false); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.ResolvePreset(StandardPresetID); err == nil ||
		!strings.Contains(err.Error(), string(WebFetchCapabilityID)) ||
		!strings.Contains(err.Error(), string(WebSearchCapabilityID)) {
		t.Fatalf("new standard session error = %v, want every disabled Web requirement", err)
	}
	assertToolErrorContaining(t, existing.Toolset, "web_search", `{"query":""}`, "query must not be empty")
	if !reflect.DeepEqual(existing.Receipt, originalReceipt) {
		t.Fatalf("existing Composition Receipt changed after disable\n got: %#v\nwant: %#v", existing.Receipt, originalReceipt)
	}
}

func TestDurableDesiredStateSeedsDefaultsAndPrecedesLifecycleFailure(t *testing.T) {
	secret := "sk-live-must-never-escape"
	plugin := failingStopManagementPlugin{
		fakePlugin: fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"),
		stopErr:    errors.New(secret),
	}
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}
	store := &enabledStateMemoryStore{values: map[string]bool{}}
	if err := manager.ConfigureEnabledState(context.Background(), store, map[PluginID]bool{"fixture": true}); err != nil {
		t.Fatal(err)
	}
	if status, _ := manager.Status("fixture"); status.State != StateReady || !store.values["fixture"] {
		t.Fatalf("configured status=%+v durable=%v", status, store.values)
	}
	if _, err := manager.SetEnabledWithTransition(context.Background(), "fixture", false); err == nil {
		t.Fatal("disable unexpectedly succeeded despite cleanup failure")
	}
	if store.values["fixture"] {
		t.Fatal("failed cleanup reverted durable disable intent")
	}
	status, _ := manager.Status("fixture")
	if encoded := fmt.Sprintf("%+v %v", status, err); strings.Contains(encoded, secret) {
		t.Fatalf("raw plugin error escaped management state: %s", encoded)
	}
}

func TestConfigureEnabledStateRejectsUnknownDefaultWithoutWriting(t *testing.T) {
	manager, err := NewManager(tools.NewToolset(nil), fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"))
	if err != nil {
		t.Fatal(err)
	}
	store := &enabledStateMemoryStore{values: map[string]bool{}}
	if err := manager.ConfigureEnabledState(context.Background(), store, map[PluginID]bool{"removed": true}); err == nil {
		t.Fatal("unknown compiled default was accepted")
	}
	if len(store.values) != 0 {
		t.Fatalf("unknown default wrote configuration: %v", store.values)
	}
}

func TestConfigurationSensitiveBoundariesFailClosedOnRefreshError(t *testing.T) {
	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	store := &enabledStateMemoryStore{values: map[string]bool{}}
	defaults := map[PluginID]bool{WebPluginID: true, FinancePluginID: true}
	if err := manager.ConfigureEnabledState(context.Background(), store, defaults); err != nil {
		t.Fatal(err)
	}
	store.err = errors.New("sqlite-password-never-expose")
	if _, err := manager.InspectContext(context.Background()); err == nil ||
		strings.Contains(err.Error(), "sqlite-password-never-expose") {
		t.Fatalf("inspection refresh error=%v", err)
	}
	if _, err := manager.ResolvePreset(StandardPresetID); err == nil ||
		strings.Contains(err.Error(), "sqlite-password-never-expose") {
		t.Fatalf("new session did not fail closed safely: %v", err)
	}
}

func TestObservationalRefreshDoesNotRetryFailedPluginAndExplicitEnableRetriesOnce(t *testing.T) {
	starts := 0
	plugin := countedFailingStartPlugin{
		fakePlugin: fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"), starts: &starts,
	}
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}
	store := &enabledStateMemoryStore{values: map[string]bool{}}
	if err := manager.ConfigureEnabledState(context.Background(), store, map[PluginID]bool{"fixture": false}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetEnabledWithTransition(context.Background(), "fixture", true); err != nil {
		t.Fatal(err)
	}
	if starts != 1 {
		t.Fatalf("initial explicit enable starts=%d, want 1", starts)
	}
	_, _ = manager.Inspect()
	_, _ = manager.Status("fixture")
	_, _ = manager.InspectPresets()
	_, _ = manager.ValidatePreset(StandardPresetID)
	_, _ = manager.ResolvePreset(StandardPresetID)
	_, _ = manager.NewSessionToolset()
	_, _ = manager.ResumeComposition(CompositionReceipt{})
	if starts != 1 {
		t.Fatalf("observational boundaries retried failed plugin %d times", starts)
	}
	if _, err := manager.SetEnabledWithTransition(context.Background(), "fixture", true); err != nil {
		t.Fatal(err)
	}
	if starts != 2 {
		t.Fatalf("explicit retry starts=%d, want exactly 2", starts)
	}

	freshStarts := 0
	fresh, err := NewManager(tools.NewToolset(nil), countedFailingStartPlugin{
		fakePlugin: fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"), starts: &freshStarts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.ConfigureEnabledState(context.Background(), store, map[PluginID]bool{"fixture": false}); err != nil {
		t.Fatal(err)
	}
	if freshStarts != 1 {
		t.Fatalf("fresh Manager durable enable retry starts=%d, want 1", freshStarts)
	}
}

func TestSameValueEnableRevisionRetriesEachFailedManagerExactlyOnce(t *testing.T) {
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
	startsA, startsB := 0, 0
	managerA, err := NewManager(tools.NewToolset(nil), countedFailingStartPlugin{
		fakePlugin: fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"), starts: &startsA,
	})
	if err != nil {
		t.Fatal(err)
	}
	managerB, err := NewManager(tools.NewToolset(nil), countedFailingStartPlugin{
		fakePlugin: fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"), starts: &startsB,
	})
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[PluginID]bool{"fixture": true}
	if err := managerA.ConfigureEnabledState(context.Background(), storeA, defaults); err != nil {
		t.Fatal(err)
	}
	if err := managerB.ConfigureEnabledState(context.Background(), storeB, defaults); err != nil {
		t.Fatal(err)
	}
	if startsA != 1 || startsB != 1 {
		t.Fatalf("initial durable intent starts A=%d B=%d, want 1 each", startsA, startsB)
	}

	if _, err := managerB.SetEnabledWithTransition(context.Background(), "fixture", true); err != nil {
		t.Fatal(err)
	}
	if startsB != 2 {
		t.Fatalf("explicit same-value retry starts B=%d, want 2", startsB)
	}
	if _, err := managerA.ResumeComposition(CompositionReceipt{}); err == nil {
		t.Fatal("invalid receipt unexpectedly resumed")
	}
	if startsA != 2 {
		t.Fatalf("other Manager did not consume retry revision exactly once: starts A=%d", startsA)
	}
	_, _ = managerA.Inspect()
	_, _ = managerA.ValidatePreset(StandardPresetID)
	_, _ = managerA.NewSessionToolset()
	if startsA != 2 {
		t.Fatalf("same revision caused observational retries: starts A=%d", startsA)
	}

	dbReady, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbReady.Close()
	readyStarts := 0
	readyManager, err := NewManager(tools.NewToolset(nil), countedStartPlugin{
		fakePlugin: fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"), starts: &readyStarts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := readyManager.ConfigureEnabledState(context.Background(), eviedb.NewStore(dbReady), defaults); err != nil {
		t.Fatal(err)
	}
	if readyStarts != 1 {
		t.Fatalf("fresh ready Manager starts=%d, want 1", readyStarts)
	}
	if _, err := managerB.SetEnabledWithTransition(context.Background(), "fixture", true); err != nil {
		t.Fatal(err)
	}
	if _, err := readyManager.Inspect(); err != nil {
		t.Fatal(err)
	}
	if readyStarts != 1 {
		t.Fatalf("ready Manager restarted for same-value revision: starts=%d", readyStarts)
	}
	enabled, revision, found, err := storeA.PluginEnabled(context.Background(), "fixture")
	if err != nil || !found || !enabled || revision != 3 {
		t.Fatalf("durable intent enabled=%v revision=%d found=%v err=%v, want true@3", enabled, revision, found, err)
	}
}

func TestNewEnableRevisionRetriesCrossManagerCleanupPendingExactlyOnce(t *testing.T) {
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
	writer, err := NewManager(tools.NewToolset(nil),
		fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"),
		fakeToolPlugin("unrelated", "unrelated.echo", "unrelated_echo", "ok"),
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &scriptedCleanupPlugin{
		fakePlugin:      fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"),
		failingStarts:   1,
		failingCleanups: 2,
	}
	observer, err := NewManager(tools.NewToolset(nil), fixture,
		fakeToolPlugin("unrelated", "unrelated.echo", "unrelated_echo", "ok"))
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[PluginID]bool{"fixture": true, "unrelated": true}
	if err := writer.ConfigureEnabledState(context.Background(), storeA, defaults); err != nil {
		t.Fatal(err)
	}
	if err := observer.ConfigureEnabledState(context.Background(), storeB, defaults); err != nil {
		t.Fatal(err)
	}
	if starts, stops := fixture.counts(); starts != 1 || stops != 1 {
		t.Fatalf("initial failed start/rollback counts=(%d,%d), want (1,1)", starts, stops)
	}
	if toolset, err := observer.NewSessionToolset(); err != nil || countSchema(toolset, "fixture_echo") != 0 {
		t.Fatalf("cleanup-pending plugin exposed capability: schemas=%v err=%v", schemaNames(toolset), err)
	}

	for _, enabled := range []bool{false, true, true} {
		if _, err := writer.SetEnabledWithTransition(context.Background(), "unrelated", enabled); err != nil {
			t.Fatal(err)
		}
		if _, err := observer.InspectContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		status, err := observer.Status("unrelated")
		if err != nil {
			t.Fatal(err)
		}
		wantState := StateDisabled
		if enabled {
			wantState = StateReady
		}
		if status.Enabled != enabled || status.State != wantState {
			t.Fatalf("unrelated enabled=%v status=%+v, want state %s", enabled, status, wantState)
		}
		if starts, stops := fixture.counts(); starts != 1 || stops != 1 {
			t.Fatalf("unrelated enabled=%v retried fixture cleanup: counts=(%d,%d)", enabled, starts, stops)
		}
	}
	if err := observer.Stop(context.Background(), "unrelated"); err != nil {
		t.Fatal(err)
	}
	if status, err := observer.Status("unrelated"); err != nil || status.State != StateStopped {
		t.Fatalf("direct unrelated Stop status=%+v err=%v", status, err)
	}
	if err := observer.Start(context.Background(), "unrelated"); err != nil {
		t.Fatal(err)
	}
	if status, err := observer.Status("unrelated"); err != nil || status.State != StateReady {
		t.Fatalf("direct unrelated Start status=%+v err=%v", status, err)
	}
	if starts, stops := fixture.counts(); starts != 1 || stops != 1 {
		t.Fatalf("direct unrelated lifecycle retried fixture cleanup: counts=(%d,%d)", starts, stops)
	}

	if _, err := writer.SetEnabledWithTransition(context.Background(), "fixture", true); err != nil {
		t.Fatal(err)
	}
	if _, err := observer.InspectContext(context.Background()); err == nil || strings.Contains(err.Error(), "credential") {
		t.Fatalf("first cleanup retry error=%v, want safe failure", err)
	}
	if starts, stops := fixture.counts(); starts != 1 || stops != 2 {
		t.Fatalf("first revision counts=(%d,%d), want (1,2)", starts, stops)
	}
	enabled, revision, found, err := storeB.PluginEnabled(context.Background(), "fixture")
	if err != nil || !found || !enabled || revision != 2 {
		t.Fatalf("first durable state enabled=%v revision=%d found=%v err=%v, want true@2", enabled, revision, found, err)
	}
	inspection, err := observer.InspectContext(context.Background())
	if err != nil || inspection.Plugins[0].State != StateFailed || !inspection.Plugins[0].Enabled {
		t.Fatalf("same-revision inspection=%+v err=%v", inspection, err)
	}
	if toolset, err := observer.NewSessionToolset(); err != nil || countSchema(toolset, "fixture_echo") != 0 {
		t.Fatalf("same-revision toolset schemas=%v err=%v", schemaNames(toolset), err)
	}
	if starts, stops := fixture.counts(); starts != 1 || stops != 2 {
		t.Fatalf("observations retried consumed revision: counts=(%d,%d)", starts, stops)
	}

	if _, err := writer.SetEnabledWithTransition(context.Background(), "fixture", true); err != nil {
		t.Fatal(err)
	}
	inspection, err = observer.InspectContext(context.Background())
	if err != nil || inspection.Plugins[0].State != StateReady || !inspection.Plugins[0].Enabled {
		t.Fatalf("successful cleanup/restart inspection=%+v err=%v", inspection, err)
	}
	if starts, stops := fixture.counts(); starts != 2 || stops != 3 {
		t.Fatalf("second revision counts=(%d,%d), want (2,3)", starts, stops)
	}
	toolset, err := observer.NewSessionToolset()
	if err != nil || countSchema(toolset, "fixture_echo") != 1 || countSchema(toolset, "unrelated_echo") != 1 {
		t.Fatalf("recovered toolset schemas=%v err=%v", schemaNames(toolset), err)
	}
	enabled, revision, found, err = storeB.PluginEnabled(context.Background(), "fixture")
	if err != nil || !found || !enabled || revision != 3 {
		t.Fatalf("durable state enabled=%v revision=%d found=%v err=%v, want true@3", enabled, revision, found, err)
	}
}

func TestTargetRevisionAuthorizesCleanupPendingRequiredDependentClosure(t *testing.T) {
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
	provider := fakeToolPlugin("provider", "provider.echo", "provider_echo", "ok")
	dependentBase := fakeToolPlugin("dependent", "dependent.echo", "dependent_echo", "ok")
	dependentBase.manifest.RequiredDependencies = []Dependency{{
		ID: "provider", Compatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
	}}
	writer, err := NewManager(tools.NewToolset(nil), provider, dependentBase)
	if err != nil {
		t.Fatal(err)
	}
	dependent := &scriptedCleanupPlugin{
		fakePlugin:      dependentBase,
		failingStarts:   1,
		failingCleanups: 1,
	}
	observer, err := NewManager(tools.NewToolset(nil), provider, dependent)
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[PluginID]bool{"provider": true, "dependent": true}
	if err := writer.ConfigureEnabledState(context.Background(), storeA, defaults); err != nil {
		t.Fatal(err)
	}
	if err := observer.ConfigureEnabledState(context.Background(), storeB, defaults); err != nil {
		t.Fatal(err)
	}
	if starts, stops := dependent.counts(); starts != 1 || stops != 1 {
		t.Fatalf("initial dependent failed start/rollback counts=(%d,%d), want (1,1)", starts, stops)
	}

	if _, err := writer.SetEnabledWithTransition(context.Background(), "provider", false); err != nil {
		t.Fatal(err)
	}
	inspection, err := observer.InspectContext(context.Background())
	if err != nil || strings.Contains(fmt.Sprintf("%+v %v", inspection, err), "credential") {
		t.Fatalf("provider disable inspection=%+v err=%v", inspection, err)
	}
	if starts, stops := dependent.counts(); starts != 1 || stops != 2 {
		t.Fatalf("dependent closure cleanup counts=(%d,%d), want (1,2)", starts, stops)
	}
	dependentStatus, err := observer.Status("dependent")
	if err != nil || dependentStatus.State != StateWaiting || !dependentStatus.Enabled {
		t.Fatalf("dependent status=%+v err=%v", dependentStatus, err)
	}
	providerStatus, err := observer.Status("provider")
	if err != nil || providerStatus.State != StateDisabled || providerStatus.Enabled {
		t.Fatalf("provider status=%+v err=%v", providerStatus, err)
	}
	toolset, err := observer.NewSessionToolset()
	if err != nil || countSchema(toolset, "dependent_echo") != 0 || countSchema(toolset, "provider_echo") != 0 {
		t.Fatalf("disabled dependency closure schemas=%v err=%v", schemaNames(toolset), err)
	}
	enabled, revision, found, err := storeB.PluginEnabled(context.Background(), "provider")
	if err != nil || !found || enabled || revision != 2 {
		t.Fatalf("provider durable state enabled=%v revision=%d found=%v err=%v, want false@2", enabled, revision, found, err)
	}
}

func TestNewDisableRevisionRetriesCrossManagerCleanupPendingExactlyOnce(t *testing.T) {
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
	writer, err := NewManager(tools.NewToolset(nil), fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"))
	if err != nil {
		t.Fatal(err)
	}
	fixture := &scriptedCleanupPlugin{
		fakePlugin:      fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"),
		failingCleanups: 1,
	}
	observer, err := NewManager(tools.NewToolset(nil), fixture)
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[PluginID]bool{"fixture": true}
	if err := writer.ConfigureEnabledState(context.Background(), storeA, defaults); err != nil {
		t.Fatal(err)
	}
	if err := observer.ConfigureEnabledState(context.Background(), storeB, defaults); err != nil {
		t.Fatal(err)
	}
	toolset, err := observer.NewSessionToolset()
	if err != nil || !reflect.DeepEqual(schemaNames(toolset), []string{"fixture_echo"}) {
		t.Fatalf("initial toolset schemas=%v err=%v", schemaNames(toolset), err)
	}

	if _, err := writer.SetEnabledWithTransition(context.Background(), "fixture", false); err != nil {
		t.Fatal(err)
	}
	if _, err := observer.InspectContext(context.Background()); err == nil || strings.Contains(err.Error(), "credential") {
		t.Fatalf("first disable cleanup error=%v, want safe failure", err)
	}
	if starts, stops := fixture.counts(); starts != 1 || stops != 1 {
		t.Fatalf("first disable revision counts=(%d,%d), want (1,1)", starts, stops)
	}
	enabled, revision, found, err := storeB.PluginEnabled(context.Background(), "fixture")
	if err != nil || !found || enabled || revision != 2 {
		t.Fatalf("first durable state enabled=%v revision=%d found=%v err=%v, want false@2", enabled, revision, found, err)
	}
	inspection, err := observer.InspectContext(context.Background())
	if err != nil || inspection.Plugins[0].State != StateFailed || inspection.Plugins[0].Enabled {
		t.Fatalf("same-revision disabled inspection=%+v err=%v", inspection, err)
	}
	toolset, err = observer.NewSessionToolset()
	if err != nil || len(toolset.Schemas()) != 0 {
		t.Fatalf("disabled cleanup-pending schemas=%v err=%v", toolset.Schemas(), err)
	}
	if starts, stops := fixture.counts(); starts != 1 || stops != 1 {
		t.Fatalf("observations retried consumed disable: counts=(%d,%d)", starts, stops)
	}

	if _, err := writer.SetEnabledWithTransition(context.Background(), "fixture", false); err != nil {
		t.Fatal(err)
	}
	inspection, err = observer.InspectContext(context.Background())
	if err != nil || inspection.Plugins[0].State != StateDisabled || inspection.Plugins[0].Enabled {
		t.Fatalf("successful disable cleanup inspection=%+v err=%v", inspection, err)
	}
	if starts, stops := fixture.counts(); starts != 1 || stops != 2 {
		t.Fatalf("second disable revision counts=(%d,%d), want (1,2)", starts, stops)
	}
	toolset, err = observer.NewSessionToolset()
	if err != nil || len(toolset.Schemas()) != 0 {
		t.Fatalf("cleanly disabled toolset schemas=%v err=%v", toolset.Schemas(), err)
	}
	enabled, revision, found, err = storeA.PluginEnabled(context.Background(), "fixture")
	if err != nil || !found || enabled || revision != 3 {
		t.Fatalf("durable state enabled=%v revision=%d found=%v err=%v, want false@3", enabled, revision, found, err)
	}
}

func TestAffectedDependentsUsesExactTransitiveRequiredAndLoadedOptionalClosure(t *testing.T) {
	provider := fakeToolPlugin("provider", "provider.echo", "provider_echo", "ok")
	middleBase := fakeToolPlugin("middle", "middle.echo", "middle_echo", "ok")
	middleBase.manifest.RequiredDependencies = []Dependency{{ID: "provider", Compatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}}}
	middleCalls := 0
	middle := flappingConnectionPlugin{fakePlugin: middleBase, calls: &middleCalls}
	top := fakeToolPlugin("top", "top.echo", "top_echo", "ok")
	top.manifest.RequiredDependencies = []Dependency{{ID: "middle", Compatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}}}
	optional := fakeToolPlugin("optional", "optional.echo", "optional_echo", "ok")
	optional.manifest.OptionalDependencies = []Dependency{{ID: "provider", Compatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}}}
	calls := 0
	unrelated := flappingConnectionPlugin{fakePlugin: fakeToolPlugin("unrelated", "unrelated.echo", "unrelated_echo", "ok"), calls: &calls}
	manager, err := NewManager(tools.NewToolset(nil), unrelated, top, optional, middle, provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{"provider", "middle", "top", "optional", "unrelated"} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	transition, err := manager.SetEnabledWithTransition(context.Background(), "provider", false)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]PluginID, len(transition.AffectedDependents))
	for i, status := range transition.AffectedDependents {
		got[i] = status.ID
	}
	if want := []PluginID{"middle", "optional", "top"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("affected dependents=%v, want exact dependency order %v", got, want)
	}
	if _, err := manager.SetEnabledWithTransition(context.Background(), "provider", true); err != nil {
		t.Fatal(err)
	}
	transition, err = manager.SetEnabledWithTransition(context.Background(), "provider", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(transition.AffectedDependents) != 0 {
		t.Fatalf("readiness-only changes reported as affected dependents: %+v", transition.AffectedDependents)
	}
}

func TestOptionalProviderEnableUsesBeforeAfterDependencyEdgeUnion(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	provider := newLifecycleFake("provider", &events, &eventMu)
	optional := newLifecycleFake("optional", &events, &eventMu).withOptional("provider")
	top := newLifecycleFake("top", &events, &eventMu).withRequired("optional")
	calls := 0
	unrelated := flappingConnectionPlugin{fakePlugin: fakeToolPlugin("unrelated", "unrelated.echo", "unrelated_echo", "ok"), calls: &calls}
	manager, err := NewManager(tools.NewToolset(nil), unrelated, top, optional, provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{"optional", "top", "unrelated"} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	events = nil
	transition, err := manager.SetEnabledWithTransition(context.Background(), "provider", true)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]PluginID, len(transition.AffectedDependents))
	for i, status := range transition.AffectedDependents {
		got[i] = status.ID
	}
	if want := []PluginID{"optional", "top"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("optional enable affected=%v, want exact before/after edge union %v", got, want)
	}
	if want := []string{"start:provider", "stop:top", "stop:optional", "start:optional", "start:top"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("optional enable lifecycle events=%v, want %v", events, want)
	}
}

func TestDurableEnableFailureRetriesOnFreshManager(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	failing := failingStartManagementPlugin{fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok")}
	manager, err := NewManager(tools.NewToolset(nil), failing)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ConfigureEnabledState(context.Background(), store, map[PluginID]bool{"fixture": true}); err != nil {
		t.Fatal(err)
	}
	status, _ := manager.Status("fixture")
	if !status.Enabled || status.State != StateFailed || strings.Contains(status.Diagnostic, "credential") {
		t.Fatalf("failed requested enable status=%+v", status)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	restarted, err := NewManager(tools.NewToolset(nil), fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"))
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.ConfigureEnabledState(context.Background(), eviedb.NewStore(db), map[PluginID]bool{"fixture": false}); err != nil {
		t.Fatal(err)
	}
	status, _ = restarted.Status("fixture")
	if !status.Enabled || status.State != StateReady {
		t.Fatalf("fresh manager did not retry durable enable: %+v", status)
	}
}

func TestAttachedManagersConvergeOnLatestDurableSQLiteConfiguration(t *testing.T) {
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
	managerA, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube())
	if err != nil {
		t.Fatal(err)
	}
	managerB, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube())
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[PluginID]bool{WebPluginID: true, FinancePluginID: true, YouTubePluginID: true}
	for _, configured := range []struct {
		manager *Manager
		store   EnabledStateStore
	}{{managerA, storeA}, {managerB, storeB}} {
		if err := configured.manager.ConfigureEnabledState(context.Background(), configured.store, defaults); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := managerA.SetEnabledWithTransition(context.Background(), WebPluginID, false); err != nil {
		t.Fatal(err)
	}
	inspection, err := managerB.InspectContext(context.Background())
	if err != nil || inspection.Plugins[1].ID != WebPluginID || inspection.Plugins[1].Enabled {
		t.Fatalf("running Manager did not observe durable CLI disable: %+v err=%v", inspection, err)
	}
	if _, err := managerB.ResolvePreset(StandardPresetID); err == nil {
		t.Fatal("new session composition used stale enabled state")
	}

	if _, err := managerB.SetEnabledWithTransition(context.Background(), WebPluginID, true); err != nil {
		t.Fatal(err)
	}
	inspection, err = managerA.InspectContext(context.Background())
	if err != nil || !inspection.Plugins[1].Enabled {
		t.Fatalf("CLI Manager did not observe durable web enable: %+v err=%v", inspection, err)
	}

	var writers sync.WaitGroup
	writerErrors := make(chan error, 2)
	writers.Add(2)
	go func() {
		defer writers.Done()
		_, err := managerA.SetEnabledWithTransition(context.Background(), WebPluginID, false)
		writerErrors <- err
	}()
	go func() {
		defer writers.Done()
		_, err := managerB.SetEnabledWithTransition(context.Background(), WebPluginID, true)
		writerErrors <- err
	}()
	writers.Wait()
	close(writerErrors)
	for err := range writerErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	durable, _, found, err := storeA.PluginEnabled(context.Background(), string(WebPluginID))
	if err != nil || !found {
		t.Fatalf("read final durable state found=%v err=%v", found, err)
	}
	for name, manager := range map[string]*Manager{"A": managerA, "B": managerB} {
		inspection, err := manager.InspectContext(context.Background())
		if err != nil || inspection.Plugins[1].Enabled != durable {
			t.Fatalf("Manager %s enabled=%v durable=%v err=%v", name, inspection.Plugins[1].Enabled, durable, err)
		}
	}
}

func TestAttachedManagersApplyDurableDisableDespiteCleanupFailure(t *testing.T) {
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
	managerA, err := NewManager(tools.NewToolset(nil), failingStopManagementPlugin{
		fakePlugin: fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"),
		stopErr:    errors.New("cleanup-credential-never-expose"),
	})
	if err != nil {
		t.Fatal(err)
	}
	managerB, err := NewManager(tools.NewToolset(nil), fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"))
	if err != nil {
		t.Fatal(err)
	}
	if err := managerA.ConfigureEnabledState(context.Background(), eviedb.NewStore(dbA), map[PluginID]bool{"fixture": true}); err != nil {
		t.Fatal(err)
	}
	if err := managerB.ConfigureEnabledState(context.Background(), eviedb.NewStore(dbB), map[PluginID]bool{"fixture": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := managerA.SetEnabledWithTransition(context.Background(), "fixture", false); err == nil ||
		strings.Contains(err.Error(), "cleanup-credential") {
		t.Fatalf("cleanup failure=%v", err)
	}
	inspection, err := managerB.InspectContext(context.Background())
	if err != nil || inspection.Plugins[0].Enabled || inspection.Plugins[0].State != StateDisabled {
		t.Fatalf("other Manager did not apply durable disable: %+v err=%v", inspection, err)
	}
}

func TestAttachedManagerRetriesAnotherProcessDurableFailedEnable(t *testing.T) {
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
	managerA, err := NewManager(tools.NewToolset(nil), failingStartManagementPlugin{fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok")})
	if err != nil {
		t.Fatal(err)
	}
	managerB, err := NewManager(tools.NewToolset(nil), fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"))
	if err != nil {
		t.Fatal(err)
	}
	if err := managerA.ConfigureEnabledState(context.Background(), eviedb.NewStore(dbA), map[PluginID]bool{"fixture": false}); err != nil {
		t.Fatal(err)
	}
	if err := managerB.ConfigureEnabledState(context.Background(), eviedb.NewStore(dbB), map[PluginID]bool{"fixture": false}); err != nil {
		t.Fatal(err)
	}
	if _, err := managerA.SetEnabledWithTransition(context.Background(), "fixture", true); err != nil {
		t.Fatal(err)
	}
	inspection, err := managerB.InspectContext(context.Background())
	if err != nil || !inspection.Plugins[0].Enabled || inspection.Plugins[0].State != StateReady {
		t.Fatalf("other Manager did not retry durable enable: %+v err=%v", inspection, err)
	}
}

func TestResumeCompositionRefreshesExternalDurableConfiguration(t *testing.T) {
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
	storeA, storeB := eviedb.NewStore(dbA), eviedb.NewStore(dbB)
	managerA, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube())
	if err != nil {
		t.Fatal(err)
	}
	managerB, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube())
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[PluginID]bool{WebPluginID: true, FinancePluginID: true, YouTubePluginID: true}
	if err := managerA.ConfigureEnabledState(context.Background(), storeA, defaults); err != nil {
		t.Fatal(err)
	}
	if err := managerB.ConfigureEnabledState(context.Background(), storeB, defaults); err != nil {
		t.Fatal(err)
	}
	pinned, err := managerB.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storeA.SetPluginEnabled(context.Background(), string(WebPluginID), false); err != nil {
		t.Fatal(err)
	}
	if resumed, err := managerB.ResumeComposition(pinned.Receipt); err == nil || len(resumed.Toolset.Schemas()) != 0 {
		t.Fatalf("stale resume exposed tools after external disable: schemas=%v err=%v", resumed.Toolset.Schemas(), err)
	}
	if _, err := storeA.SetPluginEnabled(context.Background(), string(WebPluginID), true); err != nil {
		t.Fatal(err)
	}
	resumed, err := managerB.ResumeComposition(pinned.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	assertToolErrorContaining(t, resumed.Toolset, "web_search", `{"query":""}`, "query must not be empty")
	if err := dbB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := managerB.ResumeComposition(pinned.Receipt); err == nil ||
		!errors.Is(err, ErrEnabledStateUnavailable) || strings.Contains(strings.ToLower(err.Error()), "sqlite") {
		t.Fatalf("resume configuration read failure was not safe: %v", err)
	}
}
