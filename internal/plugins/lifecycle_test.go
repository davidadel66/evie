package plugins

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/tools"
)

type lifecycleFake struct {
	plugin  fakePlugin
	events  *[]string
	eventMu *sync.Mutex
	start   func(context.Context) error
	stop    func(context.Context) error
}

func newLifecycleFake(id string, events *[]string, eventMu *sync.Mutex) *lifecycleFake {
	return &lifecycleFake{
		plugin:  fakeToolPlugin(id, id+".echo", id+"_echo", id+" result"),
		events:  events,
		eventMu: eventMu,
	}
}

func (p *lifecycleFake) Manifest() Manifest { return p.plugin.Manifest() }

func (p *lifecycleFake) ToolCapabilities() []ToolCapability {
	return p.plugin.ToolCapabilities()
}

func (p *lifecycleFake) Start(ctx context.Context) error {
	p.record("start:" + string(p.Manifest().ID))
	if p.start != nil {
		return p.start(ctx)
	}
	return nil
}

func (p *lifecycleFake) Stop(ctx context.Context) error {
	p.record("stop:" + string(p.Manifest().ID))
	if p.stop != nil {
		return p.stop(ctx)
	}
	return nil
}

func (p *lifecycleFake) record(event string) {
	p.eventMu.Lock()
	defer p.eventMu.Unlock()
	*p.events = append(*p.events, event)
}

func (p *lifecycleFake) withRequired(ids ...PluginID) *lifecycleFake {
	p.plugin.manifest.RequiredDependencies = make([]Dependency, len(ids))
	for i, id := range ids {
		p.plugin.manifest.RequiredDependencies[i] = Dependency{
			ID: id,
			Compatibility: VersionRange{
				Minimum: "1.0.0", MaximumExclusive: "2.0.0",
			},
		}
	}
	return p
}

func (p *lifecycleFake) withOptional(id PluginID) *lifecycleFake {
	p.plugin.manifest.OptionalDependencies = append(p.plugin.manifest.OptionalDependencies, Dependency{
		ID: id,
		Compatibility: VersionRange{
			Minimum: "1.0.0", MaximumExclusive: "2.0.0",
		},
	})
	return p
}

func (p *lifecycleFake) addToolCapability(capabilityID, toolName string, optional ...PluginID) *lifecycleFake {
	addition := fakeToolPlugin(string(p.Manifest().ID), capabilityID, toolName, toolName+" result")
	contract := addition.manifest.Capabilities[0]
	contract.OptionalDependencies = append([]PluginID(nil), optional...)
	p.plugin.manifest.Capabilities = append(p.plugin.manifest.Capabilities, contract)
	p.plugin.capabilities = append(p.plugin.capabilities, addition.capabilities[0])
	return p
}

func TestManagerLoadsRequiredDependenciesBeforeDependentsAndStopsThemAfter(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	dependency := newLifecycleFake("dependency", &events, &eventMu)
	dependent := newLifecycleFake("dependent", &events, &eventMu).withRequired("dependency")

	manager, err := NewManager(tools.NewToolset(nil), dependent, dependency)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), "dependent"); err != nil {
		t.Fatal(err)
	}
	assertPluginStatus(t, manager, "dependent", StateWaiting, `required dependency "dependency" is disabled`)
	if len(events) != 0 {
		t.Fatalf("events after blocked enable = %v, want none", events)
	}

	if err := manager.Enable(context.Background(), "dependency"); err != nil {
		t.Fatal(err)
	}
	if got, want := events, []string{"start:dependency", "start:dependent"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("load events = %v, want %v", got, want)
	}
	assertPluginStatus(t, manager, "dependency", StateReady, "")
	assertPluginStatus(t, manager, "dependent", StateReady, "")

	if err := manager.Disable(context.Background(), "dependency"); err != nil {
		t.Fatal(err)
	}
	if got, want := events, []string{
		"start:dependency", "start:dependent", "stop:dependent", "stop:dependency",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lifecycle events = %v, want %v", got, want)
	}
	assertPluginStatus(t, manager, "dependency", StateDisabled, "")
	assertPluginStatus(t, manager, "dependent", StateWaiting, `required dependency "dependency" is disabled`)
}

func TestManagerRejectsDependencyCyclesAndIncompatibleVersionsBeforeStart(t *testing.T) {
	tests := []struct {
		name    string
		build   func(*[]string, *sync.Mutex) []Plugin
		wantErr string
	}{
		{
			name: "required dependency cycle",
			build: func(events *[]string, eventMu *sync.Mutex) []Plugin {
				return []Plugin{
					newLifecycleFake("a", events, eventMu).withRequired("b"),
					newLifecycleFake("b", events, eventMu).withRequired("a"),
				}
			},
			wantErr: "plugin dependency cycle: a -> b -> a",
		},
		{
			name: "incompatible required dependency",
			build: func(events *[]string, eventMu *sync.Mutex) []Plugin {
				dependent := newLifecycleFake("dependent", events, eventMu).withRequired("dependency")
				dependent.plugin.manifest.RequiredDependencies[0].Compatibility = VersionRange{
					Minimum: "2.0.0", MaximumExclusive: "3.0.0",
				}
				return []Plugin{dependent, newLifecycleFake("dependency", events, eventMu)}
			},
			wantErr: `plugin "dependent" requires required dependency "dependency" in range [2.0.0,3.0.0)`,
		},
		{
			name: "optional dependency without explicit range",
			build: func(events *[]string, eventMu *sync.Mutex) []Plugin {
				dependent := newLifecycleFake("dependent", events, eventMu).withOptional("missing")
				dependent.plugin.manifest.OptionalDependencies[0].Compatibility = VersionRange{}
				return []Plugin{dependent}
			},
			wantErr: `plugin "dependent" has invalid optional dependency "missing" compatibility`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			var eventMu sync.Mutex
			manager, err := NewManager(tools.NewToolset(nil), tc.build(&events, &eventMu)...)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("NewManager = (%v, %v), want error containing %q", manager, err, tc.wantErr)
			}
			if len(events) != 0 {
				t.Fatalf("start events = %v, want none", events)
			}
		})
	}
}

