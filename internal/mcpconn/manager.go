package mcpconn

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// attemptLifetime bounds how long a sign-in may take between Connect and
// the callback.
const attemptLifetime = 10 * time.Minute

// refreshMargin refreshes an access token this long before it expires.
const refreshMargin = time.Minute

// tokenWorkTimeout bounds a code exchange or refresh together with saving
// its result. That work is detached from the caller: once a server has
// rotated a refresh token, a cancelled turn must not lose the new one.
const tokenWorkTimeout = 30 * time.Second

var ErrUnknownAttempt = errors.New("no such sign-in attempt, or it expired or was already used")

// Records persists connection records; eviedb implements it.
type Records interface {
	MCPConnections(ctx context.Context) ([]Connection, error)
	MCPConnection(ctx context.Context, id string) (Connection, error)
	CreateMCPConnection(ctx context.Context, connection Connection) error
	// SetMCPConnectionStatus records status and detail, and the OAuth
	// metadata when oauth is non-nil.
	SetMCPConnectionStatus(ctx context.Context, id string, status Status, detail string, oauth *OAuthMetadata, at time.Time) error
}

// Secrets holds credentials at rest; Keychain implements it.
type Secrets interface {
	Save(ctx context.Context, id string, credentials Credentials) error
	Load(ctx context.Context, id string) (Credentials, error)
	Delete(ctx context.Context, id string) error
}

type Config struct {
	Records Records
	Secrets Secrets
	// HTTPClient carries discovery, registration and token requests; it
	// never carries a bearer token.
	HTTPClient *http.Client
	// RedirectURL is serve's callback route.
	RedirectURL string
	Now         func() time.Time
}

// Manager is serve's owner of every connection's sign-in and tokens.
type Manager struct {
	cfg Config

	mu       sync.Mutex
	attempts map[string]*attempt // keyed by state, which is also the attempt ID
	conns    map[string]*connectionState
}

type attempt struct {
	connectionID string
	verifier     string
	credentials  Credentials // client registration for this attempt
	meta         OAuthMetadata
	expires      time.Time
	used         bool
	done         chan struct{}
	result       Connection
	err          error
}

// connectionState serializes one connection's token work and caches its
// credentials so a request does not run security(1) every time.
type connectionState struct {
	mu          sync.Mutex
	loaded      bool
	credentials Credentials
	// unsaved marks credentials a refresh rotated that the Keychain has not
	// accepted yet; they are not used until they are saved.
	unsaved bool
}

