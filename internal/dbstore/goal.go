package dbstore

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// Every query filters on deleted_at IS NULL: a soft-deleted goal is invisible.
type goalRepo struct {
	store sqlClientProvider
}

type dbGoal struct {
	ID        string    `db:"id"`
	NameUz    string    `db:"name_uz"`
	NameRu    string    `db:"name_ru"`
	NameEn    string    `db:"name_en"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

const goalColumns = `id, name_uz, name_ru, name_en, created_at, updated_at`

func (r *goalRepo) Create(ctx context.Context, g domain.Goal) (domain.Goal, error) {
	l := logger.FromCtx(ctx, "goalRepo.Create")

	q := `INSERT INTO goals (name_uz, name_ru, name_en) VALUES ($1, $2, $3) RETURNING ` + goalColumns

	return r.one(ctx, l, q, g.NameUz, g.NameRu, g.NameEn)
}

func (r *goalRepo) Get(ctx context.Context, id string) (domain.Goal, error) {
	l := logger.FromCtx(ctx, "goalRepo.Get").With(zap.String("id", id))

	return r.one(ctx, l, `SELECT `+goalColumns+` FROM goals WHERE id = $1 AND deleted_at IS NULL`, id)
}

// List returns every active goal, oldest first. The list is short, so it is not paginated.
func (r *goalRepo) List(ctx context.Context) ([]domain.Goal, error) {
	l := logger.FromCtx(ctx, "goalRepo.List")

	q := `SELECT ` + goalColumns + ` FROM goals WHERE deleted_at IS NULL ORDER BY created_at, id`

	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbGoal])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.Goal, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, dbRows[i].toDomain())
	}

	return out, nil
}

// CountActive returns how many of ids are active goals. ids must not repeat.
func (r *goalRepo) CountActive(ctx context.Context, ids []string) (int, error) {
	l := logger.FromCtx(ctx, "goalRepo.CountActive")

	var n int

	q := `SELECT COUNT(*) FROM goals WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL`
	if err := r.store.sqlClientByCtx(ctx).QueryRow(ctx, q, ids).Scan(&n); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return n, nil
}

// Update applies the non-nil fields of upd.
func (r *goalRepo) Update(ctx context.Context, id string, upd domain.GoalUpdate) (domain.Goal, error) {
	l := logger.FromCtx(ctx, "goalRepo.Update").With(zap.String("id", id))

	var (
		sets, args = goalSet(upd)
		q          = `UPDATE goals SET ` + sets +
			` WHERE id = $` + strconv.Itoa(len(args)+1) + ` AND deleted_at IS NULL RETURNING ` + goalColumns
	)

	return r.one(ctx, l, q, append(args, id)...)
}

// Delete soft-deletes the goal and removes its id from every user's goal_ids in the same statement,
// since users.goal_ids cannot carry a foreign key.
func (r *goalRepo) Delete(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx, "goalRepo.Delete").With(zap.String("id", id))

	q := `
		WITH deleted AS (
			UPDATE goals SET deleted_at = NOW(), updated_at = NOW()
			WHERE id = $1 AND deleted_at IS NULL
			RETURNING id
		), unlinked AS (
			UPDATE users SET goal_ids = array_remove(goal_ids, deleted.id), updated_at = NOW()
			FROM deleted WHERE deleted.id = ANY(users.goal_ids)
		)
		SELECT COUNT(*) FROM deleted`

	var n int
	if err := r.store.sqlClientByCtx(ctx).QueryRow(ctx, q, id).Scan(&n); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	if n == 0 {
		return errs.ErrGoalNotFound
	}

	return nil
}

// one runs q, which returns at most one goal row (SELECT, or a write with RETURNING). No row means
// ErrGoalNotFound; a unique-name violation means ErrGoalNameTaken.
func (r *goalRepo) one(ctx context.Context, l *zap.Logger, q string, args ...any) (domain.Goal, error) {
	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, args...)
	if err == nil {
		var g dbGoal

		g, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbGoal])
		if err == nil {
			return g.toDomain(), nil
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Goal{}, errs.ErrGoalNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return domain.Goal{}, errs.Errf(errs.ErrGoalNameTaken, "%s: %s",
			errs.ErrGoalNameTaken.Error(), strings.TrimSuffix(strings.TrimPrefix(pgErr.ConstraintName, "goals_"), "_uniq"))
	}

	l.Error("goalRepo query failed", zap.Error(err), zap.String("query", q))

	return domain.Goal{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
}

// goalSet builds a parameterized SET clause from fixed fragments; values never enter the SQL text.
func goalSet(upd domain.GoalUpdate) (string, []any) {
	var (
		sets = []string{setUpdatedAt}
		args []any
	)

	for _, f := range []struct {
		column string
		value  *string
	}{
		{"name_uz", upd.NameUz},
		{"name_ru", upd.NameRu},
		{"name_en", upd.NameEn},
	} {
		if f.value == nil {
			continue
		}

		args = append(args, *f.value)
		sets = append(sets, f.column+" = $"+strconv.Itoa(len(args)))
	}

	return strings.Join(sets, ", "), args
}

func (g dbGoal) toDomain() domain.Goal {
	return domain.Goal{
		ID:        g.ID,
		NameUz:    g.NameUz,
		NameRu:    g.NameRu,
		NameEn:    g.NameEn,
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
	}
}