func TestManagerKeepsPluginUsableWhenOptionalDependencyIsUnavailable(t *testing.T) {
	tests := []struct {
		name       string
		dependency func(*[]string, *sync.Mutex) *lifecycleFake
		wantWarn   string
		degraded   bool
	}{
		{
			name:     "missing",
			wantWarn: `optional dependency "extension" is not compiled into Evie`,
		},
		{
			name: "failed",
			dependency: func(events *[]string, eventMu *sync.Mutex) *lifecycleFake {
				plugin := newLifecycleFake("extension", events, eventMu)
				plugin.start = func(context.Context) error { return lifecycleError("extension", "start") }
				return plugin
			},
			wantWarn: `optional dependency "extension" failed`,
			degraded: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			var eventMu sync.Mutex
			consumer := newLifecycleFake("consumer", &events, &eventMu).
				withOptional("extension").
				addToolCapability("consumer.extension", "consumer_extension", "extension")
			compiled := []Plugin{consumer}
			if tc.dependency != nil {
				extension := tc.dependency(&events, &eventMu)
				compiled = append(compiled, extension)
			}
			manager, err := NewManager(tools.NewToolset(nil), compiled...)
			if err != nil {
				t.Fatal(err)
			}
			if tc.dependency != nil {
				if err := manager.Enable(context.Background(), "extension"); err != nil {
					t.Fatal(err)
				}
			}
			if err := manager.Enable(context.Background(), "consumer"); err != nil {
				t.Fatal(err)
			}
			status, err := manager.Status("consumer")
			if err != nil {
				t.Fatal(err)
			}
			if status.State != StateReady || len(status.Warnings) != 1 || !strings.Contains(status.Warnings[0], tc.wantWarn) {
				t.Fatalf("consumer status = %+v, want ready with warning containing %q", status, tc.wantWarn)
			}
			toolset, err := manager.NewSessionToolset()
			if err != nil {
				t.Fatal(err)
			}
			assertToolResult(t, toolset, "consumer_echo", "consumer result")
			assertUnknownTool(t, toolset, "consumer_extension")
			if got := manager.Inspect().Degraded; got != tc.degraded {
				t.Fatalf("Inspect().Degraded = %v, want %v", got, tc.degraded)
			}
		})
	}
}

