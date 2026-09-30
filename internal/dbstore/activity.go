package dbstore

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type activityRepo struct {
	store sqlClientProvider
}

type dbActivity struct {
	ID              string    `db:"id"`
	TitleUz         string    `db:"title_uz"`
	TitleRu         string    `db:"title_ru"`
	DescriptionUz   string    `db:"description_uz"`
	DescriptionRu   string    `db:"description_ru"`
	Goal            string    `db:"goal"`
	MinAge          int16     `db:"min_age"`
	MaxAge          int16     `db:"max_age"`
	DurationMinutes int16     `db:"duration_minutes"`
	IsPublished     bool      `db:"is_published"`
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`
}

const activityColumns = `
	id, title_uz, title_ru, description_uz, description_ru, goal,
	min_age, max_age, duration_minutes, is_published, created_at, updated_at`

func (r *activityRepo) Create(ctx context.Context, a domain.Activity) (domain.Activity, error) {
	l := logger.FromCtx(ctx, "activityRepo.Create")

	var (
		sqlClient = r.store.sqlClientByCtx(ctx)
		q         = `
		INSERT INTO activities (
			title_uz, title_ru, description_uz, description_ru, goal,
			min_age, max_age, duration_minutes, is_published
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING ` + activityColumns
	)

	rows, err := sqlClient.Query(ctx, q,
		a.TitleUz, a.TitleRu, a.DescriptionUz, a.DescriptionRu, a.Goal,
		a.MinAge, a.MaxAge, a.DurationMinutes, a.IsPublished,
	)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return domain.Activity{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	created, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbActivity])
	if err != nil {
		l.Error("pgx.CollectExactlyOneRow", zap.Error(err))

		return domain.Activity{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return created.toDomain(), nil
}

func (r *activityRepo) Get(ctx context.Context, id string) (domain.Activity, error) {
	l := logger.FromCtx(ctx, "activityRepo.Get").With(zap.String("id", id))

	var (
		sqlClient = r.store.sqlClientByCtx(ctx)
		q         = `SELECT ` + activityColumns + ` FROM activities WHERE id = $1`
	)

	rows, err := sqlClient.Query(ctx, q, id)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return domain.Activity{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	a, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbActivity])
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Activity{}, errs.ErrActivityNotFound
	}

	if err != nil {
		l.Error("pgx.CollectExactlyOneRow", zap.Error(err))

		return domain.Activity{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return a.toDomain(), nil
}

// List returns one page of activities matching f and the total number of matches.
func (r *activityRepo) List(ctx context.Context, f domain.ActivityFilter) ([]domain.Activity, int, error) {
	l := logger.FromCtx(ctx, "activityRepo.List")

	var (
		sqlClient   = r.store.sqlClientByCtx(ctx)
		where, args = activityWhere(f)
		countQ      = `SELECT COUNT(*) FROM activities` + where
		listQ       = `SELECT ` + activityColumns + ` FROM activities` + where +
			` ORDER BY created_at DESC, id` +
			` LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2) //nolint:mnd
	)

	var total int
	if err := sqlClient.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err), zap.String("query", countQ))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	rows, err := sqlClient.Query(ctx, listQ, append(args, f.Limit, f.Offset)...)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err), zap.String("query", listQ))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbActivity])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.Activity, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, dbRows[i].toDomain())
	}

	return out, total, nil
}

// activityWhere builds a parameterized WHERE clause; values never enter the SQL text.
func activityWhere(f domain.ActivityFilter) (string, []any) {
	var (
		conds []string
		args  []any
	)

	if f.PublishedOnly {
		conds = append(conds, "is_published")
	}

	if f.Goal != "" {
		args = append(args, f.Goal)
		conds = append(conds, "goal = $"+strconv.Itoa(len(args)))
	}

	if f.Age > 0 {
		args = append(args, f.Age)
		n := strconv.Itoa(len(args))
		conds = append(conds, "min_age <= $"+n+" AND max_age >= $"+n)
	}

	if len(conds) == 0 {
		return "", args
	}

	return " WHERE " + strings.Join(conds, " AND "), args
}

func (a dbActivity) toDomain() domain.Activity {
	return domain.Activity{
		ID:              a.ID,
		TitleUz:         a.TitleUz,
		TitleRu:         a.TitleRu,
		DescriptionUz:   a.DescriptionUz,
		DescriptionRu:   a.DescriptionRu,
		Goal:            a.Goal,
		MinAge:          int(a.MinAge),
		MaxAge:          int(a.MaxAge),
		DurationMinutes: int(a.DurationMinutes),
		IsPublished:     a.IsPublished,
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
	}
}
