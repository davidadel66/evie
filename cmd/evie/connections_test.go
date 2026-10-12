package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeServe answers the connection routes the way evie serve does and
// records each request body.
func fakeServe(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	bodies := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s %s with content type %q", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		var body bytes.Buffer
		_, _ = body.ReadFrom(r.Body)
		bodies = append(bodies, r.URL.Path+" "+body.String())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/connections/list":
			_, _ = w.Write([]byte(`{"connections":[{"id":"mcp-square","name":"square","url":"https://mcp.squareup.com/mcp","status":"connected"}]}`))
		case "/api/connections/add":
			_, _ = w.Write([]byte(`{"id":"mcp-canva","name":"canva","status":"not_connected"}`))
		case "/api/connections/connect":
			_, _ = w.Write([]byte(`{"id":"state-1","connection_id":"mcp-square","authorization_url":"https://mcp.squareup.com/authorize?state=state-1"}`))
		case "/api/connections/wait":
			_, _ = w.Write([]byte(`{"id":"mcp-square","name":"square","status":"connected"}`))
		case "/api/connections/disconnect":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"connection_not_found","error":"no such MCP connection"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &bodies
}

func runConnections(t *testing.T, addr, stdin string, opened *[]string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	handled, err := runConnectionManagement(context.Background(), append([]string{"connections"}, args...), &out, strings.NewReader(stdin), connectionCommands{
		addr: addr,
		open: func(url string) error { *opened = append(*opened, url); return nil },
	})
	if !handled {
		t.Fatal("connections command not handled")
	}
	return out.String(), err
}

