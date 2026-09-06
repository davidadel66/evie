package eviedb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

// ErrCompilerStatusIndexing reports incomplete exact totals, never a capped or
// estimated count. Repeating the authorized status request advances a persisted
// bounded cursor. Small sessions complete in their first request.
var ErrCompilerStatusIndexing = errors.New("compiler status totals are indexing; repeat status to continue bounded indexing")

func ensureCompilerStatusProjectionSchema(ctx context.Context, db *sql.DB) error {
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(ctx, compilerStatusProjectionSchema)
		return err
	})
}

// Startup creates indexes but never reconstructs retained totals. Each authorized
// request visits at most 128 event coordinates and 32 root records for indexing. History union
// changes invalidate only the exact generation/destination/session cache; their
// existing <=10,000-event owner transaction maintains that revision atomically.
// Cancellation preserves the ever-selected union and therefore does not reset
// it. Live activation boundaries only select future append positions, so normal
// appends extend existing cursors rather than restarting them.
const compilerStatusProjectionSchema = `
CREATE INDEX IF NOT EXISTS memory_compiler_status_selection_interval ON memory_compiler_selections(generation_id,destination,session_id,root_id,first_sequence DESC);
CREATE INDEX IF NOT EXISTS memory_compiler_status_activation_list ON memory_compiler_activations(source_scope,source_session,revision DESC,activation_id);
CREATE INDEX IF NOT EXISTS memory_compiler_status_activation_interval ON memory_compiler_activations(source_scope,source_session,destination,after_position DESC,(through_position IS NULL) DESC,through_position DESC);
CREATE INDEX IF NOT EXISTS memory_compiler_status_activation_generation_interval ON memory_compiler_activations(generation_id,source_scope,source_session,destination,after_position DESC,(through_position IS NULL) DESC,through_position DESC);
CREATE INDEX IF NOT EXISTS memory_compiler_status_root_list ON memory_compiler_activation_roots(session_id,position DESC,activation_id);
CREATE INDEX IF NOT EXISTS memory_compiler_status_root_bootstrap ON memory_compiler_activation_roots(session_id,activation_id,root_id);
CREATE TABLE IF NOT EXISTS memory_compiler_status_history_revision (
 generation_id TEXT NOT NULL,destination TEXT NOT NULL,session_id TEXT NOT NULL,
 revision INTEGER NOT NULL,PRIMARY KEY(generation_id,destination,session_id)
);
CREATE TRIGGER IF NOT EXISTS memory_compiler_status_history_insert AFTER INSERT ON memory_compiler_history_selection_refs BEGIN
 INSERT INTO memory_compiler_status_history_revision VALUES(NEW.generation_id,NEW.destination,NEW.session_id,1)
 ON CONFLICT(generation_id,destination,session_id) DO UPDATE SET revision=revision+1;
END;
CREATE TABLE IF NOT EXISTS memory_compiler_status_event_revision (
 session_id TEXT PRIMARY KEY,revision INTEGER NOT NULL
);
CREATE TRIGGER IF NOT EXISTS memory_compiler_status_event_delete AFTER DELETE ON events BEGIN
 INSERT INTO memory_compiler_status_event_revision VALUES(OLD.session_id,1)
 ON CONFLICT(session_id) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_status_position_delete AFTER DELETE ON memory_compiler_event_positions BEGIN
 INSERT INTO memory_compiler_status_event_revision SELECT session_id,1 FROM events WHERE id=OLD.event_id
 ON CONFLICT(session_id) DO UPDATE SET revision=revision+1;
END;
CREATE TABLE IF NOT EXISTS memory_compiler_status_events (
 source_scope TEXT NOT NULL,session_id TEXT NOT NULL,generation_id TEXT NOT NULL,destination TEXT NOT NULL,
 event_revision INTEGER NOT NULL,history_revision INTEGER NOT NULL,
 after_sequence INTEGER NOT NULL,total INTEGER NOT NULL,selected INTEGER NOT NULL,
 PRIMARY KEY(source_scope,session_id,generation_id,destination)
);
CREATE TABLE IF NOT EXISTS memory_compiler_status_roots (
 session_id TEXT PRIMARY KEY,after_activation TEXT NOT NULL DEFAULT '',after_root TEXT NOT NULL DEFAULT '',
 pending INTEGER NOT NULL DEFAULT 0,failed INTEGER NOT NULL DEFAULT 0
);
CREATE TRIGGER IF NOT EXISTS memory_compiler_status_root_insert AFTER INSERT ON memory_compiler_activation_roots BEGIN
 UPDATE memory_compiler_status_roots SET pending=pending+(NEW.state IN ('selected_unmaterialized','deferred_live')),failed=failed+(NEW.state='failed') WHERE session_id=NEW.session_id AND (after_activation,after_root)>=(NEW.activation_id,NEW.root_id);
END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_status_root_update AFTER UPDATE OF state ON memory_compiler_activation_roots BEGIN
 UPDATE memory_compiler_status_roots SET pending=pending+(NEW.state IN ('selected_unmaterialized','deferred_live'))-(OLD.state IN ('selected_unmaterialized','deferred_live')),failed=failed+(NEW.state='failed')-(OLD.state='failed') WHERE session_id=NEW.session_id AND (after_activation,after_root)>=(NEW.activation_id,NEW.root_id);
END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_status_root_delete AFTER DELETE ON memory_compiler_activation_roots BEGIN
 UPDATE memory_compiler_status_roots SET pending=pending-(OLD.state IN ('selected_unmaterialized','deferred_live')),failed=failed-(OLD.state='failed') WHERE session_id=OLD.session_id AND (after_activation,after_root)>=(OLD.activation_id,OLD.root_id);
END;
`

