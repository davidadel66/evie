package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (s *Store) PluginEnabled(ctx context.Context, pluginID string) (bool, uint64, bool, error) {
	if strings.TrimSpace(pluginID) == "" {
		return false, 0, false, errors.New("plugin ID must not be empty")
	}
	var enabled bool
	var revision uint64
	err := s.db.QueryRowContext(ctx, `
		SELECT enabled, revision FROM plugin_enabled_configuration WHERE plugin_id = ?
	`, pluginID).Scan(&enabled, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, false, nil
	}
	if err != nil {
		return false, 0, false, fmt.Errorf("read plugin enabled configuration for %q: %w", pluginID, err)
	}
	return enabled, revision, true, nil
}

// ResolvePluginEnabled seeds a compiled plugin's default exactly once and
// returns the durable desired value. ON CONFLICT preserves an owner's prior
// choice across upgrades and concurrent process startup.
func (s *Store) ResolvePluginEnabled(ctx context.Context, pluginID string, defaultEnabled bool) (bool, uint64, error) {
	if strings.TrimSpace(pluginID) == "" {
		return false, 0, errors.New("plugin ID must not be empty")
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO plugin_enabled_configuration (plugin_id, enabled, revision, updated_at)
		VALUES (?, ?, 1, ?)
		ON CONFLICT (plugin_id) DO NOTHING
	`, pluginID, defaultEnabled, s.now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")); err != nil {
		return false, 0, fmt.Errorf("seed plugin enabled configuration for %q: %w", pluginID, err)
	}
	var enabled bool
	var revision uint64
	if err := s.db.QueryRowContext(ctx, `
		SELECT enabled, revision FROM plugin_enabled_configuration WHERE plugin_id = ?
	`, pluginID).Scan(&enabled, &revision); err != nil {
		return false, 0, fmt.Errorf("read plugin enabled configuration for %q: %w", pluginID, err)
	}
	return enabled, revision, nil
}

// SetPluginEnabled durably records owner intent before the Manager applies a
// lifecycle transition and increments its command revision even when the value
// is unchanged. A failed cleanup therefore cannot undo a disable, and a failed
// start remains a durable enable/retry request for every attached Manager.
func (s *Store) SetPluginEnabled(ctx context.Context, pluginID string, enabled bool) (uint64, error) {
	if strings.TrimSpace(pluginID) == "" {
		return 0, errors.New("plugin ID must not be empty")
	}
	var revision uint64
	if err := s.db.QueryRowContext(ctx, `
		INSERT INTO plugin_enabled_configuration (plugin_id, enabled, revision, updated_at)
		VALUES (?, ?, 1, ?)
		ON CONFLICT (plugin_id) DO UPDATE SET
			enabled = excluded.enabled,
			revision = plugin_enabled_configuration.revision + 1,
			updated_at = excluded.updated_at
		RETURNING revision
	`, pluginID, enabled, s.now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")).Scan(&revision); err != nil {
		return 0, fmt.Errorf("write plugin enabled configuration for %q: %w", pluginID, err)
	}
	return revision, nil
}
