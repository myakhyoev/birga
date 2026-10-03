package dbstore

import (
	"context"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type completionRepo struct {
	store sqlClientProvider
}

type dbCompletion struct {
	ID          string    `db:"id"`
	ChildID     string    `db:"child_id"`
	ActivityID  string    `db:"activity_id"`
	UserID      *string   `db:"user_id"`
	CompletedOn time.Time `db:"completed_on"`
	Note        *string   `db:"note"`
	CreatedAt   time.Time `db:"created_at"`
}

const completionColumns = `id, child_id, activity_id, user_id, completed_on, note, created_at`

// Create records a completion. If the child already did this activity on that day, the existing row
// is returned, with its note replaced when c.Note is set, and created reports false.
func (r *completionRepo) Create(ctx context.Context, c domain.Completion) (domain.Completion, bool, error) {
	l := logger.FromCtx(ctx, "completionRepo.Create").With(zap.String("child_id", c.ChildID))

	q := `
		INSERT INTO activity_completions (child_id, activity_id, user_id, completed_on, note)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (child_id, activity_id, completed_on)
		DO UPDATE SET note = COALESCE(EXCLUDED.note, activity_completions.note)
		RETURNING ` + completionColumns + `, (xmax = 0) AS created`

	var (
		row     dbCompletion
		created bool
	)

	err := r.store.sqlClientByCtx(ctx).QueryRow(ctx, q,
		c.ChildID, c.ActivityID, c.UserID, c.CompletedOn.Format(time.DateOnly), c.Note,
	).Scan(&row.ID, &row.ChildID, &row.ActivityID, &row.UserID, &row.CompletedOn, &row.Note, &row.CreatedAt, &created)
	if err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return domain.Completion{}, false, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return row.toDomain(), created, nil
}

// List returns one page of a child's completions, newest first, and the total count.
func (r *completionRepo) List(ctx context.Context, f domain.CompletionFilter) ([]domain.Completion, int, error) {
	l := logger.FromCtx(ctx, "completionRepo.List").With(zap.String("child_id", f.ChildID))

	sqlClient := r.store.sqlClientByCtx(ctx)

	var total int
	if err := sqlClient.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_completions WHERE child_id = $1`, f.ChildID).Scan(&total); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	rows, err := sqlClient.Query(ctx, `SELECT `+completionColumns+` FROM activity_completions
		WHERE child_id = $1 ORDER BY completed_on DESC, created_at DESC, id LIMIT $2 OFFSET $3`,
		f.ChildID, f.Limit, f.Offset)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbCompletion])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.Completion, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, dbRows[i].toDomain())
	}

	return out, total, nil
}

// Days returns the distinct days on which the child completed anything, newest first, and the
// total number of completions.
func (r *completionRepo) Days(ctx context.Context, childID string) ([]time.Time, int, error) {
	l := logger.FromCtx(ctx, "completionRepo.Days").With(zap.String("child_id", childID))

	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, `
		SELECT completed_on, COUNT(*) FROM activity_completions
		WHERE child_id = $1 GROUP BY completed_on ORDER BY completed_on DESC`, childID)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	var (
		days  []time.Time
		total int
		day   time.Time
		count int
	)

	if _, err := pgx.ForEachRow(rows, []any{&day, &count}, func() error {
		days = append(days, day)
		total += count

		return nil
	}); err != nil {
		l.Error("pgx.ForEachRow", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return days, total, nil
}

func (c dbCompletion) toDomain() domain.Completion {
	return domain.Completion{
		ID:          c.ID,
		ChildID:     c.ChildID,
		ActivityID:  c.ActivityID,
		UserID:      c.UserID,
		CompletedOn: c.CompletedOn,
		Note:        c.Note,
		CreatedAt:   c.CreatedAt,
	}
}