func TestManagerReconcilesOptionalContributionWhenDependencyBecomesReady(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	extension := newLifecycleFake("extension", &events, &eventMu)
	consumer := newLifecycleFake("consumer", &events, &eventMu).
		withOptional("extension").
		addToolCapability("consumer.extension", "consumer_extension", "extension")
	manager, err := NewManager(tools.NewToolset(nil), consumer, extension)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), "consumer"); err != nil {
		t.Fatal(err)
	}
	before, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertUnknownTool(t, before, "consumer_extension")

	if err := manager.Enable(context.Background(), "extension"); err != nil {
		t.Fatal(err)
	}
	after, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, after, "consumer_extension", "consumer_extension result")
	status, err := manager.Status("consumer")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StateReady || len(status.Warnings) != 0 {
		t.Fatalf("consumer status after optional dependency recovery = %+v, want ready without warnings", status)
	}
	if got, want := events, []string{
		"start:consumer", "start:extension", "stop:consumer", "start:consumer",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestManagerRepeatedLifecycleOnUnavailableOptionalProviderLeavesConsumerReady(t *testing.T) {
	tests := []struct {
		name           string
		provider       func(*[]string, *sync.Mutex) *lifecycleFake
		operate        func(*Manager) error
		initialState   LifecycleState
		initialDegrade bool
		wantState      LifecycleState
		wantDegrade    bool
		wantWarning    string
	}{
		{
			name: "disabled provider repeated Disable and Stop",
			provider: func(events *[]string, eventMu *sync.Mutex) *lifecycleFake {
				return newLifecycleFake("extension", events, eventMu)
			},
			operate: func(manager *Manager) error {
				for range 25 {
					if err := manager.Disable(context.Background(), "extension"); err != nil {
						return err
					}
					if err := manager.Stop(context.Background(), "extension"); err != nil {
						return err
					}
				}
				return nil
			},
			initialState: StateDisabled,
			wantState:    StateDisabled,
			wantWarning:  `optional dependency "extension" is disabled`,
		},
		{
			name: "failed provider repeated Stop",
			provider: func(events *[]string, eventMu *sync.Mutex) *lifecycleFake {
				provider := newLifecycleFake("extension", events, eventMu)
				provider.start = func(context.Context) error { return lifecycleError("extension", "start") }
				return provider
			},
			operate: func(manager *Manager) error {
				for range 5 {
					if err := manager.Stop(context.Background(), "extension"); err != nil {
						return err
					}
				}
				return nil
			},
			initialState:   StateFailed,
			initialDegrade: true,
			wantState:      StateStopped,
			wantWarning:    `optional dependency "extension" is stopped`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			var eventMu sync.Mutex
			provider := tc.provider(&events, &eventMu)
			consumer := newLifecycleFake("consumer", &events, &eventMu).
				withOptional("extension").
				addToolCapability("consumer.extension", "consumer_extension", "extension")
			manager, err := NewManager(tools.NewToolset(nil), consumer, provider)
			if err != nil {
				t.Fatal(err)
			}
			if tc.initialState == StateFailed {
				if err := manager.Enable(context.Background(), "extension"); err != nil {
					t.Fatal(err)
				}
			}
			if err := manager.Enable(context.Background(), "consumer"); err != nil {
				t.Fatal(err)
			}
			assertPluginStatus(t, manager, "extension", tc.initialState, "")
			if got := manager.Inspect().Degraded; got != tc.initialDegrade {
				t.Fatalf("initial Inspect().Degraded = %v, want %v", got, tc.initialDegrade)
			}
			consumerStartsBefore := countLifecycleEvent(events, "start:consumer")
			consumerStopsBefore := countLifecycleEvent(events, "stop:consumer")

			if err := tc.operate(manager); err != nil {
				t.Fatal(err)
			}
			assertPluginStatus(t, manager, "extension", tc.wantState, "")
			consumerStatus, err := manager.Status("consumer")
			if err != nil {
				t.Fatal(err)
			}
			if consumerStatus.State != StateReady || len(consumerStatus.Warnings) != 1 ||
				!strings.Contains(consumerStatus.Warnings[0], tc.wantWarning) {
				t.Fatalf("consumer status = %+v, want ready with warning containing %q", consumerStatus, tc.wantWarning)
			}
			if got := manager.Inspect().Degraded; got != tc.wantDegrade {
				t.Fatalf("Inspect().Degraded = %v, want %v", got, tc.wantDegrade)
			}
			toolset, err := manager.NewSessionToolset()
			if err != nil {
				t.Fatal(err)
			}
			assertToolResult(t, toolset, "consumer_echo", "consumer result")
			assertUnknownTool(t, toolset, "consumer_extension")
			if got := countLifecycleEvent(events, "start:consumer"); got != consumerStartsBefore {
				t.Fatalf("consumer Start calls = %d after repeated provider lifecycle, want %d", got, consumerStartsBefore)
			}
			if got := countLifecycleEvent(events, "stop:consumer"); got != consumerStopsBefore {
				t.Fatalf("consumer Stop calls = %d after repeated provider lifecycle, want %d", got, consumerStopsBefore)
			}
		})
	}
}

func TestManagerOptionalProviderTransitionStillHidesConsumerImmediately(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	provider := newLifecycleFake("extension", &events, &eventMu)
	consumer := newLifecycleFake("consumer", &events, &eventMu).
		withOptional("extension").
		addToolCapability("consumer.extension", "consumer_extension", "extension")
	stopEntered := make(chan struct{})
	releaseStop := make(chan struct{})
	consumer.stop = func(context.Context) error {
		close(stopEntered)
		<-releaseStop
		return nil
	}
	manager, err := NewManager(tools.NewToolset(nil), consumer, provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{"extension", "consumer"} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	ready, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, ready, "consumer_extension", "consumer_extension result")

	result := make(chan error, 1)
	go func() { result <- manager.Stop(context.Background(), "extension") }()
	<-stopEntered
	assertPluginStatus(t, manager, "consumer", StateStopping, "")
	hidden, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertUnknownTool(t, hidden, "consumer_echo")
	assertUnknownTool(t, hidden, "consumer_extension")
	close(releaseStop)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	consumerStatus, err := manager.Status("consumer")
	if err != nil {
		t.Fatal(err)
	}
	if consumerStatus.State != StateReady || len(consumerStatus.Warnings) != 1 {
		t.Fatalf("consumer status after optional provider stopped = %+v", consumerStatus)
	}
	degraded, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, degraded, "consumer_echo", "consumer result")
	assertUnknownTool(t, degraded, "consumer_extension")
}

