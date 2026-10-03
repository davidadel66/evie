package eviedb

import (
	"context"
	"database/sql"
	"time"
)

type Store struct {
	db  *sql.DB
	now func() time.Time
	// location is the owner's time zone: the process zone in production. It
	// resolves calendar words such as "yesterday" in recall requests.
	location                    *time.Location
	resolveImmediateTransaction immediateTransactionResolver
	newResolutionContext        immediateTransactionContextFactory
	semanticMaintenance         semanticMaintenanceHooks
	afterTaskTreeRead           func()
	afterLegacyTodoItem         func()
}

type semanticMaintenanceHooks struct {
	afterLock              func() error
	beforeShadowValidation func(*sql.DB) error
	beforeSwap             func() error
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		db:                          db,
		now:                         time.Now,
		location:                    time.Local,
		resolveImmediateTransaction: executeImmediateTransactionStatement,
		newResolutionContext:        transactionResolutionContext,
	}
}

// UseClock replaces the Store's clock and the owner's time zone before the
// Store is shared. Production keeps the wall clock and the process zone; a
// deterministic evaluation uses it to replay history on a synthetic timeline,
// so event, lease and memory times all come from one clock.
func (s *Store) UseClock(now func() time.Time, location *time.Location) {
	if now == nil {
		now = time.Now
	}
	if location == nil {
		location = time.Local
	}
	s.now, s.location = now, location
}

func (s *Store) withImmediateTransaction(
	ctx context.Context,
	operation func(*sql.Conn) error,
) error {
	return withImmediateTransactionResolver(
		ctx,
		s.db,
		s.resolveImmediateTransaction,
		s.newResolutionContext,
		operation,
	)
}