const compilerStatusEventPage = `SELECT e.id,e.sequence,p.commit_position FROM events e LEFT JOIN memory_compiler_event_positions p ON p.event_id=e.id WHERE e.session_id=? AND e.sequence>? ORDER BY e.sequence LIMIT 128`
const compilerStatusLivePredecessor = `SELECT through_position FROM memory_compiler_activations WHERE source_scope=? AND source_session=? AND destination=? AND after_position<? ORDER BY after_position DESC,(through_position IS NULL) DESC,through_position DESC LIMIT 1`
const compilerStatusGenerationPredecessor = `SELECT through_position FROM memory_compiler_activations WHERE generation_id=? AND source_scope=? AND source_session=? AND destination=? AND after_position<? ORDER BY after_position DESC,(through_position IS NULL) DESC,through_position DESC LIMIT 1`
const compilerStatusRootPage = `SELECT activation_id,root_id,state FROM memory_compiler_activation_roots WHERE session_id=? AND (activation_id,root_id)>(?,?) ORDER BY activation_id,root_id LIMIT 32`

// A blank generation/destination requests the legacy activation union. A pinned
// pair requests history plus live selection for that exact generation and scope.
func compilerStatusEventCounts(ctx context.Context, conn *sql.Conn, sourceScope string, session memory.SessionID, generation, destination string) (total, selected int64, ready bool, err error) {
	var eventRevision, historyRevision int64
	if err = conn.QueryRowContext(ctx, `SELECT COALESCE((SELECT revision FROM memory_compiler_status_event_revision WHERE session_id=?),0),COALESCE((SELECT revision FROM memory_compiler_status_history_revision WHERE generation_id=? AND destination=? AND session_id=?),0)`, session, generation, destination, session).Scan(&eventRevision, &historyRevision); err != nil {
		return
	}
	var after, previousEvent, previousHistory int64
	err = conn.QueryRowContext(ctx, `SELECT event_revision,history_revision,after_sequence,total,selected FROM memory_compiler_status_events WHERE source_scope=? AND session_id=? AND generation_id=? AND destination=?`, sourceScope, session, generation, destination).Scan(&previousEvent, &previousHistory, &after, &total, &selected)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	} else if err != nil {
		return
	}
	if previousEvent != eventRevision || previousHistory != historyRevision {
		after, total, selected = 0, 0, 0
	}
	var high int64
	if err = conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM events WHERE session_id=?`, session).Scan(&high); err != nil {
		return
	}
	rows, readErr := conn.QueryContext(ctx, compilerStatusEventPage, session, after)
	if readErr != nil {
		err = readErr
		return
	}
	type coordinate struct {
		id       string
		sequence int64
		position sql.NullInt64
	}
	var events []coordinate
	for rows.Next() {
		var e coordinate
		if err = rows.Scan(&e.id, &e.sequence, &e.position); err != nil {
			rows.Close()
			return
		}
		events = append(events, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return
	}
	for _, e := range events {
		var included bool
		if generation != "" {
			var exists int
			lookupErr := conn.QueryRowContext(ctx, `SELECT 1 FROM memory_compiler_history_selection_refs WHERE generation_id=? AND destination=? AND session_id=? AND event_id=?`, generation, destination, session, e.id).Scan(&exists)
			if lookupErr == nil {
				included = true
			} else if !errors.Is(lookupErr, sql.ErrNoRows) {
				err = lookupErr
				return
			}
		}
		if !included && e.position.Valid {
			destinations := []string{destination}
			if generation == "" {
				destinations = []string{sourceScope, "session:" + string(session)}
			}
			for _, dest := range destinations {
				for _, sourceSession := range []string{"", string(session)} {
					var through sql.NullInt64
					var lookupErr error
					if generation == "" {
						lookupErr = conn.QueryRowContext(ctx, compilerStatusLivePredecessor, sourceScope, sourceSession, dest, e.position.Int64).Scan(&through)
					} else {
						lookupErr = conn.QueryRowContext(ctx, compilerStatusGenerationPredecessor, generation, sourceScope, sourceSession, dest, e.position.Int64).Scan(&through)
					}
					if lookupErr == nil && (!through.Valid || e.position.Int64 <= through.Int64) {
						included = true
					}
					if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
						err = lookupErr
						return
					}
				}
			}
		}
		total++
		if included {
			selected++
		}
		after = e.sequence
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO memory_compiler_status_events VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(source_scope,session_id,generation_id,destination) DO UPDATE SET event_revision=excluded.event_revision,history_revision=excluded.history_revision,after_sequence=excluded.after_sequence,total=excluded.total,selected=excluded.selected`, sourceScope, session, generation, destination, eventRevision, historyRevision, after, total, selected)
	ready = after >= high
	return
}