func TestManagerOptionalProviderStopFailureRecomposesConsumerBeforeReturningError(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	provider := newLifecycleFake("extension", &events, &eventMu)
	providerStops := 0
	provider.stop = func(context.Context) error {
		providerStops++
		if providerStops == 1 {
			return lifecycleError("extension", "stop")
		}
		return nil
	}
	consumer := newLifecycleFake("consumer", &events, &eventMu).
		withOptional("extension").
		addToolCapability("consumer.extension", "consumer_extension", "extension")
	manager, err := NewManager(tools.NewToolset(nil), consumer, provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{"extension", "consumer"} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}

	err = manager.Stop(context.Background(), "extension")
	if err == nil || !strings.Contains(err.Error(), "extension stop failed") {
		t.Fatalf("Stop error = %v, want extension stop failure", err)
	}
	assertPluginStatus(t, manager, "extension", StateFailed, "cleanup pending: stop failed: extension stop failed")
	consumerStatus, statusErr := manager.Status("consumer")
	if statusErr != nil {
		t.Fatal(statusErr)
	}
	if consumerStatus.State != StateReady || len(consumerStatus.Warnings) != 1 ||
		!strings.Contains(consumerStatus.Warnings[0], `optional dependency "extension" failed: cleanup pending`) {
		t.Fatalf("consumer status after provider Stop failure = %+v, want ready with failed-provider warning", consumerStatus)
	}
	toolset, toolsetErr := manager.NewSessionToolset()
	if toolsetErr != nil {
		t.Fatal(toolsetErr)
	}
	assertToolResult(t, toolset, "consumer_echo", "consumer result")
	assertUnknownTool(t, toolset, "consumer_extension")
	if got, want := events, []string{
		"start:extension", "start:consumer",
		"stop:consumer", "stop:extension", "start:consumer",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("degraded recovery events = %v, want %v", got, want)
	}
	if !manager.Inspect().Degraded {
		t.Fatal("cleanup-pending provider did not keep inspection degraded")
	}

	if err := manager.Start(context.Background(), "extension"); err != nil {
		t.Fatal(err)
	}
	assertPluginStatus(t, manager, "extension", StateReady, "")
	assertPluginStatus(t, manager, "consumer", StateReady, "")
	recovered, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, recovered, "consumer_extension", "consumer_extension result")
	if got, want := events, []string{
		"start:extension", "start:consumer",
		"stop:consumer", "stop:extension", "start:consumer",
		"stop:extension", "start:extension", "stop:consumer", "start:consumer",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanup recovery events = %v, want %v", got, want)
	}
}

func TestManagerRefreshesOptionalWarningWhenWaitingProviderStartFails(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	prerequisite := newLifecycleFake("prerequisite", &events, &eventMu)
	provider := newLifecycleFake("extension", &events, &eventMu).withRequired("prerequisite")
	provider.start = func(context.Context) error { return lifecycleError("extension", "start") }
	provider.stop = func(context.Context) error { return lifecycleError("extension", "cleanup") }
	consumer := newLifecycleFake("consumer", &events, &eventMu).withOptional("extension")
	manager, err := NewManager(tools.NewToolset(nil), consumer, provider, prerequisite)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), "extension"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), "consumer"); err != nil {
		t.Fatal(err)
	}
	waiting, err := manager.Status("consumer")
	if err != nil {
		t.Fatal(err)
	}
	if waiting.State != StateReady || len(waiting.Warnings) != 1 ||
		waiting.Warnings[0] != `optional dependency "extension" is waiting` {
		t.Fatalf("consumer status while provider waits = %+v", waiting)
	}

	err = manager.Enable(context.Background(), "prerequisite")
	if err != nil {
		t.Fatal(err)
	}
	providerStatus, err := manager.Status("extension")
	if err != nil {
		t.Fatal(err)
	}
	if providerStatus.State != StateFailed ||
		!strings.Contains(providerStatus.Diagnostic, "start failed: extension start failed") ||
		!strings.Contains(providerStatus.Diagnostic, "cleanup pending: extension cleanup failed") {
		t.Fatalf("provider status = %+v, want failed start with cleanup pending", providerStatus)
	}
	failed, err := manager.Status("consumer")
	if err != nil {
		t.Fatal(err)
	}
	wantWarning := `optional dependency "extension" failed: ` + providerStatus.Diagnostic
	if failed.State != StateReady || len(failed.Warnings) != 1 || failed.Warnings[0] != wantWarning {
		t.Fatalf("consumer status after provider failure = %+v, want warning %q", failed, wantWarning)
	}
	toolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, toolset, "consumer_echo", "consumer result")
}

func TestManagerCleanupErrorDoesNotPreventOptionalCompositionFromStabilizing(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	unrelated := newLifecycleFake("unrelated", &events, &eventMu)
	unrelated.start = func(context.Context) error { return lifecycleError("unrelated", "start") }
	unrelated.stop = func(context.Context) error { return lifecycleError("unrelated", "cleanup") }
	provider := newLifecycleFake("extension", &events, &eventMu)
	consumer := newLifecycleFake("consumer", &events, &eventMu).
		withOptional("extension").
		addToolCapability("consumer.extension", "consumer_extension", "extension")
	manager, err := NewManager(tools.NewToolset(nil), consumer, unrelated, provider)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), "consumer"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), "unrelated"); err != nil {
		t.Fatal(err)
	}
	assertPluginStatus(t, manager, "unrelated", StateFailed, "cleanup pending")
	eventsBefore := len(events)

	err = manager.Enable(context.Background(), "extension")
	if err == nil || !strings.Contains(err.Error(), "unrelated cleanup failed") {
		t.Fatalf("Enable extension error = %v, want unrelated cleanup failure", err)
	}
	assertPluginStatus(t, manager, "extension", StateReady, "")
	consumerStatus, err := manager.Status("consumer")
	if err != nil {
		t.Fatal(err)
	}
	if consumerStatus.State != StateReady || len(consumerStatus.Warnings) != 0 {
		t.Fatalf("consumer status = %+v, want ready without warnings", consumerStatus)
	}
	toolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, toolset, "consumer_echo", "consumer result")
	assertToolResult(t, toolset, "consumer_extension", "consumer_extension result")
	if got, want := events[eventsBefore:], []string{
		"stop:unrelated", "start:extension", "stop:consumer", "start:consumer",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stabilization events = %v, want %v", got, want)
	}
	if got := countLifecycleEvent(events, "stop:unrelated"); got != 2 {
		t.Fatalf("unrelated cleanup attempts = %d, want rollback plus one retry", got)
	}
	if got := countLifecycleEvent(events, "start:consumer"); got != 2 {
		t.Fatalf("consumer Start calls = %d, want initial plus stabilized composition", got)
	}
	if got := countLifecycleEvent(events, "stop:consumer"); got != 1 {
		t.Fatalf("consumer Stop calls = %d, want one recomposition cleanup", got)
	}
}

