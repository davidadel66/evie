package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/web"
)

// Optional real-browser fixture: temporary database and synthetic provider only.
func TestChatModelsBrowserFixture(t *testing.T) {
	if os.Getenv("EVIE_MODELS_BROWSER_FIXTURE") != "1" {
		t.Skip("opt-in chat model browser fixture")
	}
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "browser.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	controller := modelFixtureController(t, store, &resumeCaptureClient{})
	opened, err := controller.SelectSession(context.Background(), web.ContextSessionSelection{Unscoped: true})
	if err != nil {
		t.Fatal(err)
	}
	server := web.NewContextServer(nil, nil, store, controller)
	defer server.Close()
	host := httptest.NewServer(server.Handler())
	defer host.Close()
	fmt.Printf("MODELS_BROWSER_READY=%s %s\n", host.URL, opened.Session.ID)
	var stop [1]byte
	_, _ = os.Stdin.Read(stop[:])
}
