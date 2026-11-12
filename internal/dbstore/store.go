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

// setUpdatedAt starts every partial-update SET clause.
const setUpdatedAt = "updated_at = NOW()"

func New(db *pgxpool.Pool) *DBStore {
	s := &DBStore{db: db}

	s.activityRepo = &activityRepo{store: s}
	s.userRepo = &userRepo{store: s}
	s.mediaRepo = &mediaRepo{store: s}
	s.authRepo = &authRepo{store: s}
	s.childRepo = &childRepo{store: s}
	s.completionRepo = &completionRepo{store: s}
	s.goalRepo = &goalRepo{store: s}
	s.planRepo = &planRepo{store: s}
	s.articleRepo = &articleRepo{store: s}
	s.postRepo = &postRepo{store: s}
	s.topicRepo = &topicRepo{store: s}
	s.tipRepo = &tipRepo{store: s}
	s.extraRepo = &extraRepo{store: s}
	s.milestoneRepo = &milestoneRepo{store: s}

	return s
}

type DBStore struct {
	db *pgxpool.Pool

	activityRepo *activityRepo
	userRepo     *userRepo
	mediaRepo    *mediaRepo
	authRepo     *authRepo
	childRepo    *childRepo

	completionRepo *completionRepo
	goalRepo       *goalRepo
	planRepo       *planRepo
	articleRepo    *articleRepo
	postRepo       *postRepo
	topicRepo      *topicRepo
	tipRepo        *tipRepo
	extraRepo      *extraRepo
	milestoneRepo  *milestoneRepo
}

// Column names shared by several update builders.
const (
	colGoalID = "goal_id"
	colStatus = "status"
	colSource = "source"
)

func (s *DBStore) Activity() *activityRepo {
	return s.activityRepo
}

func (s *DBStore) User() *userRepo {
	return s.userRepo
}

func (s *DBStore) Media() *mediaRepo {
	return s.mediaRepo
}

func (s *DBStore) Auth() *authRepo {
	return s.authRepo
}

func (s *DBStore) Child() *childRepo {
	return s.childRepo
}

func (s *DBStore) Completion() *completionRepo {
	return s.completionRepo
}

func (s *DBStore) Goal() *goalRepo {
	return s.goalRepo
}

func (s *DBStore) Plan() *planRepo {
	return s.planRepo
}

func (s *DBStore) Article() *articleRepo {
	return s.articleRepo
}

func (s *DBStore) Post() *postRepo {
	return s.postRepo
}

func (s *DBStore) Topic() *topicRepo {
	return s.topicRepo
}

func (s *DBStore) Tip() *tipRepo {
	return s.tipRepo
}

func (s *DBStore) Extra() *extraRepo {
	return s.extraRepo
}

func (s *DBStore) SeedPhoto() *seedPhotoRepo {
	return &seedPhotoRepo{store: s}
}

func (s *DBStore) Milestone() *milestoneRepo {
	return s.milestoneRepo
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
