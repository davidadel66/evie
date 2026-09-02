package eviedb

import (
	"context"
	"database/sql"
	"time"
)

type Store struct {
	db                          *sql.DB
	now                         func() time.Time
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
		resolveImmediateTransaction: executeImmediateTransactionStatement,
		newResolutionContext:        transactionResolutionContext,
	}
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
