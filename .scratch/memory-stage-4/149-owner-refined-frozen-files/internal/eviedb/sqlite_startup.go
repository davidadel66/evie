package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// connectSQLiteStartup retries only physical connection initialization, before
// the first application statement. Concurrent fresh processes can receive BUSY
// while enabling WAL despite busy_timeout. Schema migrations and uncertain
// application writes are never replayed by this loop.
func connectSQLiteStartup(ctx context.Context, db *sql.DB) error {
	return connectSQLiteStartupWithin(ctx, db.PingContext, 5*time.Second, 10*time.Millisecond)
}

// The deadline bounds retry admission and is passed to PingContext. The pinned
// SQLite driver initializes connection PRAGMAs without a context; a connection
// already inside that driver call remains subject to its existing 5s SQLite
// busy timeout. Do not describe this loop as interrupting Driver.Open itself.
func connectSQLiteStartupWithin(ctx context.Context, ping func(context.Context) error, budget, delay time.Duration) error {
	retryCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	var lastBusy error
	for {
		if err := retryCtx.Err(); err != nil {
			return errors.Join(err, lastBusy)
		}
		err := ping(retryCtx)
		if stopped := retryCtx.Err(); stopped != nil {
			return errors.Join(stopped, err, lastBusy)
		}
		if err == nil {
			return nil
		}
		var coded interface{ Code() int }
		if !errors.As(err, &coded) || coded.Code()&255 != 5 {
			return err
		}
		lastBusy = err
		timer := time.NewTimer(delay)
		select {
		case <-retryCtx.Done():
			timer.Stop()
			return errors.Join(retryCtx.Err(), lastBusy)
		case <-timer.C:
		}
	}
}
