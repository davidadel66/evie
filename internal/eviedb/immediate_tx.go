package eviedb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
)

type immediateTransactionResolver func(context.Context, *sql.Conn, string) (sql.Result, error)
type immediateTransactionContextFactory func(context.Context) (context.Context, context.CancelFunc)

func executeImmediateTransactionStatement(
	ctx context.Context,
	conn *sql.Conn,
	statement string,
) (sql.Result, error) {
	return conn.ExecContext(ctx, statement)
}

// transactionResolutionContext ignores an already-selected caller cancellation
// while preserving the cleanup attempt's deadline. Terminal evidence and lease
// release use independent bounded contexts, so COMMIT and the explicit rollback
// attempt must not silently become unbounded during transaction resolution.
func transactionResolutionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	detached := context.WithoutCancel(ctx)
	deadline, ok := ctx.Deadline()
	if !ok {
		return detached, func() {}
	}
	return context.WithDeadline(detached, deadline)
}

// discardImmediateTransactionConnection prevents an unresolved raw BEGIN from
// returning to the pool after the bounded rollback attempt fails. The driver
// closes the bad connection, which rolls back any transaction that did not
// reach a durable commit; callers still receive the original uncertain error.
func discardImmediateTransactionConnection(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
}

func withImmediateTransaction(
	ctx context.Context,
	db *sql.DB,
	operation func(*sql.Conn) error,
) (err error) {
	return withImmediateTransactionResolver(
		ctx,
		db,
		executeImmediateTransactionStatement,
		transactionResolutionContext,
		operation,
	)
}

func withImmediateTransactionResolver(
	ctx context.Context,
	db *sql.DB,
	resolve immediateTransactionResolver,
	resolutionContext immediateTransactionContextFactory,
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
		rollbackCtx, cancel := resolutionContext(ctx)
		defer cancel()
		if _, rollbackErr := resolve(rollbackCtx, conn, `ROLLBACK`); rollbackErr != nil {
			discardImmediateTransactionConnection(conn)
			if err == nil {
				err = fmt.Errorf("rollback immediate transaction: %w", rollbackErr)
			}
		}
	}()

	if err := operation(conn); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	commitCtx, cancel := resolutionContext(ctx)
	defer cancel()
	if _, err := resolve(commitCtx, conn, `COMMIT`); err != nil {
		return fmt.Errorf("commit immediate transaction: %w", err)
	}
	committed = true
	return nil
}
