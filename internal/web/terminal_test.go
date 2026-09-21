package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
)

func TestTerminalHTTPStreamingAndLifetime(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("HOME", t.TempDir())
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	workspace, err := store.RegisterWorkspace(context.Background(), "Terminal")
	if err != nil {
		t.Fatal(err)
	}
	folder, err := store.SetWorkspaceFolder(context.Background(), workspace.ID, 0, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := NewContextServer(nil, nil, nil, &folderTestController{store: store})
	host := httptest.NewServer(server.Handler())
	defer func() { server.Close(); host.Close() }()
	client := &http.Client{Timeout: 5 * time.Second}
	request := terminalRequest{WorkspaceID: workspace.ID, Revision: folder.Revision, Columns: 80, Rows: 24}
	call := func(action string, body any) *http.Response {
		t.Helper()
		encoded, _ := json.Marshal(body)
		response, err := client.Post(host.URL+"/api/terminal/"+action, "application/json", strings.NewReader(string(encoded)))
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	control := func(action string, body terminalRequest) {
		t.Helper()
		res := call(action, body)
		defer res.Body.Close()
		if res.StatusCode != 200 {
			data, _ := io.ReadAll(res.Body)
			t.Fatalf("%s: %d %s", action, res.StatusCode, data)
		}
	}
	open := func() (*http.Response, *json.Decoder, *terminalEntry) {
		t.Helper()
		res := call("open", request)
		if res.StatusCode != 200 {
			data, _ := io.ReadAll(res.Body)
			res.Body.Close()
			t.Fatalf("open: %d %s", res.StatusCode, data)
		}
		decoder := json.NewDecoder(res.Body)
		var event terminalEvent
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		if event.Type != "ready" || event.ID == "" {
			t.Fatalf("ready=%+v", event)
		}
		request.ID = event.ID
		server.terminalMu.Lock()
		entry := server.terminals[event.ID]
		server.terminalMu.Unlock()
		return res, decoder, entry
	}
	awaitClosed := func(entry *terminalEntry) {
		t.Helper()
		select {
		case <-entry.process.Closed():
		case <-time.After(4 * time.Second):
			t.Fatal("terminal did not close")
		}
	}
	for _, action := range []string{"open", "input", "resize", "close"} {
		bad := request
		bad.Revision--
		res := call(action, bad)
		res.Body.Close()
		if res.StatusCode != 409 {
			t.Fatalf("stale %s: %d", action, res.StatusCode)
		}
	}
	bad := request
	bad.Columns = 0
	res := call("open", bad)
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("dimensions=%d", res.StatusCode)
	}
	origin := managementRequest("/api/terminal/open", `{}`)
	origin.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, origin)
	if rec.Code != 403 {
		t.Fatalf("origin=%d", rec.Code)
	}
	res, decoder, entry := open()
	request.Columns = 103
	request.Rows = 39
	control("resize", request)
	request.Data = "printf '\\n__ROOT__%s\\n' \"$PWD\"; stty size; exit 7\n"
	control("input", request)
	var output strings.Builder
	for {
		var event terminalEvent
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		if event.Type == "output" {
			output.Write(event.Data)
		}
		if event.Type == "exit" {
			if event.Code != 7 {
				t.Fatalf("exit=%d", event.Code)
			}
			break
		}
	}
	res.Body.Close()
	awaitClosed(entry)
	if !strings.Contains(output.String(), "__ROOT__"+folder.Path) || !strings.Contains(output.String(), "39 103") {
		t.Fatalf("output: %s", output.String())
	}
	res, _, entry = open()
	res.Body.Close()
	awaitClosed(entry) // disconnected browser
	res, _, entry = open()
	control("close", request)
	awaitClosed(entry)
	res.Body.Close()
	res, _, entry = open()
	data, _ := json.Marshal(folderRequest{WorkspaceID: workspace.ID, Revision: folder.Revision, Path: ""})
	changed, err := client.Post(host.URL+"/api/workspaces/folder", "application/json", strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	changed.Body.Close()
	if changed.StatusCode != 200 {
		t.Fatalf("detach=%d", changed.StatusCode)
	}
	awaitClosed(entry)
	res.Body.Close()
	folder, err = store.SetWorkspaceFolder(context.Background(), workspace.ID, folder.Revision+1, folder.Path)
	if err != nil {
		t.Fatal(err)
	}
	request.Revision = folder.Revision
	res, _, entry = open()
	server.Close()
	awaitClosed(entry)
	res.Body.Close()
}
