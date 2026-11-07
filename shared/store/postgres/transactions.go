package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type (
	contextKey string

	TransactionManager struct {
		db *pgxpool.Pool
	}

	Querier interface {
		Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
		Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
		QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	}
)

const txKey contextKey = "tx"

func NewTransactionManager(db *pgxpool.Pool) *TransactionManager {
	return &TransactionManager{db: db}
}

func (tm *TransactionManager) WithTransaction(
	ctx context.Context,
	fn func(ctx context.Context) error,
) error {
	return tm.withTransaction(ctx, pgx.TxOptions{}, fn)
}

// WithReadOnlySnapshot runs fn in a read-only REPEATABLE READ transaction, so
// every query in it sees the same committed state - for reads that must agree
// with each other. Called inside a transaction, it joins that one instead.
func (tm *TransactionManager) WithReadOnlySnapshot(
	ctx context.Context,
	fn func(ctx context.Context) error,
) error {
	return tm.withTransaction(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, fn)
}

func (tm *TransactionManager) withTransaction(
	ctx context.Context,
	opts pgx.TxOptions,
	fn func(ctx context.Context) error,
) error {
	if _, ok := ctx.Value(txKey).(pgx.Tx); ok {
		return fn(ctx)
	}

	tx, err := tm.db.BeginTx(ctx, opts)
	if err != nil {
		return err
	}

	ctx = context.WithValue(ctx, txKey, tx)

	err = fn(ctx)
	if err != nil {
		rbErr := tx.Rollback(ctx)
		if rbErr != nil {
			return fmt.Errorf("failed to rollback transaction: %w (originall error %w)", rbErr, err)
		}
		return err
	}

	return tx.Commit(ctx)
}

func (tm *TransactionManager) Querier(ctx context.Context) Querier {
	tx, ok := ctx.Value(txKey).(pgx.Tx)
	if ok {
		return tx
	}
	return tm.db
}