func TestManagerOptionalRecompositionStopsTransitiveRequiredDependentsFirst(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	provider := newLifecycleFake("extension", &events, &eventMu)
	consumer := newLifecycleFake("consumer", &events, &eventMu).
		withOptional("extension").
		addToolCapability("consumer.extension", "consumer_extension", "extension")
	dependent := newLifecycleFake("dependent", &events, &eventMu).withRequired("consumer")
	dependentStopEntered := make(chan struct{})
	releaseDependentStop := make(chan struct{})
	dependent.stop = func(context.Context) error {
		close(dependentStopEntered)
		<-releaseDependentStop
		return nil
	}
	manager, err := NewManager(tools.NewToolset(nil), dependent, consumer, provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{"consumer", "dependent"} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	eventsBefore := len(events)

	result := make(chan error, 1)
	go func() { result <- manager.Enable(context.Background(), "extension") }()
	select {
	case <-dependentStopEntered:
	case <-time.After(time.Second):
		t.Fatal("dependent Stop did not begin before consumer recomposition")
	}
	assertPluginStatus(t, manager, "dependent", StateStopping, "")
	assertPluginStatus(t, manager, "consumer", StateStopping, "")
	hidden, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertUnknownTool(t, hidden, "dependent_echo")
	assertUnknownTool(t, hidden, "consumer_echo")
	assertUnknownTool(t, hidden, "consumer_extension")
	assertToolResult(t, hidden, "extension_echo", "extension result")
	close(releaseDependentStop)
	if err := <-result; err != nil {
		t.Fatal(err)
	}

	if got, want := events[eventsBefore:], []string{
		"start:extension", "stop:dependent", "stop:consumer",
		"start:consumer", "start:dependent",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("transitive recomposition events = %v, want %v", got, want)
	}
	assertPluginStatus(t, manager, "extension", StateReady, "")
	assertPluginStatus(t, manager, "consumer", StateReady, "")
	assertPluginStatus(t, manager, "dependent", StateReady, "")
	ready, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, ready, "consumer_extension", "consumer_extension result")
	assertToolResult(t, ready, "dependent_echo", "dependent result")
}

func TestManagerFailedOptionalRecompositionBlocksTransitiveRequiredDependentUntilRecovery(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	provider := newLifecycleFake("extension", &events, &eventMu)
	consumer := newLifecycleFake("consumer", &events, &eventMu).
		withOptional("extension").
		addToolCapability("consumer.extension", "consumer_extension", "extension")
	consumerStops := 0
	consumer.stop = func(context.Context) error {
		consumerStops++
		if consumerStops == 1 {
			return lifecycleError("consumer", "stop")
		}
		return nil
	}
	dependent := newLifecycleFake("dependent", &events, &eventMu).withRequired("consumer")
	manager, err := NewManager(tools.NewToolset(nil), dependent, consumer, provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{"consumer", "dependent"} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	eventsBefore := len(events)

	err = manager.Enable(context.Background(), "extension")
	if err == nil || !strings.Contains(err.Error(), "consumer stop failed") {
		t.Fatalf("Enable extension error = %v, want consumer cleanup failure", err)
	}
	if got, want := events[eventsBefore:], []string{
		"start:extension", "stop:dependent", "stop:consumer",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("failed recomposition events = %v, want %v", got, want)
	}
	assertPluginStatus(t, manager, "consumer", StateFailed, "cleanup pending")
	assertPluginStatus(t, manager, "dependent", StateWaiting, `required dependency "consumer" failed`)
	failed, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, failed, "extension_echo", "extension result")
	assertUnknownTool(t, failed, "consumer_echo")
	assertUnknownTool(t, failed, "consumer_extension")
	assertUnknownTool(t, failed, "dependent_echo")
	if !manager.Inspect().Degraded {
		t.Fatal("failed consumer did not leave inspection degraded")
	}

	recoveryBefore := len(events)
	if err := manager.Start(context.Background(), "consumer"); err != nil {
		t.Fatal(err)
	}
	if got, want := events[recoveryBefore:], []string{
		"stop:consumer", "start:consumer", "start:dependent",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recovery events = %v, want %v", got, want)
	}
	assertPluginStatus(t, manager, "consumer", StateReady, "")
	assertPluginStatus(t, manager, "dependent", StateReady, "")
	recovered, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, recovered, "consumer_extension", "consumer_extension result")
	assertToolResult(t, recovered, "dependent_echo", "dependent result")
}

func countLifecycleEvent(events []string, want string) int {
	count := 0
	for _, event := range events {
		if event == want {
			count++
		}
	}
	return count
}

