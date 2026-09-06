package eviedb

import (
 "context"
 "database/sql"
 "errors"
 "time"
)

// connectSQLiteStartup retries only physical connection initialization. No
// application statement has run yet, so an uncertain schema or operation is
// never replayed by this loop. SQLite can return BUSY immediately while two
// processes both enable WAL on a fresh database despite the busy timeout.
func connectSQLiteStartup(ctx context.Context, db *sql.DB) error {
 deadline := time.Now().Add(5 * time.Second)
 for {
  if err := ctx.Err(); err != nil { return err }
  err := db.PingContext(ctx)
  if err == nil { return nil }
  if canceled := ctx.Err(); canceled != nil { return errors.Join(canceled, err) }
  var coded interface{ Code() int }
  if !errors.As(err, &coded) || coded.Code()&255 != 5 { return err }
  remaining := time.Until(deadline)
  if remaining <= 0 { return err }
  delay := 10*time.Millisecond
  if remaining < delay { delay=remaining }
  timer:=time.NewTimer(delay)
  select {
  case <-ctx.Done(): timer.Stop(); return errors.Join(ctx.Err(),err)
  case <-timer.C:
  }
 }
}
