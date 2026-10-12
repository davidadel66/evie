// Package mcpconn owns MCP Connections (workspace-mcp-connections.spec.md):
// the Kernel's record of each hosted MCP server, the OAuth sign-in that
// connects it, the Keychain that holds its credentials, and the HTTP
// transport that carries its tokens. Only `evie serve` runs a Manager, so
// one process ever refreshes a connection's tokens.
package mcpconn

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Status is a connection's secret-free state, written by serve.
type Status string

const (
	StatusNotConnected        Status = "not_connected"
	StatusConnected           Status = "connected"
	StatusNeedsSignIn         Status = "needs_sign_in"
	StatusUnreachable         Status = "unreachable"
	StatusKeychainUnavailable Status = "keychain_unavailable"
)

// Connection is one MCP Connection record. It never carries credentials.
type Connection struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	URL          string        `json:"url"`
	ClientID     string        `json:"client_id,omitempty"`
	Status       Status        `json:"status"`
	StatusDetail string        `json:"status_detail,omitempty"`
	OAuth        OAuthMetadata `json:"-"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// OAuthMetadata is the non-secret discovery result a connection signs in
// and refreshes against.
type OAuthMetadata struct {
	Resource              string   `json:"resource,omitempty"`
	Issuer                string   `json:"issuer,omitempty"`
	AuthorizationEndpoint string   `json:"authorization_endpoint,omitempty"`
	TokenEndpoint         string   `json:"token_endpoint,omitempty"`
	RevocationEndpoint    string   `json:"revocation_endpoint,omitempty"`
	RegistrationEndpoint  string   `json:"registration_endpoint,omitempty"`
	TokenAuthMethods      []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	Scopes                []string `json:"scopes,omitempty"`
	// RegisteredRedirect is the redirect URI the client was registered or
	// used with; a reconnect reuses a dynamic registration only while it
	// still matches serve's callback.
	RegisteredRedirect string `json:"registered_redirect_uri,omitempty"`
}

// Credentials are one connection's secret material. Only the Keychain holds
// them at rest.
type Credentials struct {
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret,omitempty"`
}

var (
	ErrConnectionNotFound = errors.New("no such MCP connection")
	ErrConnectionExists   = errors.New("an MCP connection with that name already exists")
	ErrNotConnected       = errors.New("the MCP connection is not signed in")
	// ErrInvalid marks a request the owner can correct: a bad name or URL.
	ErrInvalid = errors.New("invalid MCP connection request")
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,23}$`)
var idPattern = regexp.MustCompile(`^mcp-[a-z][a-z0-9-]{0,23}$`)

// ConnectionID is the Connection ID for an owner-chosen name. The prefix
// keeps MCP connections apart from built-in Connection IDs such as
// `github` and `finance-local`.
func ConnectionID(name string) string { return "mcp-" + name }

// ValidateName checks an owner-chosen connection name.
func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("%w: connection name %q must be 1-24 lowercase letters, digits or dashes, starting with a letter", ErrInvalid, name)
	}
	return nil
}

// ValidateURL checks a hosted server URL: https, a host, no credentials,
// no fragment.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || strings.ContainsAny(raw, " \t\r\n") {
		return fmt.Errorf("%w: server URL %q must be an https URL with a host", ErrInvalid, raw)
	}
	return nil
}
