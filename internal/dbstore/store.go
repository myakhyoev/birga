package dbstore

import (
	"context"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type contextKey string

const (
	txContextKey contextKey = "TX_CONTEXT_KEY"
)

func New(db *pgxpool.Pool) *DBStore {
	s := &DBStore{db: db}

	s.activityRepo = &activityRepo{store: s}
	s.userRepo = &userRepo{store: s}
	s.mediaRepo = &mediaRepo{store: s}

	return s
}

type DBStore struct {
	db *pgxpool.Pool

	activityRepo *activityRepo
	userRepo     *userRepo
	mediaRepo    *mediaRepo
}

func (s *DBStore) Activity() *activityRepo {
	return s.activityRepo
}

func (s *DBStore) User() *userRepo {
	return s.userRepo
}

func (s *DBStore) Media() *mediaRepo {
	return s.mediaRepo
}

// Ping checks database connectivity (used by the health endpoint).
func (s *DBStore) Ping(ctx context.Context) error {
	return errs.Wrap(s.db.Ping(ctx))
}

// InTx runs h inside one transaction. Repositories called with the ctx that h
// receives take part in it automatically.
func (s *DBStore) InTx(ctx context.Context, h func(context.Context) error) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:       pgx.ReadCommitted,
		AccessMode:     pgx.ReadWrite,
		DeferrableMode: "",
		BeginQuery:     "",
		CommitQuery:    "",
	})
	if err != nil {
		return errs.Wrap(err)
	}

	defer func() {
		_ = tx.Rollback(context.WithoutCancel(ctx))
	}()

	if err := h(context.WithValue(ctx, txContextKey, tx)); err != nil {
		return err
	}

	return errs.Wrap(tx.Commit(ctx))
}

func (s *DBStore) sqlClientByCtx(ctx context.Context) sqlClient {
	if ctx == nil {
		return s.db
	}

	if tx, ok := ctx.Value(txContextKey).(pgx.Tx); ok {
		return tx
	}

	return s.db
}

// sqlClientProvider - returns valid sqlClient interface
// One of *pgxpool.Pool, pgx.Tx
type sqlClientProvider interface {
	sqlClientByCtx(ctx context.Context) sqlClient
}

// sqlClient - common interface for *pgxpool.Pool and pgx.Tx
type sqlClient interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}