func TestManagerBlocksPluginWithMissingOrFailedRequiredDependency(t *testing.T) {
	tests := []struct {
		name       string
		dependency func(*[]string, *sync.Mutex) *lifecycleFake
		want       string
	}{
		{
			name: "missing",
			want: `required dependency "dependency" is not compiled into Evie`,
		},
		{
			name: "failed",
			dependency: func(events *[]string, eventMu *sync.Mutex) *lifecycleFake {
				dependency := newLifecycleFake("dependency", events, eventMu)
				dependency.start = func(context.Context) error { return lifecycleError("dependency", "start") }
				return dependency
			},
			want: `required dependency "dependency" failed: start failed`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			var eventMu sync.Mutex
			dependent := newLifecycleFake("dependent", &events, &eventMu).withRequired("dependency")
			compiled := []Plugin{dependent}
			if tc.dependency != nil {
				dependency := tc.dependency(&events, &eventMu)
				compiled = append(compiled, dependency)
			}
			manager, err := NewManager(tools.NewToolset(nil), compiled...)
			if err != nil {
				t.Fatal(err)
			}
			if tc.dependency != nil {
				if err := manager.Enable(context.Background(), "dependency"); err != nil {
					t.Fatal(err)
				}
			}
			if err := manager.Enable(context.Background(), "dependent"); err != nil {
				t.Fatal(err)
			}
			assertPluginStatus(t, manager, "dependent", StateWaiting, tc.want)
			toolset, err := manager.NewSessionToolset()
			if err != nil {
				t.Fatal(err)
			}
			assertUnknownTool(t, toolset, "dependent_echo")
		})
	}
}

func TestManagerRollsBackFailedInitializationAndRestartsCompiledPlugin(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	plugin := newLifecycleFake("fixture", &events, &eventMu)
	attempts := 0
	plugin.start = func(context.Context) error {
		attempts++
		if attempts == 1 {
			return lifecycleError("fixture", "start")
		}
		return nil
	}
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}

	if err := manager.Enable(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
	assertPluginStatus(t, manager, "fixture", StateFailed, "start failed")
	if got, want := events, []string{"start:fixture", "stop:fixture"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("failed initialization events = %v, want %v", got, want)
	}
	failedToolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertUnknownTool(t, failedToolset, "fixture_echo")
	if !manager.Inspect().Degraded {
		t.Fatal("failed enabled plugin did not make inspection degraded")
	}

	if err := manager.Start(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
	assertPluginStatus(t, manager, "fixture", StateReady, "")
	readyToolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, readyToolset, "fixture_echo", "fixture result")

	if err := manager.Stop(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
	assertPluginStatus(t, manager, "fixture", StateStopped, "")
	stoppedToolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertUnknownTool(t, stoppedToolset, "fixture_echo")
	assertToolResult(t, readyToolset, "fixture_echo", "fixture result")

	if err := manager.Start(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
	assertPluginStatus(t, manager, "fixture", StateReady, "")
	if got, want := events, []string{
		"start:fixture", "stop:fixture",
		"start:fixture", "stop:fixture", "start:fixture",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("restart events = %v, want %v", got, want)
	}
}

func TestManagerFailureLeavesKernelToolsetAndInspectionAvailable(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	plugin := newLifecycleFake("optional", &events, &eventMu)
	plugin.start = func(context.Context) error { return lifecycleError("optional", "start") }
	kernelDefinition := fakeToolPlugin("kernel", "kernel.inspect", "kernel_inspect", "kernel available").capabilities[0].Tool
	manager, err := NewManager(tools.NewToolset([]tools.Tool{kernelDefinition}), plugin)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), "optional"); err != nil {
		t.Fatal(err)
	}

	inspection := manager.Inspect()
	if !inspection.Degraded || len(inspection.Plugins) != 1 || inspection.Plugins[0].State != StateFailed {
		t.Fatalf("inspection = %+v, want visible degraded failed plugin", inspection)
	}
	toolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, toolset, "kernel_inspect", "kernel available")
	assertUnknownTool(t, toolset, "optional_echo")
}

func TestManagerCleansUpDependencyChainOnceInReverseOrder(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	base := newLifecycleFake("base", &events, &eventMu)
	middle := newLifecycleFake("middle", &events, &eventMu).withRequired("base")
	top := newLifecycleFake("top", &events, &eventMu).withRequired("middle")
	manager, err := NewManager(tools.NewToolset(nil), top, base, middle)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{"top", "middle", "base"} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.Disable(context.Background(), "base"); err != nil {
		t.Fatal(err)
	}
	if got, want := events, []string{
		"start:base", "start:middle", "start:top",
		"stop:top", "stop:middle", "stop:base",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestManagerSerializesConcurrentLifecycleRequestsAndExposesTransitions(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	plugin := newLifecycleFake("fixture", &events, &eventMu)
	startEntered := make(chan struct{})
	releaseStart := make(chan struct{})
	stopEntered := make(chan struct{})
	releaseStop := make(chan struct{})
	plugin.start = func(context.Context) error {
		close(startEntered)
		<-releaseStart
		return nil
	}
	plugin.stop = func(context.Context) error {
		close(stopEntered)
		<-releaseStop
		return nil
	}
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}

	enableResult := make(chan error, 1)
	go func() { enableResult <- manager.Enable(context.Background(), "fixture") }()
	<-startEntered
	assertPluginStatus(t, manager, "fixture", StateLoading, "")

	stopResult := make(chan error, 1)
	go func() { stopResult <- manager.Stop(context.Background(), "fixture") }()
	close(releaseStart)
	if err := <-enableResult; err != nil {
		t.Fatal(err)
	}
	<-stopEntered
	assertPluginStatus(t, manager, "fixture", StateStopping, "")
	toolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertUnknownTool(t, toolset, "fixture_echo")
	close(releaseStop)
	if err := <-stopResult; err != nil {
		t.Fatal(err)
	}
	assertPluginStatus(t, manager, "fixture", StateStopped, "")
	if got, want := events, []string{"start:fixture", "stop:fixture"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestManagerCancellationRollsBackWithoutPublishingCapabilities(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	plugin := newLifecycleFake("fixture", &events, &eventMu)
	startEntered := make(chan struct{})
	plugin.start = func(ctx context.Context) error {
		close(startEntered)
		<-ctx.Done()
		return ctx.Err()
	}
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- manager.Enable(ctx, "fixture") }()
	<-startEntered
	assertPluginStatus(t, manager, "fixture", StateLoading, "")
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Enable error = %v, want context canceled", err)
	}
	assertPluginStatus(t, manager, "fixture", StateFailed, "context canceled")
	if got, want := events, []string{"start:fixture", "stop:fixture"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("canceled initialization events = %v, want %v", got, want)
	}
	toolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertUnknownTool(t, toolset, "fixture_echo")
}

func TestManagerConcurrentEnableStartsPluginOnce(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	plugin := newLifecycleFake("fixture", &events, &eventMu)
	var starts atomic.Int32
	plugin.start = func(context.Context) error {
		starts.Add(1)
		return nil
	}
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}

	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- manager.Enable(context.Background(), "fixture")
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("Start calls = %d, want 1", got)
	}
	assertPluginStatus(t, manager, "fixture", StateReady, "")
}

