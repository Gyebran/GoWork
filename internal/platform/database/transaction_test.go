package database

import (
	"context"
	"errors"
	dbsql "github.com/Gyebran/GoWork/internal/platform/database/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"io"
	"testing"
)

type fakeBeginner struct {
	tx     *fakeTx
	begins int
}

func (b *fakeBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	b.begins++
	return b.tx, nil
}

type fakeTx struct {
	pgx.Tx
	commitErr          error
	commits, rollbacks int
	cleanupErr         error
}

func (t *fakeTx) Commit(context.Context) error { t.commits++; return t.commitErr }
func (t *fakeTx) Rollback(ctx context.Context) error {
	t.rollbacks++
	t.cleanupErr = ctx.Err()
	return pgx.ErrTxClosed
}
func TestCommitOutcomeAndNoReplay(t *testing.T) {
	for _, c := range []struct {
		name    string
		mode    pgx.TxAccessMode
		err     error
		unknown bool
	}{{"write network loss", "", io.EOF, true}, {"server rejected commit", "", &pgconn.PgError{Code: "40001"}, false}, {"aborted transaction", "", pgx.ErrTxCommitRollback, false}, {"read network loss", pgx.ReadOnly, io.EOF, false}, {"success", "", nil, false}} {
		t.Run(c.name, func(t *testing.T) {
			tx := &fakeTx{commitErr: c.err}
			b := &fakeBeginner{tx: tx}
			calls := 0
			err := InTx(context.Background(), b, pgx.TxOptions{AccessMode: c.mode}, func(pgx.Tx, *dbsql.Queries) error { calls++; return nil })
			if errors.Is(err, ErrWriteOutcomeUnknown) != c.unknown || !errors.Is(err, c.err) {
				t.Fatal(err)
			}
			if calls != 1 || b.begins != 1 || tx.commits != 1 || tx.rollbacks != 1 {
				t.Fatal("transaction replayed or cleanup missing")
			}
		})
	}
}
func TestCallbackFailureAndPanicCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tx := &fakeTx{}
	b := &fakeBeginner{tx: tx}
	cause := errors.New("audit failed")
	err := InTx(ctx, b, pgx.TxOptions{}, func(pgx.Tx, *dbsql.Queries) error { cancel(); return cause })
	if !errors.Is(err, cause) || tx.commits != 0 || tx.rollbacks != 1 || tx.cleanupErr != nil {
		t.Fatal("cancelled transaction not cleaned up", err)
	}
	tx = &fakeTx{}
	b = &fakeBeginner{tx: tx}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic swallowed")
			}
		}()
		_ = InTx(context.Background(), b, pgx.TxOptions{}, func(pgx.Tx, *dbsql.Queries) error { panic("test panic") })
	}()
	if tx.commits != 0 || tx.rollbacks != 1 {
		t.Fatal("panic did not roll back")
	}
}