// Attempt is a started sign-in.
type Attempt struct {
	ID               string    `json:"id"`
	ConnectionID     string    `json:"connection_id"`
	AuthorizationURL string    `json:"authorization_url"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type AddRequest struct {
	Name         string
	URL          string
	ClientID     string
	ClientSecret string
}

func NewManager(cfg Config) *Manager {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Manager{cfg: cfg, attempts: map[string]*attempt{}, conns: map[string]*connectionState{}}
}

func (m *Manager) now() time.Time { return m.cfg.Now().UTC() }

func (m *Manager) List(ctx context.Context) ([]Connection, error) {
	return m.cfg.Records.MCPConnections(ctx)
}

// Connection reads one connection by name.
func (m *Manager) Connection(ctx context.Context, name string) (Connection, error) {
	if err := ValidateName(name); err != nil {
		return Connection{}, err
	}
	return m.cfg.Records.MCPConnection(ctx, ConnectionID(name))
}

// Add records a new connection. A pre-registered client's secret goes
// straight to the Keychain.
func (m *Manager) Add(ctx context.Context, req AddRequest) (Connection, error) {
	if err := ValidateName(req.Name); err != nil {
		return Connection{}, err
	}
	if err := ValidateURL(req.URL); err != nil {
		return Connection{}, err
	}
	if req.ClientSecret != "" && req.ClientID == "" {
		return Connection{}, fmt.Errorf("%w: a client secret needs its client ID", ErrInvalid)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := ConnectionID(req.Name)
	if _, err := m.cfg.Records.MCPConnection(ctx, id); err == nil {
		return Connection{}, ErrConnectionExists
	} else if !errors.Is(err, ErrConnectionNotFound) {
		return Connection{}, err
	}
	if req.ClientSecret != "" {
		if err := m.cfg.Secrets.Save(ctx, id, Credentials{ClientID: req.ClientID, ClientSecret: req.ClientSecret}); err != nil {
			return Connection{}, err
		}
	}
	now := m.now()
	connection := Connection{ID: id, Name: req.Name, URL: req.URL, ClientID: req.ClientID, Status: StatusNotConnected, CreatedAt: now, UpdatedAt: now}
	if err := m.cfg.Records.CreateMCPConnection(ctx, connection); err != nil {
		return Connection{}, err
	}
	return connection, nil
}

// BeginConnect discovers the server's authorization server, settles the
// client (pre-registered, an earlier registration, or a new one), and
// returns the URL the owner opens to sign in.
func (m *Manager) BeginConnect(ctx context.Context, name string) (Attempt, error) {
	connection, err := m.Connection(ctx, name)
	if err != nil {
		return Attempt{}, err
	}
	meta, err := discover(ctx, m.cfg.HTTPClient, connection.URL)
	if err != nil {
		return Attempt{}, err
	}
	stored, err := m.cfg.Secrets.Load(ctx, connection.ID)
	if err != nil && !errors.Is(err, ErrNoCredentials) {
		return Attempt{}, err
	}
	client := Credentials{ClientID: connection.ClientID}
	switch {
	case connection.ClientID != "":
		if stored.ClientID == connection.ClientID {
			client.ClientSecret = stored.ClientSecret
		}
	case stored.ClientID != "" && connection.OAuth.RegisteredRedirect == m.cfg.RedirectURL && connection.OAuth.Issuer == meta.Issuer:
		// The registration from an earlier sign-in still fits.
		client.ClientID = stored.ClientID
	default:
		if meta.RegistrationEndpoint == "" {
			return Attempt{}, errors.New("the server does not offer client registration; add the connection again with a pre-registered client ID")
		}
		if client.ClientID, err = register(ctx, m.cfg.HTTPClient, meta, m.cfg.RedirectURL); err != nil {
			return Attempt{}, err
		}
	}
	meta.RegisteredRedirect = m.cfg.RedirectURL
	verifier := randomToken(48)
	sum := sha256.Sum256([]byte(verifier))
	state := randomToken(24)
	now := m.now()
	a := &attempt{connectionID: connection.ID, verifier: verifier, credentials: client, meta: meta, expires: now.Add(attemptLifetime), done: make(chan struct{})}
	m.mu.Lock()
	m.pruneLocked(now)
	m.attempts[state] = a
	m.mu.Unlock()
	return Attempt{ID: state, ConnectionID: connection.ID, ExpiresAt: a.expires,
		AuthorizationURL: authorizationURL(meta, client.ClientID, m.cfg.RedirectURL, state, base64.RawURLEncoding.EncodeToString(sum[:]))}, nil
}

// Callback completes the attempt named by the query's state. Each state is
// accepted once, before it expires.
func (m *Manager) Callback(ctx context.Context, query url.Values) (Connection, error) {
	state := query.Get("state")
	m.mu.Lock()
	a, ok := m.attempts[state]
	now := m.now()
	if !ok || a.used || !now.Before(a.expires) {
		if ok && !a.used && !now.Before(a.expires) {
			delete(m.attempts, state)
		}
		m.mu.Unlock()
		return Connection{}, ErrUnknownAttempt
	}
	a.used = true
	m.mu.Unlock()

	connection, err := m.completeAttempt(ctx, a, query)
	m.mu.Lock()
	a.result, a.err = connection, err
	close(a.done)
	m.mu.Unlock()
	return connection, err
}

func (m *Manager) completeAttempt(ctx context.Context, a *attempt, query url.Values) (Connection, error) {
	if code := query.Get("error"); code != "" {
		if description := query.Get("error_description"); description != "" {
			code += ": " + description
		}
		return Connection{}, fmt.Errorf("sign-in was not completed: %s", code)
	}
	if query.Get("code") == "" {
		return Connection{}, errors.New("sign-in returned no authorization code")
	}
	work, cancel := detached(ctx)
	defer cancel()
	token, err := exchangeCode(work, m.cfg.HTTPClient, a.meta, a.credentials, query.Get("code"), a.verifier, m.cfg.RedirectURL)
	if err != nil {
		return Connection{}, err
	}
	state := m.state(a.connectionID)
	state.mu.Lock()
	defer state.mu.Unlock()
	credentials := token.credentials(a.credentials, m.now())
	if err := m.cfg.Secrets.Save(work, a.connectionID, credentials); err != nil {
		_ = m.setStatus(work, a.connectionID, StatusKeychainUnavailable, err.Error(), nil)
		return Connection{}, err
	}
	state.credentials, state.loaded, state.unsaved = credentials, true, false
	meta := a.meta
	if err := m.setStatus(work, a.connectionID, StatusConnected, "", &meta); err != nil {
		return Connection{}, err
	}
	return m.cfg.Records.MCPConnection(work, a.connectionID)
}

// Wait blocks until the attempt's callback completes, it expires unused,
// or ctx ends. Once the callback has started, only its result ends the
// wait.
func (m *Manager) Wait(ctx context.Context, id string) (Connection, error) {
	m.mu.Lock()
	a, ok := m.attempts[id]
	now := m.now()
	if ok && !a.used && !now.Before(a.expires) {
		delete(m.attempts, id)
		ok = false
	}
	used := ok && a.used
	m.mu.Unlock()
	if !ok {
		return Connection{}, ErrUnknownAttempt
	}
	if !used {
		timer := time.NewTimer(a.expires.Sub(now))
		defer timer.Stop()
		select {
		case <-a.done:
			return a.result, a.err
		case <-ctx.Done():
			return Connection{}, ctx.Err()
		case <-timer.C:
			m.mu.Lock()
			used = a.used
			m.mu.Unlock()
			if !used {
				return Connection{}, ErrUnknownAttempt
			}
		}
	}
	select {
	case <-a.done:
		return a.result, a.err
	case <-ctx.Done():
		return Connection{}, ctx.Err()
	}
}

// Disconnect revokes and forgets a connection's tokens. The client
// registration (and a pre-registered client's secret) stays so a reconnect
// can use it; the record and any Workspace references remain.
func (m *Manager) Disconnect(ctx context.Context, name string) (Connection, error) {
	connection, err := m.Connection(ctx, name)
	if err != nil {
		return Connection{}, err
	}
	state := m.state(connection.ID)
	state.mu.Lock()
	defer state.mu.Unlock()
	credentials, err := m.cfg.Secrets.Load(ctx, connection.ID)
	if err != nil && !errors.Is(err, ErrNoCredentials) {
		return Connection{}, err
	}
	if state.unsaved {
		credentials = state.credentials
	}
	if credentials.RefreshToken != "" {
		revoke(ctx, m.cfg.HTTPClient, connection.OAuth, credentials, credentials.RefreshToken, "refresh_token")
	} else {
		revoke(ctx, m.cfg.HTTPClient, connection.OAuth, credentials, credentials.AccessToken, "access_token")
	}
	client := Credentials{ClientID: credentials.ClientID, ClientSecret: credentials.ClientSecret}
	if client.ClientID == "" {
		err = m.cfg.Secrets.Delete(ctx, connection.ID)
	} else {
		err = m.cfg.Secrets.Save(ctx, connection.ID, client)
	}
	if err != nil {
		return Connection{}, err
	}
	state.credentials, state.loaded, state.unsaved = client, true, false
	if err := m.setStatus(ctx, connection.ID, StatusNotConnected, "", nil); err != nil {
		return Connection{}, err
	}
	return m.cfg.Records.MCPConnection(ctx, connection.ID)
}

// TokenSource is one connection's bearer tokens for the transport.
func (m *Manager) TokenSource(id string) TokenSource { return managerTokens{m: m, id: id} }

type managerTokens struct {
	m  *Manager
	id string
}

func (t managerTokens) Token(ctx context.Context) (string, error) {
	return t.m.token(ctx, t.id, "", false)
}

func (t managerTokens) Refresh(ctx context.Context, rejected string) (string, error) {
	return t.m.token(ctx, t.id, rejected, true)
}

// token returns a usable access token, refreshing under the connection's
// lock when it is about to expire or when the server rejected it. A
// rejection of a token another caller already replaced does not refresh.
func (m *Manager) token(ctx context.Context, id, rejected string, afterRejection bool) (string, error) {
	state := m.state(id)
	state.mu.Lock()
	defer state.mu.Unlock()
	work, cancel := detached(ctx)
	defer cancel()
	if state.unsaved {
		if err := m.cfg.Secrets.Save(work, id, state.credentials); err != nil {
			_ = m.setStatus(work, id, StatusKeychainUnavailable, err.Error(), nil)
			return "", err
		}
		state.unsaved = false
		m.markConnected(work, id)
	}
	if !state.loaded {
		credentials, err := m.cfg.Secrets.Load(work, id)
		switch {
		case errors.Is(err, ErrNoCredentials):
			credentials = Credentials{}
		case err != nil:
			_ = m.setStatus(work, id, StatusKeychainUnavailable, err.Error(), nil)
			return "", err
		}
		state.credentials, state.loaded = credentials, true
		if credentials.AccessToken != "" {
			m.markConnected(work, id)
		}
	}
	current := state.credentials
	if current.AccessToken == "" {
		return "", ErrNotConnected
	}
	if afterRejection && current.AccessToken != rejected {
		return current.AccessToken, nil
	}
	fresh := current.Expiry.IsZero() || m.now().Add(refreshMargin).Before(current.Expiry)
	if fresh && !afterRejection {
		return current.AccessToken, nil
	}
	if current.RefreshToken == "" {
		return "", m.needSignIn(work, id, state, "the access token expired and there is no refresh token")
	}
	connection, err := m.cfg.Records.MCPConnection(work, id)
	if err != nil {
		return "", err
	}
	response, err := refreshToken(work, m.cfg.HTTPClient, connection.OAuth, current)
	if errors.Is(err, errInvalidGrant) {
		return "", m.needSignIn(work, id, state, err.Error())
	}
	if err != nil {
		_ = m.setStatus(work, id, StatusUnreachable, err.Error(), nil)
		return "", err
	}
	// The server has rotated; the new credentials are the only good ones
	// now, so keep them even if the Keychain refuses them for the moment.
	state.credentials = response.credentials(current, m.now())
	if err := m.cfg.Secrets.Save(work, id, state.credentials); err != nil {
		state.unsaved = true
		_ = m.setStatus(work, id, StatusKeychainUnavailable, err.Error(), nil)
		return "", err
	}
	m.markConnected(work, id)
	return state.credentials.AccessToken, nil
}

// needSignIn keeps the client registration but drops the tokens, so later
// calls fail fast until the owner connects again.
func (m *Manager) needSignIn(ctx context.Context, id string, state *connectionState, detail string) error {
	client := Credentials{ClientID: state.credentials.ClientID, ClientSecret: state.credentials.ClientSecret}
	if err := m.cfg.Secrets.Save(ctx, id, client); err == nil {
		state.credentials, state.unsaved = client, false
	}
	_ = m.setStatus(ctx, id, StatusNeedsSignIn, detail, nil)
	return fmt.Errorf("%w: %s", ErrNotConnected, detail)
}

// markConnected clears a stale failure status once tokens work again.
func (m *Manager) markConnected(ctx context.Context, id string) {
	if connection, err := m.cfg.Records.MCPConnection(ctx, id); err == nil && connection.Status != StatusConnected {
		_ = m.setStatus(ctx, id, StatusConnected, "", nil)
	}
}

func (m *Manager) state(id string) *connectionState {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.conns[id]
	if !ok {
		state = &connectionState{}
		m.conns[id] = state
	}
	return state
}

func (m *Manager) setStatus(ctx context.Context, id string, status Status, detail string, oauth *OAuthMetadata) error {
	return m.cfg.Records.SetMCPConnectionStatus(ctx, id, status, detail, oauth, m.now())
}

// pruneLocked forgets unused attempts that expired; m.mu is held.
func (m *Manager) pruneLocked(now time.Time) {
	for state, a := range m.attempts {
		if !now.Before(a.expires) {
			delete(m.attempts, state)
		}
	}
}

func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), tokenWorkTimeout)
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
