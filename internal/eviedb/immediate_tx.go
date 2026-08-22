package eviedb

import (
	"context"
	"database/sql"
	"fmt"
)

func withImmediateTransaction(
	ctx context.Context,
	db *sql.DB,
	operation func(*sql.Conn) error,
) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open immediate transaction connection: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close immediate transaction connection: %w", closeErr)
		}
	}()

	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return fmt.Errorf("begin immediate transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if _, rollbackErr := conn.ExecContext(context.WithoutCancel(ctx), `ROLLBACK`); err == nil && rollbackErr != nil {
			err = fmt.Errorf("rollback immediate transaction: %w", rollbackErr)
		}
	}()

	if err := operation(conn); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := conn.ExecContext(context.WithoutCancel(ctx), `COMMIT`); err != nil {
		return fmt.Errorf("commit immediate transaction: %w", err)
	}
	committed = true
	return nil
}