func TestLifecycleStateVocabulary(t *testing.T) {
	got := []LifecycleState{
		StateDisabled, StateWaiting, StateLoading, StateReady,
		StateFailed, StateStopping, StateStopped,
	}
	want := []LifecycleState{
		"disabled", "waiting", "loading", "ready", "failed", "stopping", "stopped",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Lifecycle states = %v, want exactly %v", got, want)
	}
}

func TestManagerStopFailureHidesDependentsContinuesCleanupAndRecoversBeforeRestart(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	dependency := newLifecycleFake("dependency", &events, &eventMu)
	dependent := newLifecycleFake("dependent", &events, &eventMu).withRequired("dependency")
	dependentStops := 0
	dependent.stop = func(context.Context) error {
		dependentStops++
		if dependentStops == 1 {
			return lifecycleError("dependent", "stop")
		}
		return nil
	}
	manager, err := NewManager(tools.NewToolset(nil), dependent, dependency)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{"dependent", "dependency"} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}

	err = manager.Stop(context.Background(), "dependency")
	if err == nil || !strings.Contains(err.Error(), "dependent stop failed") {
		t.Fatalf("Stop error = %v, want dependent cleanup failure", err)
	}
	toolset, toolsetErr := manager.NewSessionToolset()
	if toolsetErr != nil {
		t.Fatal(toolsetErr)
	}
	assertUnknownTool(t, toolset, "dependent_echo")
	assertUnknownTool(t, toolset, "dependency_echo")
	assertPluginStatus(t, manager, "dependent", StateFailed, "cleanup pending")
	assertPluginStatus(t, manager, "dependency", StateStopped, "")
	if got, want := events, []string{
		"start:dependency", "start:dependent", "stop:dependent", "stop:dependency",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events after failed Stop = %v, want %v", got, want)
	}

	if err := manager.Start(context.Background(), "dependency"); err != nil {
		t.Fatal(err)
	}
	if got, want := events, []string{
		"start:dependency", "start:dependent", "stop:dependent", "stop:dependency",
		"stop:dependent", "start:dependency", "start:dependent",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recovery events = %v, want %v", got, want)
	}
	toolset, err = manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, toolset, "dependency_echo", "dependency result")
	assertToolResult(t, toolset, "dependent_echo", "dependent result")
}

func TestManagerDisableFailureStaysHiddenAndRetriesCleanup(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	plugin := newLifecycleFake("fixture", &events, &eventMu)
	stops := 0
	plugin.stop = func(context.Context) error {
		stops++
		if stops == 1 {
			return lifecycleError("fixture", "stop")
		}
		return nil
	}
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Disable(context.Background(), "fixture"); err == nil {
		t.Fatal("Disable succeeded despite failed cleanup")
	}
	status, err := manager.Status("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.State != StateFailed || !strings.Contains(status.Diagnostic, "cleanup pending") {
		t.Fatalf("status after failed Disable = %+v, want disabled eligibility with failed cleanup pending", status)
	}
	if !manager.Inspect().Degraded {
		t.Fatal("cleanup-pending disabled plugin did not keep inspection degraded")
	}
	toolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertUnknownTool(t, toolset, "fixture_echo")

	if err := manager.Disable(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
	assertPluginStatus(t, manager, "fixture", StateDisabled, "")
	if manager.Inspect().Degraded {
		t.Fatal("inspection remained degraded after cleanup retry succeeded")
	}
}

func TestManagerAggregatesReverseOrderCleanupFailures(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	base := newLifecycleFake("base", &events, &eventMu)
	first := newLifecycleFake("first", &events, &eventMu).withRequired("base")
	second := newLifecycleFake("second", &events, &eventMu).withRequired("base")
	for _, plugin := range []*lifecycleFake{base, first, second} {
		id := plugin.Manifest().ID
		plugin.stop = func(context.Context) error { return lifecycleError(id, "stop") }
	}
	manager, err := NewManager(tools.NewToolset(nil), second, base, first)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{"second", "first", "base"} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}

	err = manager.Stop(context.Background(), "base")
	if err == nil {
		t.Fatal("Stop succeeded despite three cleanup failures")
	}
	for _, want := range []string{"second stop failed", "first stop failed", "base stop failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Stop error %q does not contain %q", err, want)
		}
	}
	if got, want := events[len(events)-3:], []string{"stop:second", "stop:first", "stop:base"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanup events = %v, want %v", got, want)
	}
}

