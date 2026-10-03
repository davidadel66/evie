package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/memorybackground"
)

func backgroundTestStore(t *testing.T) *eviedb.Store {
	t.Helper()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return eviedb.NewStore(db)
}

func lookup(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

// `evie memory-background status` shows whether the pass is on, its model,
// triggers, caps and today's spend, and the cursor of each conversation,
// without starting a pass or contacting a provider.
func TestMemoryBackgroundStatusCommandReportsConfigurationAndProgress(t *testing.T) {
	store := backgroundTestStore(t)
	session, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	handled, err := runMemoryBackgroundManagement(context.Background(), []string{"memory-background", "status"}, &out, store,
		lookup(map[string]string{"EVIE_REMOTE_MEMORY": "on", "OPENROUTER_API_KEY": "or-test-credential"}))
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	var status memory.BackgroundStatus
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.State != memory.BackgroundStateOn || status.Model != memorybackground.DefaultModel || status.Provider != "openrouter" ||
		status.IdleMinutes != 10 || status.TurnTrigger != 20 || status.DailyTokenCap != 300_000 || status.DailyMicroUSDCap != 250_000 ||
		len(status.Progress.Sessions) != 1 || status.Progress.Sessions[0].SessionID != session.ID {
		t.Fatalf("status = %+v", status)
	}
	if strings.Contains(out.String(), "or-test-credential") {
		t.Fatal("status printed the API key")
	}

	out.Reset()
	if _, err := runMemoryBackgroundManagement(context.Background(), []string{"memory-background", "status"}, &out, store, lookup(nil)); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.State != memory.BackgroundStateDisabled || !strings.Contains(status.Reason, "EVIE_REMOTE_MEMORY=on") {
		t.Fatalf("disabled status = %+v", status)
	}
	if handled, err := runMemoryBackgroundManagement(context.Background(), []string{"memory-background", "run"}, &out, store, lookup(nil)); !handled || err == nil {
		t.Fatalf("an unknown action handled=%v err=%v", handled, err)
	}
	if handled, _ := runMemoryBackgroundManagement(context.Background(), []string{"sessions"}, &out, store, lookup(nil)); handled {
		t.Fatal("another command was handled")
	}
}

// A disabled host starts and stops without a goroutine doing work; an
// enabled host stops within its bound on shutdown.
func TestMemoryBackgroundHostStartsAndStopsWithinItsBound(t *testing.T) {
	store := backgroundTestStore(t)
	disabled := newMemoryBackgroundHost(lookup(nil), store)
	stop := disabled.Start(context.Background())
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	status, err := disabled.MemoryBackgroundStatus(context.Background())
	if err != nil || status.Enabled {
		t.Fatalf("status = %+v, %v", status, err)
	}

	local := newMemoryBackgroundHost(lookup(map[string]string{"EVIE_MEMORY_BACKGROUND": "local", "EVIE_COMPILER_CONFIG": filepath.Join(t.TempDir(), "missing.json")}), store)
	status, err = local.MemoryBackgroundStatus(context.Background())
	if err != nil || status.Enabled || !strings.Contains(status.Reason, "local extractor") {
		t.Fatalf("an unreadable local configuration = %+v, %v", status, err)
	}

	enabled := newMemoryBackgroundHost(lookup(map[string]string{"EVIE_REMOTE_MEMORY": "on", "OPENROUTER_API_KEY": "key"}), store)
	fetched := make(chan struct{}, 1)
	enabled.pricing = func(context.Context) (memorybackground.Pricing, error) {
		fetched <- struct{}{}
		return memorybackground.Pricing{InputPerMillion: 0.3, OutputPerMillion: 1.2, Source: "catalog"}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	stop = enabled.Start(ctx)
	<-fetched
	cancel()
	started := time.Now()
	if err := stop(); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if time.Since(started) > 6*time.Second {
		t.Fatal("shutdown exceeded its bound")
	}
}
