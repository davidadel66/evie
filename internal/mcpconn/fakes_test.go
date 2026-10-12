package mcpconn

import (
	"context"
	"sort"
	"sync"
	"time"
)

// memoryRecords is an in-memory Records for Manager tests.
type memoryRecords struct {
	mu      sync.Mutex
	records map[string]Connection
}

func (m *memoryRecords) MCPConnections(context.Context) ([]Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Connection
	for _, c := range m.records {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *memoryRecords) MCPConnection(_ context.Context, id string) (Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.records[id]
	if !ok {
		return Connection{}, ErrConnectionNotFound
	}
	return c, nil
}

func (m *memoryRecords) CreateMCPConnection(_ context.Context, c Connection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.records == nil {
		m.records = map[string]Connection{}
	}
	if _, ok := m.records[c.ID]; ok {
		return ErrConnectionExists
	}
	m.records[c.ID] = c
	return nil
}

func (m *memoryRecords) SetMCPConnectionStatus(_ context.Context, id string, status Status, detail string, oauth *OAuthMetadata, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.records[id]
	if !ok {
		return ErrConnectionNotFound
	}
	c.Status, c.StatusDetail, c.UpdatedAt = status, detail, at
	if oauth != nil {
		c.OAuth = *oauth
	}
	m.records[id] = c
	return nil
}

// memorySecrets is an in-memory Secrets for Manager tests.
type memorySecrets struct {
	mu    sync.Mutex
	items map[string]Credentials
	saves int
	fail  error
}

func (m *memorySecrets) Save(_ context.Context, id string, c Credentials) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	if m.items == nil {
		m.items = map[string]Credentials{}
	}
	m.items[id] = c
	m.saves++
	return nil
}

func (m *memorySecrets) Load(_ context.Context, id string) (Credentials, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return Credentials{}, m.fail
	}
	c, ok := m.items[id]
	if !ok {
		return Credentials{}, ErrNoCredentials
	}
	return c, nil
}

func (m *memorySecrets) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	delete(m.items, id)
	return nil
}

func (m *memorySecrets) setFail(err error) {
	m.mu.Lock()
	m.fail = err
	m.mu.Unlock()
}

func (m *memorySecrets) get(id string) (Credentials, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.items[id]
	return c, ok
}