func TestManagerFailedStartCleanupMustSucceedBeforeRetryStart(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	plugin := newLifecycleFake("fixture", &events, &eventMu)
	starts := 0
	stops := 0
	plugin.start = func(context.Context) error {
		starts++
		if starts == 1 {
			return lifecycleError("fixture", "start")
		}
		return nil
	}
	plugin.stop = func(context.Context) error {
		stops++
		if stops == 1 {
			return lifecycleError("fixture", "rollback")
		}
		return nil
	}
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
	assertPluginStatus(t, manager, "fixture", StateFailed, "cleanup pending")
	if starts != 1 || stops != 1 {
		t.Fatalf("after failed initialization starts=%d stops=%d, want 1/1", starts, stops)
	}

	if err := manager.Start(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
	if starts != 2 || stops != 2 {
		t.Fatalf("after recovery starts=%d stops=%d, want cleanup retry before second start", starts, stops)
	}
	if got, want := events, []string{
		"start:fixture", "stop:fixture", "stop:fixture", "start:fixture",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	assertPluginStatus(t, manager, "fixture", StateReady, "")
}

func TestManagerShutdownHidesContributionsBeforeHooksAndContinuesAfterCancellation(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	dependency := newLifecycleFake("dependency", &events, &eventMu)
	dependent := newLifecycleFake("dependent", &events, &eventMu).withRequired("dependency")
	stopEntered := make(chan struct{})
	stopAttempts := 0
	dependent.stop = func(ctx context.Context) error {
		stopAttempts++
		if stopAttempts == 1 {
			close(stopEntered)
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	manager, err := NewManager(tools.NewToolset(nil), dependent, dependency)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{"dependent", "dependency"} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- manager.Stop(ctx, "dependency") }()
	<-stopEntered
	toolset, toolsetErr := manager.NewSessionToolset()
	if toolsetErr != nil {
		t.Fatal(toolsetErr)
	}
	assertUnknownTool(t, toolset, "dependent_echo")
	assertUnknownTool(t, toolset, "dependency_echo")
	assertPluginStatus(t, manager, "dependent", StateStopping, "")
	assertPluginStatus(t, manager, "dependency", StateStopping, "")
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop error = %v, want context canceled", err)
	}
	if got := events; len(got) < 4 || got[len(got)-2] != "stop:dependent" || got[len(got)-1] != "stop:dependency" {
		t.Fatalf("events = %v, want both reverse-order stop attempts", got)
	}
	assertPluginStatus(t, manager, "dependent", StateFailed, "cleanup pending")
	assertPluginStatus(t, manager, "dependency", StateStopped, "")

	if err := manager.Start(context.Background(), "dependency"); err != nil {
		t.Fatal(err)
	}
	assertPluginStatus(t, manager, "dependency", StateReady, "")
	assertPluginStatus(t, manager, "dependent", StateReady, "")
	recovered, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, recovered, "dependency_echo", "dependency result")
	assertToolResult(t, recovered, "dependent_echo", "dependent result")
}

func TestManagerLifecycleAdmissionHonorsCancellation(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	plugin := newLifecycleFake("fixture", &events, &eventMu)
	startEntered := make(chan struct{})
	releaseStart := make(chan struct{})
	plugin.start = func(context.Context) error {
		close(startEntered)
		<-releaseStart
		return nil
	}
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}
	first := make(chan error, 1)
	go func() { first <- manager.Enable(context.Background(), "fixture") }()
	<-startEntered

	waitCtx, cancelWait := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelWait()
	if err := manager.Stop(waitCtx, "fixture"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting Stop error = %v, want deadline exceeded", err)
	}
	assertPluginStatus(t, manager, "fixture", StateLoading, "")
	close(releaseStart)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	assertPluginStatus(t, manager, "fixture", StateReady, "")
}

func TestManagerFailedStartRollbackIsBoundedAndRecoverable(t *testing.T) {
	var events []string
	var eventMu sync.Mutex
	plugin := newLifecycleFake("fixture", &events, &eventMu)
	plugin.start = func(context.Context) error { return lifecycleError("fixture", "start") }
	plugin.stop = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}
	manager.cleanupTimeout = 20 * time.Millisecond

	result := make(chan error, 1)
	go func() { result <- manager.Enable(context.Background(), "fixture") }()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Enable hung on failed-start rollback cleanup")
	}
	assertPluginStatus(t, manager, "fixture", StateFailed, "cleanup pending")
	toolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertUnknownTool(t, toolset, "fixture_echo")
}

func assertPluginStatus(t *testing.T, manager *Manager, id PluginID, state LifecycleState, diagnostic string) {
	t.Helper()
	status, err := manager.Status(id)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != state || !strings.Contains(status.Diagnostic, diagnostic) {
		t.Fatalf("status(%q) = %+v, want state %q diagnostic containing %q", id, status, state, diagnostic)
	}
}

func lifecycleError(id PluginID, phase string) error {
	return fmt.Errorf("%s %s failed", id, phase)
}
