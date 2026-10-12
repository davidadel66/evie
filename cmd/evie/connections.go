package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/davidadel66/evie/internal/mcpconn"
)

// `evie connections` (workspace-mcp-connections.spec.md story 1). evie
// serve is the only process that holds MCP tokens, so every subcommand is a
// thin client of serve's loopback routes and fails plainly when serve is
// not running.

const connectionsUsage = "usage: evie connections list | add <name> <https-url> [--client-id <id>] (client secret on stdin) | connect <name> | disconnect <name>"

type connectionCommands struct {
	addr string                 // serve's host:port
	open func(url string) error // opens the owner's browser
}

func defaultConnectionCommands(getenv func(string) string) connectionCommands {
	addr := strings.TrimSpace(getenv("EVIE_ADDR"))
	if addr == "" {
		addr = "127.0.0.1:6687"
	}
	return connectionCommands{addr: addr, open: func(url string) error { return exec.Command("open", url).Run() }}
}

// newMCPConnectionManager is serve's only owner of MCP sign-ins and
// tokens; its callback route lives at serve's own address.
func newMCPConnectionManager(store mcpconn.Records, getenv func(string) string) *mcpconn.Manager {
	return mcpconn.NewManager(mcpconn.Config{
		Records:     store,
		Secrets:     mcpconn.Keychain{Run: mcpconn.RunSecurity},
		HTTPClient:  &http.Client{Timeout: 30 * time.Second},
		RedirectURL: "http://" + defaultConnectionCommands(getenv).addr + "/api/connections/oauth/callback",
	})
}

func runConnectionManagement(ctx context.Context, args []string, out io.Writer, in io.Reader, c connectionCommands) (bool, error) {
	if len(args) == 0 || args[0] != "connections" {
		return false, nil
	}
	args = args[1:]
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	switch {
	case command == "list" && len(args) == 1:
		return true, c.call(ctx, out, "list", struct{}{}, 30*time.Second)
	case command == "add" && (len(args) == 3 || (len(args) == 5 && args[3] == "--client-id" && args[4] != "")):
		request := struct {
			Name         string `json:"name"`
			URL          string `json:"url"`
			ClientID     string `json:"client_id,omitempty"`
			ClientSecret string `json:"client_secret,omitempty"`
		}{Name: args[1], URL: args[2]}
		if len(args) == 5 {
			request.ClientID = args[4]
			secret, err := io.ReadAll(io.LimitReader(in, 8192))
			if err != nil {
				return true, err
			}
			request.ClientSecret = strings.TrimSpace(string(secret))
		}
		return true, c.call(ctx, out, "add", request, 30*time.Second)
	case command == "connect" && len(args) == 2:
		var attempt struct {
			ID               string `json:"id"`
			AuthorizationURL string `json:"authorization_url"`
		}
		if err := c.post(ctx, "connect", struct {
			Name string `json:"name"`
		}{args[1]}, &attempt, 60*time.Second); err != nil {
			return true, err
		}
		fmt.Fprintf(out, "Sign in to finish connecting %s:\n%s\n", args[1], attempt.AuthorizationURL)
		if err := c.open(attempt.AuthorizationURL); err != nil {
			fmt.Fprintln(out, "Open that URL in your browser.")
		}
		fmt.Fprintln(out, "Waiting for the sign-in to finish…")
		// The attempt expires after ten minutes; serve ends the wait then.
		return true, c.call(ctx, out, "wait", struct {
			AttemptID string `json:"attempt_id"`
		}{attempt.ID}, 11*time.Minute)
	case command == "disconnect" && len(args) == 2:
		return true, c.call(ctx, out, "disconnect", struct {
			Name string `json:"name"`
		}{args[1]}, 60*time.Second)
	default:
		return true, errors.New(connectionsUsage)
	}
}

// call posts to one connection route and prints the JSON answer.
func (c connectionCommands) call(ctx context.Context, out io.Writer, route string, request any, timeout time.Duration) error {
	var answer json.RawMessage
	if err := c.post(ctx, route, request, &answer, timeout); err != nil {
		return err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, answer, "", "  "); err != nil {
		return err
	}
	pretty.WriteByte('\n')
	_, err := out.Write(pretty.Bytes())
	return err
}

func (c connectionCommands) post(ctx context.Context, route string, request, answer any, timeout time.Duration) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+c.addr+"/api/connections/"+route, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("evie serve is not running at %s; start it, then try again", c.addr)
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &failure) == nil && failure.Error != "" {
			return errors.New(failure.Error)
		}
		return fmt.Errorf("evie serve answered %d", resp.StatusCode)
	}
	return json.Unmarshal(raw, answer)
}
