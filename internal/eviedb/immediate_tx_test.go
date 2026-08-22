package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestImmediateTransactionCancellationBeforeCommitRollsBack(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(`CREATE TABLE immediate_tx_probe (value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create transaction probe: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	err := withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, `INSERT INTO immediate_tx_probe (value) VALUES ('uncommitted')`); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("transaction error = %v, want context.Canceled", err)
	}

	var writes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM immediate_tx_probe`).Scan(&writes); err != nil {
		t.Fatalf("count transaction writes: %v", err)
	}
	if writes != 0 {
		t.Fatalf("writes = %d, want cancellation rollback", writes)
	}
}
