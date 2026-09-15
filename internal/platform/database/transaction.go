package database

import (
	"context"
	"errors"
	"time"

	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrWriteOutcomeUnknown = errors.New("transaction commit outcome unknown")

// InTx gives every write, including the future audit writer, the same transaction.
// Callbacks must not commit or retain tx. There is deliberately no automatic retry.
func InTx(ctx context.Context, p *pgxpool.Pool, opts pgx.TxOptions, fn func(pgx.Tx, *dbsql.Queries) error) (err error) {
	tx, err := p.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		rollbackErr := tx.Rollback(cleanup)
		if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, rollbackErr)
		}
	}()
	if err = fn(tx, dbsql.New(p).WithTx(tx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) && !errors.Is(err, pgx.ErrTxCommitRollback) {
			return errors.Join(ErrWriteOutcomeUnknown, err)
		}
	}
	return err
}