func compilerStatusRootCounts(ctx context.Context, conn *sql.Conn, session memory.SessionID) (pending, failed int64, ready bool, err error) {
	if _, err = conn.ExecContext(ctx, `INSERT OR IGNORE INTO memory_compiler_status_roots(session_id) VALUES(?)`, session); err != nil {
		return
	}
	var afterActivation, afterRoot, highActivation, highRoot string
	if err = conn.QueryRowContext(ctx, `SELECT after_activation,after_root,pending,failed FROM memory_compiler_status_roots WHERE session_id=?`, session).Scan(&afterActivation, &afterRoot, &pending, &failed); err != nil {
		return
	}
	if err = conn.QueryRowContext(ctx, `SELECT activation_id,root_id FROM memory_compiler_activation_roots WHERE session_id=? ORDER BY activation_id DESC,root_id DESC LIMIT 1`, session).Scan(&highActivation, &highRoot); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return
	}
	rows, readErr := conn.QueryContext(ctx, compilerStatusRootPage, session, afterActivation, afterRoot)
	if readErr != nil {
		err = readErr
		return
	}
	for rows.Next() {
		var state string
		if err = rows.Scan(&afterActivation, &afterRoot, &state); err != nil {
			rows.Close()
			return
		}
		if state == "selected_unmaterialized" || state == "deferred_live" {
			pending++
		}
		if state == "failed" {
			failed++
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return
	}
	_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_status_roots SET after_activation=?,after_root=?,pending=?,failed=? WHERE session_id=?`, afterActivation, afterRoot, pending, failed, session)
	ready = afterActivation > highActivation || (afterActivation == highActivation && afterRoot >= highRoot)
	return
}