func TestConnectionsCLITalksToServe(t *testing.T) {
	srv, bodies := fakeServe(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	var opened []string

	out, err := runConnections(t, addr, "", &opened, "list")
	if err != nil || !strings.Contains(out, `"status": "connected"`) {
		t.Fatalf("list = %q, %v", out, err)
	}

	out, err = runConnections(t, addr, "portal-secret\n", &opened, "add", "canva", "https://mcp.canva.com/mcp", "--client-id", "portal")
	if err != nil || strings.Contains(out, "portal-secret") {
		t.Fatalf("add = %q, %v", out, err)
	}
	var added struct {
		Name, URL, ClientID, ClientSecret string
	}
	_ = json.Unmarshal([]byte(strings.TrimPrefix((*bodies)[1], "/api/connections/add ")), &struct {
		Name         *string `json:"name"`
		URL          *string `json:"url"`
		ClientID     *string `json:"client_id"`
		ClientSecret *string `json:"client_secret"`
	}{&added.Name, &added.URL, &added.ClientID, &added.ClientSecret})
	if added.Name != "canva" || added.URL != "https://mcp.canva.com/mcp" || added.ClientID != "portal" || added.ClientSecret != "portal-secret" {
		t.Fatalf("add sent %q", (*bodies)[1])
	}

	out, err = runConnections(t, addr, "", &opened, "connect", "square")
	if err != nil || len(opened) != 1 || opened[0] != "https://mcp.squareup.com/authorize?state=state-1" ||
		!strings.Contains(out, "https://mcp.squareup.com/authorize?state=state-1") || !strings.Contains(out, `"status": "connected"`) {
		t.Fatalf("connect = %q, %v, opened %v", out, err, opened)
	}
	if last := (*bodies)[len(*bodies)-1]; last != `/api/connections/wait {"attempt_id":"state-1"}` {
		t.Fatalf("connect waited with %q", last)
	}

	if _, err := runConnections(t, addr, "", &opened, "disconnect", "absent"); err == nil || !strings.Contains(err.Error(), "no such MCP connection") {
		t.Fatalf("disconnect error = %v", err)
	}
}

func TestConnectionsCLIAddWithoutClientIDReadsNoSecret(t *testing.T) {
	srv, bodies := fakeServe(t)
	var opened []string
	if _, err := runConnections(t, strings.TrimPrefix(srv.URL, "http://"), "ignored", &opened, "add", "square", "https://mcp.squareup.com/mcp"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains((*bodies)[0], "ignored") {
		t.Fatalf("add read stdin without --client-id: %q", (*bodies)[0])
	}
}

func TestConnectionsCLIExplainsWhenServeIsNotRunning(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	var opened []string
	for _, args := range [][]string{{"list"}, {"add", "square", "https://mcp.squareup.com/mcp"}, {"connect", "square"}, {"disconnect", "square"}} {
		_, err := runConnections(t, addr, "", &opened, args...)
		if err == nil || !strings.Contains(err.Error(), "evie serve is not running at "+addr) {
			t.Fatalf("%v error = %v", args, err)
		}
	}
	for _, args := range [][]string{{"ws-1", "square"}, {"ws-1", "square", "--disable", "make_api_request"}} {
		var out bytes.Buffer
		_, err := runConnectionManagement(context.Background(), append([]string{"workspaces", "mcp-tools"}, args...), &out, strings.NewReader(""), connectionCommands{addr: addr})
		if err == nil || !strings.Contains(err.Error(), "evie serve is not running at "+addr) {
			t.Fatalf("mcp-tools %v error = %v", args, err)
		}
	}
}

func TestWorkspaceMCPToolsCLITalksToServe(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body bytes.Buffer
		_, _ = body.ReadFrom(r.Body)
		bodies = append(bodies, r.URL.Path+" "+body.String())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_revision":2,"tools":[{"name":"make_api_request_read","state":"enabled"}]}`))
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		handled, err := runConnectionManagement(context.Background(), append([]string{"workspaces", "mcp-tools"}, args...), &out, strings.NewReader(""), connectionCommands{addr: addr})
		if !handled {
			t.Fatal("workspaces mcp-tools not handled")
		}
		return out.String(), err
	}
	if out, err := run("ws-1", "square"); err != nil || !strings.Contains(out, `"state": "enabled"`) {
		t.Fatalf("list = %q, %v", out, err)
	}
	if _, err := run("ws-1", "square", "--enable", "make_api_request_read"); err != nil {
		t.Fatal(err)
	}
	if _, err := run("ws-1", "square", "--disable", "make_api_request"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`/api/connections/tools {"workspace_id":"ws-1","name":"square"}`,
		`/api/connections/tools/set {"workspace_id":"ws-1","name":"square","tool":"make_api_request_read","enabled":true}`,
		`/api/connections/tools/set {"workspace_id":"ws-1","name":"square","tool":"make_api_request","enabled":false}`,
	}
	if strings.Join(bodies, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests =\n%s", strings.Join(bodies, "\n"))
	}
	for _, args := range [][]string{{"ws-1"}, {"ws-1", "square", "--enable"}, {"ws-1", "square", "--bogus", "x"}} {
		if _, err := run(args...); err == nil || !strings.Contains(err.Error(), "usage: evie workspaces mcp-tools") {
			t.Fatalf("%v error = %v", args, err)
		}
	}
	var out bytes.Buffer
	if handled, _ := runConnectionManagement(context.Background(), []string{"workspaces", "list"}, &out, strings.NewReader(""), connectionCommands{}); handled {
		t.Fatal("another workspaces command was handled")
	}
}

func TestConnectionsCLIUsage(t *testing.T) {
	var opened []string
	for _, args := range [][]string{{}, {"bogus"}, {"add", "square"}, {"connect"}, {"add", "square", "https://x", "--client-id"}} {
		if _, err := runConnections(t, "127.0.0.1:1", "", &opened, args...); err == nil || !strings.Contains(err.Error(), "usage: evie connections") {
			t.Fatalf("%v error = %v", args, err)
		}
	}
	var out bytes.Buffer
	if handled, _ := runConnectionManagement(context.Background(), []string{"workspaces"}, &out, strings.NewReader(""), connectionCommands{}); handled {
		t.Fatal("another command was handled")
	}
}
