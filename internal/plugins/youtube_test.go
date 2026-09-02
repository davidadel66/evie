package plugins

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

func TestStandardPresetRequiresYouTubeWhileKernelKeepsTranscriptInspection(t *testing.T) {
	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := manager.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"youtube_transcript", "youtube_scrape_channel"} {
		if countSchema(resolved.Toolset, name) != 1 {
			t.Fatalf("standard exposes %q %d times", name, countSchema(resolved.Toolset, name))
		}
	}
	if countSchema(tools.KernelToolset(), "query_db") != 1 {
		t.Fatal("generic read-only database inspection left the Kernel")
	}

	if err := manager.SetEnabled(YouTubePluginID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolvePreset(StandardPresetID); err == nil ||
		!strings.Contains(err.Error(), `required Capability "youtube.transcript" is unavailable`) {
		t.Fatalf("disabled YouTube resolved standard: %v", err)
	}
	if err := manager.SetEnabled(YouTubePluginID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolvePreset(StandardPresetID); err != nil {
		t.Fatalf("re-enabled YouTube did not restore standard: %v", err)
	}
}

type stoppingYouTube struct {
	YouTube
	stopStarted chan struct{}
	releaseStop chan struct{}
}

func (p *stoppingYouTube) Stop(ctx context.Context) error {
	close(p.stopStarted)
	select {
	case <-p.releaseStop:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestLegacyResumeFailsClosedDuringConcurrentYouTubeDisable(t *testing.T) {
	legacyManager, err := NewManager(tools.LegacyKernelToolset(), NewWeb(), NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID} {
		if err := legacyManager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	legacy, err := legacyManager.resolvePreset(preYouTubeStandardPreset())
	if err != nil {
		t.Fatal(err)
	}

	youtube := &stoppingYouTube{
		stopStarted: make(chan struct{}),
		releaseStop: make(chan struct{}),
	}
	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), youtube, NewTodo())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	disableDone := make(chan error, 1)
	go func() { disableDone <- manager.Disable(context.Background(), YouTubePluginID) }()
	<-youtube.stopStarted

	if resumed, err := manager.ResumeComposition(legacy.Receipt); err == nil || len(resumed.Toolset.Schemas()) != 0 ||
		!strings.Contains(err.Error(), `legacy provider plugin "youtube" is stopping`) {
		t.Fatalf("legacy resume during disable = schemas %v, error %v", resumed.Toolset.Schemas(), err)
	}
	close(youtube.releaseStop)
	if err := <-disableDone; err != nil {
		t.Fatal(err)
	}
}

func TestYouTubePluginPreservesCapabilityContractsAndDispatch(t *testing.T) {
	youtube := NewYouTube()
	wantManifest := Manifest{
		ID:                    YouTubePluginID,
		ImplementationVersion: "1.0.0",
		KernelCompatibility: VersionRange{
			Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0",
		},
		Capabilities: []CapabilityContract{
			{ID: YouTubeTranscriptCapabilityID, Version: "1.0.0"},
			{ID: YouTubeScrapeChannelCapabilityID, Version: "1.0.0"},
		},
	}
	if got := youtube.Manifest(); !reflect.DeepEqual(got, wantManifest) {
		t.Fatalf("YouTube manifest\n got: %+v\nwant: %+v", got, wantManifest)
	}

	manager, err := NewManager(tools.NewToolset(nil), youtube)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(YouTubePluginID, true); err != nil {
		t.Fatal(err)
	}
	toolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}

	wantSchemas := youtubeSchemasNamed(tools.BuiltinToolset().Schemas(), []string{
		"youtube_transcript", "youtube_scrape_channel",
	})
	if got := toolset.Schemas(); !reflect.DeepEqual(got, wantSchemas) {
		t.Fatalf("YouTube plugin schemas changed\n got: %#v\nwant: %#v", got, wantSchemas)
	}
	if got := schemaNames(toolset); !reflect.DeepEqual(got, []string{"youtube_transcript", "youtube_scrape_channel"}) {
		t.Fatalf("YouTube schema names = %v", got)
	}

	assertYouTubeDispatchResult(t, toolset, "youtube_transcript", `{}`, "tool call came back with error video must not be empty")
	assertYouTubeDispatchResult(t, toolset, "youtube_scrape_channel", `{}`, "tool call came back with error channel must not be empty")
}

func assertYouTubeDispatchResult(t *testing.T, toolset tools.Toolset, name, arguments, want string) {
	t.Helper()
	message, isErr, err := toolset.ExecuteWithApprovalAuthorizedCompletion(
		context.Background(), openrouter.ToolCall{
			ID: "call-youtube", Type: "function",
			Function: openrouter.FunctionCall{Name: name, Arguments: arguments},
		}, nil, nil, nil, nil,
	)
	if err != nil || !isErr || message.Content != want {
		t.Fatalf("execute %q = (%+v, %v, %v), want tool error %q", name, message, isErr, err, want)
	}
}

func youtubeSchemasNamed(schemas []openrouter.Tool, names []string) []openrouter.Tool {
	selected := make([]openrouter.Tool, 0, len(names))
	for _, name := range names {
		for _, schema := range schemas {
			if schema.Function.Name == name {
				selected = append(selected, schema)
				break
			}
		}
	}
	return selected
}
